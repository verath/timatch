package timatch

import (
	"bytes"
	"context"
	"github.com/Sirupsen/logrus"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
	"strings"
	"sync"
	"text/template"
	"time"
)

// MatchUpdateInterval is the number of seconds between fetches
// of live matches
const MatchUpdateInterval = 60 * time.Second

// The league ID of the tournament we are watching (5401 = TI 2017)
const tiLeagueID = 5401

var tmplGamesDrafting = template.Must(template.New("GamesDrafting").Parse(strings.TrimSpace(`
{{ range . }}
In Drafting: {{ .RadiantTeam.TeamName }} vs. {{ .DireTeam.TeamName }} (Game {{ .GameNumber }})
{{- end -}}
`)))

var tmplGamesStartedTTS = template.Must(template.New("GamesStartedTTS").Parse(strings.TrimSpace(`
{{ range . }}
Match Started: {{ .RadiantTeam.TeamName }} vs. {{ .DireTeam.TeamName }} (Game {{ .GameNumber }})
{{- end -}}
`)))

type gameFinishedTTSDataItem struct {
	*MatchDetails
	GameNumber int
}

var tmplGamesFinishedTTS = template.Must(template.New("GamesFinishedTTS").Parse(strings.TrimSpace(`
{{ range . -}}
{{- if .RadiantWin }}
Match Ended: {{ .MatchDetails.RadiantName }} defeated {{ .MatchDetails.DireName }} ({{ .MatchDetails.RadiantScore}} - {{ .MatchDetails.DireScore }}, Game {{ .GameNumber }})
{{- else }}
Match Ended: {{ .MatchDetails.DireName }} defeated {{ .MatchDetails.RadiantName }} ({{ .MatchDetails.DireScore}} - {{ .MatchDetails.RadiantScore }}, Game {{ .GameNumber }})
{{- end -}}
{{- end -}}
`)))

type gamesStartedDetailedData []struct {
	*LiveLeagueGame
	RadiantHeroes []string
	DireHeroes    []string
}

type bot struct {
	logger         *logrus.Logger
	discordSession *discordgo.Session
	steamClient    *steamClient
	// Ids of discord channels where we post updates
	channelsMu sync.RWMutex
	channels   []string

	// Map of game ids that we have seen in the drafting phase
	gamesDrafting map[int64]bool
	// Map of game ids that we have seen started
	gamesStarted map[int64]bool
	// Map of game ids that were started an are no longer live (i.e. finished)
	gamesFinished map[int64]bool
	// Map of game ids to their series game number. Needed as this information
	// is only available in GetLiveLeagueGames and not GetMatchDetails
	gameSeriesNumber map[int64]int
	// Map of dota hero ids -> english hero name
	heroNames map[int]string
}

func NewBot(logger *logrus.Logger, discordToken string, steamKey string) (*bot, error) {
	if !strings.HasPrefix(discordToken, "Bot ") {
		discordToken = "Bot " + discordToken
	}
	discordSession, err := discordgo.New(discordToken)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating discordgo session")
	}
	steamClient, err := newSteamClient(logger, steamKey)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating steamClient")
	}
	return &bot{
		logger:           logger,
		discordSession:   discordSession,
		steamClient:      steamClient,
		gamesDrafting:    make(map[int64]bool),
		gamesStarted:     make(map[int64]bool),
		gamesFinished:    make(map[int64]bool),
		gameSeriesNumber: make(map[int64]int),
		heroNames:        make(map[int]string),
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
	if err := bot.fetchHeroNames(ctx); err != nil {
		return errors.Wrap(err, "Error fetching hero names")
	}
	return errors.Wrap(bot.run(ctx), "Error during run")
}

// fetchHeroNames fetches all dota hero names from the steam api and sets
// them in the heroNames map
func (bot *bot) fetchHeroNames(ctx context.Context) error {
	heroesRes, err := bot.steamClient.GetHeroes(ctx, "en_us")
	if err != nil {
		return errors.Wrap(err, "Error fetching Dota hero data")
	}
	for _, hero := range heroesRes.Result.Heroes {
		bot.heroNames[hero.ID] = hero.LocalizedName
	}
	return nil
}

func (bot *bot) heroNameByID(heroID int) (string, error) {
	if name, ok := bot.heroNames[heroID]; ok {
		return name, nil
	}
	return "", errors.Errorf("Could not find hero name for id: %d", heroID)
}

func (bot *bot) run(ctx context.Context) error {
	// fetchWait is used to implement exponential back-off on error from
	// the steam api.
	var fetchWait = MatchUpdateInterval
	for {
		gamesRes, err := bot.steamClient.GetLiveLeagueGames(ctx, tiLeagueID)
		if err != nil {
			fetchWait *= 2
			bot.logger.Debugf("Error getting live games: %+v", err)
		} else {
			fetchWait = MatchUpdateInterval
			if err := bot.handleLiveGamesUpdated(ctx, gamesRes.Result.Games); err != nil {
				return errors.Wrap(err, "Error handling updated live games")
			}
		}
		if fetchWait > 15*time.Minute {
			fetchWait = 15 * time.Minute
		}
		bot.logger.Debugf("Waiting %s until next update", fetchWait)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(fetchWait):
		}
	}
}

func (bot *bot) handleLiveGamesUpdated(ctx context.Context, liveGames []LiveLeagueGame) error {
	var (
		newDrafting = make([]LiveLeagueGame, 0)
		newStarted  = make([]LiveLeagueGame, 0)
		newFinished = make([]gameFinishedTTSDataItem, 0)
	)
	// Find new games, or games that have gone from drafting -> started
	for _, game := range liveGames {
		bot.gameSeriesNumber[game.MatchID] = game.GameNumber
		if !isGameStarted(game) {
			if _, ok := bot.gamesDrafting[game.MatchID]; !ok {
				newDrafting = append(newDrafting, game)
				bot.gamesDrafting[game.MatchID] = true
			}
		} else {
			if _, ok := bot.gamesStarted[game.MatchID]; !ok {
				newStarted = append(newStarted, game)
				bot.gamesStarted[game.MatchID] = true
			}
		}
	}
	// Find finished games (games started that are no longer live)
	for gameID := range bot.gamesStarted {
		if _, ok := bot.gamesFinished[gameID]; ok {
			continue
		}
		finished := true
		for _, game := range liveGames {
			if gameID == game.MatchID {
				finished = false
				break
			}
		}
		if finished {
			bot.gamesFinished[gameID] = true
			// Attempt to lookup match details for the finished game
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			matchDetailsRes, err := bot.steamClient.GetMatchDetails(ctx, gameID)
			if err != nil {
				bot.logger.Debugf("Error getting match details for finished match %d: %+v", gameID, err)
			} else {
				finishedGameData := gameFinishedTTSDataItem{
					MatchDetails: matchDetailsRes.Result.MatchDetails,
					GameNumber:   bot.gameSeriesNumber[gameID],
				}
				newFinished = append(newFinished, finishedGameData)
			}
			cancel()
		}
	}
	if len(newDrafting) > 0 {
		bot.sendTemplateMessage(tmplGamesDrafting, newDrafting, false)
	}
	if len(newStarted) > 0 {
		bot.sendTemplateMessage(tmplGamesStartedTTS, newStarted, true)
	}
	if len(newFinished) > 0 {
		bot.sendTemplateMessage(tmplGamesFinishedTTS, newFinished, true)
	}
	return nil
}

// isGameStarted tests if a game is past the drafting phase.
func isGameStarted(game LiveLeagueGame) bool {
	direPicks := game.Scoreboard.Dire.Picks
	radiantPicks := game.Scoreboard.Radiant.Picks
	if game.Scoreboard.Duration > 0 {
		return true
	}
	// We check for 9 picks rather than 10 to err a bit
	// on the safe side (we don't want to miss a game starting!)
	if len(direPicks)+len(radiantPicks) >= 9 {
		return true
	}
	return false
}

// addChannel adds a discord channel id to the list of channels that
// should be notified of new games
func (bot *bot) addChannel(channelID string) {
	bot.channelsMu.Lock()
	defer bot.channelsMu.Unlock()
	bot.channels = append(bot.channels, channelID)
}

// removeChannel removes a discord channel id from the list of channels
// that should be notified of new games
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

// sendMessage sends a TTS (text to speech) message to all registered
// channels
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
// sendMessage with the template string. If tts is true, the message will be
// sent as a TTS message
func (bot *bot) sendTemplateMessage(tmpl *template.Template, data interface{}, tts bool) {
	var msg bytes.Buffer
	err := tmpl.Funcs(template.FuncMap{
		"heroNameByID": bot.heroNameByID,
	}).Execute(&msg, data)
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
