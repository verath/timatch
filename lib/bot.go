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

type finishedQueueEntry struct {
	MatchID int64
	AddedAt time.Time
}

type guildID string
type channelID string

type bot struct {
	logger         *logrus.Logger
	discordSession *discordgo.Session
	dotaClient     *dota.Client

	// leagueID is the dota 2 league ID of the tournament we
	// are watching
	leagueID int

	channelsMu sync.RWMutex
	// Ids of discord channels where we post updates, each
	// channel id mapping to the guild it is associated with
	channels map[channelID]guildID

	// Map of match ids that we have seen in the drafting phase
	matchesDrafting map[int64]struct{}
	// Map of match ids that we have seen started
	matchesStarted map[int64]struct{}
	// Map of match ids that were started an are no longer live (i.e. finished)
	matchesFinished map[int64]struct{}

	// Map of match ids to the match's game number. We must store this as
	// the game number is not provided in the GetMatchDetails result
	gameNumbers map[int64]int

	// Queue of finished matches that we have yet to fetch the finished
	// match details for.
	finishedQueue []finishedQueueEntry
}

func NewBot(logger *logrus.Logger, discordToken string, steamKey string, leagueID int) (*bot, error) {
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
		leagueID:        leagueID,
		channels:        make(map[channelID]guildID),
		matchesDrafting: make(map[int64]struct{}),
		matchesStarted:  make(map[int64]struct{}),
		matchesFinished: make(map[int64]struct{}),
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
			bot.logger.Errorf("Error closing Discord connection: %+v", closeErr)
		}
	}()
	return errors.Wrap(bot.run(ctx), "Error during run")
}

func (bot *bot) run(ctx context.Context) error {
	for {
		bot.updateLiveGames(ctx)
		bot.updateFinishedGames(ctx)
		bot.fetchFinishedMatchDetails(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(updateInterval):
		}
	}
}

func (bot *bot) updateLiveGames(ctx context.Context) {
	liveGamesRes, err := bot.dotaClient.GetLiveLeagueGames(ctx, bot.leagueID)
	if err != nil {
		bot.logger.Errorf("Error getting live games: %+v", err)
		return
	}
	newDrafting := make([]dota.LiveLeagueGame, 0)
	newStarted := make([]dota.LiveLeagueGame, 0)
	for _, game := range liveGamesRes.Result.Games {
		bot.gameNumbers[game.MatchID] = game.GameNumber
		if !isGameStarted(game) {
			if _, ok := bot.matchesDrafting[game.MatchID]; !ok {
				newDrafting = append(newDrafting, game)
				bot.matchesDrafting[game.MatchID] = struct{}{}
			}
		} else {
			if _, ok := bot.matchesStarted[game.MatchID]; !ok {
				newStarted = append(newStarted, game)
				bot.matchesStarted[game.MatchID] = struct{}{}
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

func (bot *bot) updateFinishedGames(ctx context.Context) {
	if len(bot.matchesStarted) == len(bot.matchesFinished) {
		bot.logger.Debug("Not fetching match history, all known games already finished")
		return
	}
	historyRes, err := bot.dotaClient.GetMatchHistory(ctx, bot.leagueID)
	if err != nil {
		bot.logger.Errorf("Error getting match history: %+v", err)
		return
	}
	for _, match := range historyRes.Result.Matches {
		_, isStarted := bot.matchesStarted[match.MatchID]
		_, isFinished := bot.matchesFinished[match.MatchID]
		if isStarted && !isFinished {
			bot.logger.Debugf("Match finished %d", match.MatchID)
			bot.matchesFinished[match.MatchID] = struct{}{}
			entry := finishedQueueEntry{MatchID: match.MatchID, AddedAt: time.Now()}
			bot.finishedQueue = append(bot.finishedQueue, entry)
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
				bot.logger.Debugf("<= 10 minutes ago, trying %d again next time", entry.MatchID)
				remainingQueue = append(remainingQueue, entry)
			} else {
				bot.logger.Errorf("Giving up on fetching match details for %d")
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

// addGuildChannel adds a channel id to the channels to be notified of new matches. The
// channel is associated with the provided guild id, so that all channels for a given
// guild id can be removed when the guild is removed.
func (bot *bot) addGuildChannel(guildID guildID, channelID channelID) {
	bot.channelsMu.Lock()
	defer bot.channelsMu.Unlock()
	bot.channels[channelID] = guildID
}

// removeGuildChannels removes all discord channel id associated with the guildID from
// the list of channels that should be notified of new matches
func (bot *bot) removeGuildChannels(guildID guildID) {
	bot.channelsMu.Lock()
	defer bot.channelsMu.Unlock()
	for channelID, gID := range bot.channels {
		if gID == guildID {
			delete(bot.channels, channelID)
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
			_, err = bot.discordSession.ChannelMessageSendTTS(string(channelID), content)
		} else {
			_, err = bot.discordSession.ChannelMessageSend(string(channelID), content)
		}
		if err != nil {
			bot.logger.Errorf("Failed sending message to channel %s: %+v", channelID, err)
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
		bot.logger.Errorf("Failed executing template '%s': %+v", tmpl.Name, err)
		return
	}
	bot.sendMessage(msg.String(), tts)
}

// onReadyHandler is called by discordgo when the discord session is ready,
// i.e. after we have connected to Discord.
func (bot *bot) onReadyHandler(s *discordgo.Session, msg *discordgo.Ready) {
	bot.logger.Debug("Got Ready event")
	err := s.UpdateStatus(-1, "Watching TI 2018!")
	if err != nil {
		bot.logger.Errorf("Could not update status: %+v", err)
	}
}

// onGuildCreate is called whenever a guild is "created". E.g. if we are
// added to a new guild. onGuildCreate is also called for each guild during
// the initial logon sequence
func (bot *bot) onGuildCreate(s *discordgo.Session, msg *discordgo.GuildCreate) {
	bot.logger.Debugf("Got GuildCreate event: %s (%s)", msg.ID, msg.Name)
	// Select the channel with the first (lowest) position as the channel to send
	// messages to
	var firstCh *discordgo.Channel
	for _, ch := range msg.Channels {
		if ch.Type != discordgo.ChannelTypeGuildText {
			// not a text channel
			continue
		}
		if firstCh == nil || firstCh.Position > ch.Position {
			firstCh = ch
		}
	}
	if firstCh != nil {
		bot.logger.Debugf("Using channel %s (%s)", firstCh.ID, firstCh.Name)
		bot.addGuildChannel(guildID(msg.ID), channelID(firstCh.ID))
	} else {
		bot.logger.Warnf("No channel for guild %s (%s)", msg.ID, msg.Name)
	}
}

// onGuildDelete is called whenever a guild is no longer accessible to us
func (bot *bot) onGuildDelete(s *discordgo.Session, msg *discordgo.GuildDelete) {
	bot.logger.Debugf("Got GuildDelete event: %s", msg.ID)
	bot.removeGuildChannels(guildID(msg.ID))
}
