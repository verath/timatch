package timatch

import (
	"bytes"
	"context"
	"github.com/Sirupsen/logrus"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
	"github.com/verath/timatch/lib/dota"
	"strings"
	"sync"
	"text/template"
	"time"
)

// updateInterval is the number of seconds between fetches
// of live matches
const updateInterval = 60 * time.Second

// The league ID of the tournament we are watching (5401 = TI 2017)
const tiLeagueID = 5401

type finishedQueueEntry struct {
	MatchID int64
	AddedAt time.Time
}

type bot struct {
	logger         *logrus.Logger
	discordSession *discordgo.Session
	dotaClient     *dota.Client
	// Ids of discord channels where we post updates
	channelsMu sync.RWMutex
	channels   []string

	// Map of match ids that we have seen in the drafting phase
	matchesDrafting map[int64]bool
	// Map of match ids that we have seen started
	matchesStarted map[int64]bool
	// Map of match ids that were started an are no longer live (i.e. finished)
	matchesFinished map[int64]bool

	// Map of match ids to the match's game number. We must store this as
	// the game number is not provided in the GetMatchDetails result
	gameNumbers map[int64]int

	// Queue of finished matches that we have yet to fetch the finished
	// match details for.
	finishedQueue []finishedQueueEntry
}

func NewBot(logger *logrus.Logger, discordToken string, steamKey string) (*bot, error) {
	if !strings.HasPrefix(discordToken, "Bot ") {
		discordToken = "Bot " + discordToken
	}
	discordSession, err := discordgo.New(discordToken)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating discordgo session")
	}
	dotaClient, err := dota.NewClient(logger, steamKey)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating dotaClient")
	}
	return &bot{
		logger:          logger,
		discordSession:  discordSession,
		dotaClient:      dotaClient,
		matchesDrafting: make(map[int64]bool),
		matchesStarted:  make(map[int64]bool),
		matchesFinished: make(map[int64]bool),
		gameNumbers:     make(map[int64]int),
		finishedQueue:   make([]finishedQueueEntry, 0),
	}, nil
}

func (bot *bot) Run(ctx context.Context) error {
	defer bot.discordSession.AddHandler(bot.onReadyHandler)()
	defer bot.discordSession.AddHandler(bot.onGuildCreate)()
	defer bot.discordSession.AddHandler(bot.onGuildDelete)()
	if err := bot.discordSession.Open(); err != nil {
		return errors.Wrap(err, "Error connecting to Discord")
	}
	defer func() {
		if closeErr := bot.discordSession.Close(); closeErr != nil {
			bot.logger.Error("Error closing Discord connection: %+v", closeErr)
		}
	}()
	return errors.Wrap(bot.run(ctx), "Error during run")
}

func (bot *bot) run(ctx context.Context) error {
	for {
		bot.fetchFinishedMatchDetails(ctx)
		bot.fetchLiveGames(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(updateInterval):
		}
	}
}

func (bot *bot) fetchFinishedMatchDetails(ctx context.Context) {
	remainingQueue := make([]finishedQueueEntry, 0)
	finishedDetails := make([]matchesFinishedDataItem, 0)
	for _, entry := range bot.finishedQueue {
		details, err := bot.dotaClient.GetMatchDetails(ctx, entry.MatchID)
		if err != nil {
			bot.logger.Debugf("Error getting match details for %d: %+v", entry.MatchID, err)
			// Retry entries until they have been in the queue for > 10 min
			if time.Since(entry.AddedAt) <= 10*time.Minute {
				remainingQueue = append(remainingQueue, entry)
			}
			continue
		}
		if details.Result.RadiantWin {
			finishedDetails = append(finishedDetails, matchesFinishedDataItem{
				GameNumber:  bot.gameNumbers[entry.MatchID],
				WinnerName:  details.Result.RadiantName,
				LoserName:   details.Result.DireName,
				WinnerScore: details.Result.RadiantScore,
				LoserScore:  details.Result.DireScore,
			})
		} else {
			finishedDetails = append(finishedDetails, matchesFinishedDataItem{
				GameNumber:  bot.gameNumbers[entry.MatchID],
				WinnerName:  details.Result.DireName,
				LoserName:   details.Result.RadiantName,
				WinnerScore: details.Result.DireScore,
				LoserScore:  details.Result.RadiantScore,
			})
		}
	}
	bot.finishedQueue = remainingQueue
	if len(finishedDetails) > 0 {
		bot.sendTemplateMessage(tmplMatchesFinished, finishedDetails, true)
	}
}

func (bot *bot) fetchLiveGames(ctx context.Context) {
	gamesRes, err := bot.dotaClient.GetLiveLeagueGames(ctx, tiLeagueID)
	if err != nil {
		bot.logger.Debugf("Error getting live games: %+v", err)
		return
	}
	bot.handleLiveGames(gamesRes.Result.Games)
}

func (bot *bot) handleLiveGames(liveGames []dota.LiveLeagueGame) {
	var (
		newDrafting = make([]dota.LiveLeagueGame, 0)
		newStarted  = make([]dota.LiveLeagueGame, 0)
	)
	// Find finished matches (matches started that are no longer live) and
	// add them to a queue of matches we should fetch match details for.
	// Note that fetching details directly is unlikely to work, as the
	// steam api takes some time to update
	for matchID := range bot.matchesStarted {
		if _, ok := bot.matchesFinished[matchID]; !ok {
			finished := true
			for _, game := range liveGames {
				if matchID == game.MatchID {
					finished = false
					break
				}
			}
			if finished {
				bot.matchesFinished[matchID] = true
				entry := finishedQueueEntry{MatchID: matchID, AddedAt: time.Now()}
				bot.finishedQueue = append(bot.finishedQueue, entry)
			}
		}
	}
	// Find new matches, or matches that have gone from drafting -> started
	for _, game := range liveGames {
		bot.gameNumbers[game.MatchID] = game.GameNumber
		if !isGameStarted(game) {
			if _, ok := bot.matchesDrafting[game.MatchID]; !ok {
				newDrafting = append(newDrafting, game)
				bot.matchesDrafting[game.MatchID] = true
			}
		} else {
			if _, ok := bot.matchesStarted[game.MatchID]; !ok {
				newStarted = append(newStarted, game)
				bot.matchesStarted[game.MatchID] = true
			}
		}
	}
	if len(newDrafting) > 0 {
		bot.sendTemplateMessage(tmplMatchesDrafting, newDrafting, false)
	}
	if len(newStarted) > 0 {
		bot.sendTemplateMessage(tmplMatchesStarted, newStarted, true)
	}
}

// isGameStarted tests if a game is past the drafting phase.
func isGameStarted(game dota.LiveLeagueGame) bool {
	if game.Scoreboard.Duration > 0 {
		return true
	}
	// We check for 9 picks rather than 10 to err a bit
	// on the safe side (we don't want to miss a game starting!)
	direPicks := game.Scoreboard.Dire.Picks
	radiantPicks := game.Scoreboard.Radiant.Picks
	if len(direPicks)+len(radiantPicks) >= 9 {
		return true
	}
	return false
}

// addChannel adds a discord channel id to the list of channels that
// should be notified of new matches
func (bot *bot) addChannel(channelID string) {
	bot.channelsMu.Lock()
	defer bot.channelsMu.Unlock()
	bot.channels = append(bot.channels, channelID)
}

// removeChannel removes a discord channel id from the list of channels
// that should be notified of new matches
func (bot *bot) removeChannel(channelID string) {
	bot.channelsMu.Lock()
	defer bot.channelsMu.Unlock()
	for i, c := range bot.channels {
		if c == channelID {
			bot.channels[i] = bot.channels[len(bot.channels)-1]
			bot.channels = bot.channels[:len(bot.channels)-1]
			break
		}
	}
}

// sendMessage sends a message to all registered channels. If tts is true, the
// message is sent as a TTS message
func (bot *bot) sendMessage(content string, tts bool) {
	bot.channelsMu.RLock()
	defer bot.channelsMu.RUnlock()
	for _, channelID := range bot.channels {
		var err error
		if tts {
			_, err = bot.discordSession.ChannelMessageSendTTS(channelID, content)
		} else {
			_, err = bot.discordSession.ChannelMessageSend(channelID, content)
		}
		if err != nil {
			bot.logger.Debugf("Failed sending message to channel %s: %+v", channelID, err)
		}
	}
}

// sendTemplateMessage executes a template with the provided data, then calls
// sendMessage with the template string. If tts is true, the message is sent
// as a TTS message
func (bot *bot) sendTemplateMessage(tmpl *template.Template, data interface{}, tts bool) {
	var msg bytes.Buffer
	err := tmpl.Execute(&msg, data)
	if err != nil {
		bot.logger.Errorf("Failed executing tmpl '%s': %+v", tmpl.Name, err)
		return
	}
	bot.sendMessage(msg.String(), tts)
}

// onReadyHandler is called by discordgo when the discord session is ready,
// i.e. after we have connected to Discord.
func (bot *bot) onReadyHandler(s *discordgo.Session, msg *discordgo.Ready) {
	bot.logger.Debug("Got Ready event")
	err := s.UpdateStatus(-1, "Watching TI!")
	if err != nil {
		bot.logger.Errorf("Could not update status: %+v", err)
	}
}

// onGuildCreate is called whenever a guild is "created". E.g. if we are
// added to a new guild. onGuildCreate is also called for each guild during
// the initial logon sequence
func (bot *bot) onGuildCreate(s *discordgo.Session, msg *discordgo.GuildCreate) {
	bot.logger.Debugf("Got GuildCreate event: %s (%s)", msg.ID, msg.Name)
	// The id of the #general channel is the same as the guild id
	bot.addChannel(msg.ID)
}

// onGuildDelete is called whenever a guild is no longer accessible to us
func (bot *bot) onGuildDelete(s *discordgo.Session, msg *discordgo.GuildDelete) {
	bot.logger.Debugf("Got GuildDelete event: %s", msg.ID)
	bot.removeChannel(msg.ID)
}
