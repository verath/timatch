package dota

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

type MatchHistoryResponse struct {
	Result struct {
		Status  int                 `json:"status"`
		Matches []MatchHistoryMatch `json:"matches"`
	} `json:"result"`
}

type MatchHistoryMatch struct {
	MatchID int64 `json:"match_id"`
}

func (res *MatchHistoryResponse) checkResult() bool {
	return res.Result.Status == 1
}

type MatchDetailsResponse struct {
	Result struct {
		*MatchDetails
		Error *string
	} `json:"result"`
}

func (res *MatchDetailsResponse) checkResult() bool {
	return res.Result.Error == nil && res.Result.MatchDetails != nil
}

type MatchDetails struct {
	RadiantWin   bool   `json:"radiant_win"`
	RadiantName  string `json:"radiant_name"`
	DireName     string `json:"dire_name"`
	RadiantScore int    `json:"radiant_score"`
	DireScore    int    `json:"dire_score"`
}
