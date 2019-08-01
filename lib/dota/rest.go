package dota

import (
	"context"
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/pkg/errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const apiBaseURL = "http://api.steampowered.com"
const pathGetLiveLeagueGames = "/IDOTA2Match_570/GetLiveLeagueGames/v1/"
const pathGetHeroes = "/IEconDOTA2_570/GetHeroes/v1/"
const pathGetMatchHistory = "/IDOTA2Match_570/GetMatchHistory/v1/"
const pathGetMatchDetails = "/IDOTA2Match_570/GetMatchDetails/v1/"

const limitRequestsPerSecond = 1.0

type Client struct {
	logger   *logrus.Logger
	steamKey string
	baseURL  *url.URL

	rateLimitCh chan struct{}
}

func NewClient(logger *logrus.Logger, steamKey string) (*Client, error) {
	baseURL, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, errors.Wrap(err, "Error parsing apiBaseUrl")
	}
	rateLimitCh := make(chan struct{}, 1)
	rateLimitCh <- struct{}{}
	return &Client{
		steamKey:    steamKey,
		baseURL:     baseURL,
		logger:      logger,
		rateLimitCh: rateLimitCh,
	}, nil
}

func (client *Client) getRateLimitToken(ctx context.Context) (returnToken func(), err error) {
	select {
	case <-client.rateLimitCh:
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}
	// The returned func, when called, spawns a go-routine that returns
	// the rate limit token to the channel when a new request is allowed
	// to be made.
	return func() {
		go func() {
			time.Sleep(time.Second / limitRequestsPerSecond)
			client.rateLimitCh <- struct{}{}
		}()
	}, nil
}

func (client *Client) newRequest(ctx context.Context, apiPath string) (*http.Request, error) {
	u, err := url.Parse(apiPath)
	if err != nil {
		return nil, errors.Wrap(err, "Error parsing apiPath")
	}
	reqUrl := client.baseURL.ResolveReference(u)
	query := reqUrl.Query()
	query.Set("key", client.steamKey)
	reqUrl.RawQuery = query.Encode()
	req, err := http.NewRequest("GET", reqUrl.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating Request")
	}
	return req.WithContext(ctx), nil
}

func (client *Client) getJSON(ctx context.Context, req *http.Request, jsonRes interface{}) error {
	returnToken, err := client.getRateLimitToken(ctx)
	if err != nil {
		return errors.Wrap(err, "Error while waiting for rate limit token")
	}
	defer returnToken()

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "Error sending request")
	}
	client.logger.Debugf("GET: %s - [%s]", req.URL.EscapedPath(), res.Status)
	if res.StatusCode != 200 {
		return errors.Errorf("Bad HTTP response status code: %d", res.StatusCode)
	}
	if jsonRes != nil {
		if err := json.NewDecoder(res.Body).Decode(jsonRes); err != nil {
			return errors.Wrap(err, "Error decoding result as JSON")
		}
		if s, ok := jsonRes.(resultChecker); ok {
			if !s.checkResult() {
				return errors.Errorf("Bad steam result")
			}
		}
	}
	return nil
}

func (client *Client) GetHeroes(ctx context.Context, language string) (*HeroesResponse, error) {
	req, err := client.newRequest(ctx, pathGetHeroes)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating new request")
	}
	query := req.URL.Query()
	query.Set("language", language)
	req.URL.RawQuery = query.Encode()
	data := &HeroesResponse{}
	if err := client.getJSON(ctx, req, data); err != nil {
		return nil, errors.Wrap(err, "Error sending request")
	}
	return data, nil
}

func (client *Client) GetLiveLeagueGames(ctx context.Context, leagueID int) (*LiveLeagueGamesResponse, error) {
	req, err := client.newRequest(ctx, pathGetLiveLeagueGames)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating new request")
	}
	query := req.URL.Query()
	query.Set("league_id", strconv.Itoa(leagueID))
	req.URL.RawQuery = query.Encode()
	data := &LiveLeagueGamesResponse{}
	if err := client.getJSON(ctx, req, data); err != nil {
		return nil, errors.Wrap(err, "Error sending request")
	}
	return data, nil
}

func (client *Client) GetMatchHistory(ctx context.Context, leagueID int) (*MatchHistoryResponse, error) {
	req, err := client.newRequest(ctx, pathGetMatchHistory)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating new request")
	}
	query := req.URL.Query()
	query.Set("league_id", strconv.Itoa(leagueID))
	req.URL.RawQuery = query.Encode()
	data := &MatchHistoryResponse{}
	if err := client.getJSON(ctx, req, data); err != nil {
		return nil, errors.Wrap(err, "Error sending request")
	}
	return data, nil
}

func (client *Client) GetMatchDetails(ctx context.Context, matchID int64) (*MatchDetailsResponse, error) {
	req, err := client.newRequest(ctx, pathGetMatchDetails)
	if err != nil {
		return nil, errors.Wrap(err, "Error creating new request")
	}
	query := req.URL.Query()
	query.Set("match_id", strconv.FormatInt(matchID, 10))
	req.URL.RawQuery = query.Encode()
	data := &MatchDetailsResponse{}
	if err := client.getJSON(ctx, req, data); err != nil {
		return nil, errors.Wrap(err, "Error sending request")
	}
	return data, nil
}
