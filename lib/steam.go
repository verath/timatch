package timatch

import (
	"context"
	"encoding/json"
	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"net/http"
	"net/url"
	"strconv"
)

const apiBaseURL = "http://api.steampowered.com"
const pathGetLiveLeagueGames = "/IDOTA2Match_570/GetLiveLeagueGames/v1/"
const pathGetHeroes = "/IEconDOTA2_570/GetHeroes/v1/"
const pathGetMatchDetails = "/IDOTA2Match_570/GetMatchDetails/v1/"

type steamClient struct {
	steamKey string
	baseURL  *url.URL

	logger *logrus.Logger
}

func newSteamClient(logger *logrus.Logger, steamKey string) (*steamClient, error) {
	baseURL, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, errors.Wrap(err, "Error parsing apiBaseUrl")
	}
	return &steamClient{
		steamKey: steamKey,
		baseURL:  baseURL,
		logger:   logger,
	}, nil
}

func (sc *steamClient) newRequest(ctx context.Context, apiPath string) (*http.Request, error) {
	u, err := url.Parse(apiPath)
	if err != nil {
		return nil, errors.Wrap(err, "Error parsing apiPath")
	}
	reqUrl := sc.baseURL.ResolveReference(u)
	query := reqUrl.Query()
	query.Set("key", sc.steamKey)
	reqUrl.RawQuery = query.Encode()
	req, err := http.NewRequest("GET", reqUrl.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating Request")
	}
	return req.WithContext(ctx), nil
}

func (sc *steamClient) getJSON(req *http.Request, v interface{}) error {
	sc.logger.Debugf("GET: %s", req.URL.String())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "Error sending request")
	}
	if res.StatusCode != 200 {
		return errors.Errorf("Bad HTTP response status code: %d", res.StatusCode)
	}
	if v != nil {
		if err := json.NewDecoder(res.Body).Decode(v); err != nil {
			return errors.Wrap(err, "Error decoding result as JSON")
		}
		if s, ok := v.(resultChecker); ok {
			if !s.checkResult() {
				return errors.Errorf("Bad steam result")
			}
		}
	}
	return nil
}

func (sc *steamClient) GetHeroes(ctx context.Context, language string) (*HeroesResponse, error) {
	req, err := sc.newRequest(ctx, pathGetHeroes)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating new request")
	}
	query := req.URL.Query()
	query.Set("language", language)
	req.URL.RawQuery = query.Encode()
	data := &HeroesResponse{}
	if err := sc.getJSON(req, data); err != nil {
		return nil, errors.Wrap(err, "Error sending request")
	}
	return data, nil
}

func (sc *steamClient) GetLiveLeagueGames(ctx context.Context, leagueID int) (*LiveLeagueGamesResponse, error) {
	req, err := sc.newRequest(ctx, pathGetLiveLeagueGames)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating new request")
	}
	query := req.URL.Query()
	query.Set("league_id", strconv.Itoa(leagueID))
	req.URL.RawQuery = query.Encode()
	data := &LiveLeagueGamesResponse{}
	if err := sc.getJSON(req, data); err != nil {
		return nil, errors.Wrap(err, "Error sending request")
	}
	return data, nil
}

func (sc *steamClient) GetMatchDetails(ctx context.Context, matchID int64) (*MatchDetailsResponse, error) {
	req, err := sc.newRequest(ctx, pathGetMatchDetails)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating new request")
	}
	query := req.URL.Query()
	query.Set("match_id", strconv.FormatInt(matchID, 10))
	req.URL.RawQuery = query.Encode()
	data := &MatchDetailsResponse{}
	if err := sc.getJSON(req, data); err != nil {
		return nil, errors.Wrap(err, "Error sending request")
	}
	return data, nil
}

type resultChecker interface {
	checkResult() bool
}

type LiveLeagueGamesResponse struct {
	Result struct {
		Status int              `json:"status"`
		Games  []LiveLeagueGame `json:"games"`
	} `json:"result"`
}

type LiveLeagueGame struct {
	RadiantTeam LiveLeagueGamesTeam      `json:"radiant_team"`
	DireTeam    LiveLeagueGamesTeam      `json:"dire_team"`
	GameNumber  int                      `json:"game_number"`
	MatchID     int64                    `json:"match_id"`
	Scoreboard  LiveLeagueGameScoreboard `json:"scoreboard"`
}

type LiveLeagueGamesTeam struct {
	TeamName string `json:"team_name"`
}

type LiveLeagueGameScoreboard struct {
	Duration float32                      `json:"duration"`
	Radiant  LiveLeagueGameScoreboardTeam `json:"radiant"`
	Dire     LiveLeagueGameScoreboardTeam `json:"dire"`
}

type LiveLeagueGameScoreboardTeam struct {
	Bans []struct {
		HeroID int `json:"hero_id"`
	} `json:"bans"`

	Picks []struct {
		HeroID int `json:"hero_id"`
	} `json:"picks"`

	Players []struct {
		HeroID int `json:"hero_id"`
	} `json:"players"`
}

func (res *LiveLeagueGamesResponse) checkResult() bool {
	return res.Result.Status == 200
}

type HeroesResponse struct {
	Result struct {
		Status int `json:"status"`
		Count  int `json:"count"`
		Heroes []struct {
			Name          string `json:"name"`
			ID            int    `json:"id"`
			LocalizedName string `json:"localized_name"`
		} `json:"heroes"`
	} `json:"result"`
}

func (res *HeroesResponse) checkResult() bool {
	return res.Result.Status == 200
}

type MatchDetailsResponse struct {
	Result MatchDetails `json:"result"`
}

type MatchDetails struct {
	RadiantWin   bool   `json:"radiant_win"`
	RadiantName  string `json:"radiant_name"`
	DireName     string `json:"dire_name"`
	RadiantScore int    `json:"radiant_score"`
	DireScore    int    `json:"dire_score"`
}
