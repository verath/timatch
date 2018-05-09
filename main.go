package main

import (
	"context"
	"flag"
	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/verath/timatch/lib"
	"os"
	"os/signal"
)

func main() {
	var (
		discordToken string
		steamKey     string
		leagueID     uint
		debug        bool
	)
	flag.StringVar(&discordToken, "discordtoken", "", "Discord bot token")
	flag.StringVar(&steamKey, "steamkey", "", "Steam API Key")
	flag.UintVar(&leagueID, "leagueid", 0, "Dota 2 league id of the league to watch")
	flag.BoolVar(&debug, "debug", false, "True to log debug messages")
	flag.Parse()

	logger := logrus.New()
	if debug {
		logger.Level = logrus.DebugLevel
	}
	if discordToken == "" {
		logger.Fatal("discordtoken is required")
	}
	if steamKey == "" {
		logger.Fatal("steamkey is required")
	}
	if leagueID == 0 {
		logger.Fatal("leagueid is required")
	}
	bot, err := timatch.NewBot(logger, discordToken, steamKey, int(leagueID))
	if err != nil {
		logger.Fatal("Error creating bot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopSigs := []os.Signal{os.Interrupt, os.Kill}
	stopCh := make(chan os.Signal, len(stopSigs))
	signal.Notify(stopCh, stopSigs...)
	go func() {
		<-stopCh
		cancel()
	}()
	err = bot.Run(ctx)
	if errors.Cause(err) == context.Canceled {
		logger.Debugf("Error caught in main: %+v", err)
	} else {
		logger.Fatalf("Error caught in main: %+v", err)
	}
}
