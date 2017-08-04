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
		debug        bool
	)
	flag.StringVar(&discordToken, "discordtoken", "", "Discord bot token")
	flag.StringVar(&steamKey, "steamkey", "", "Steam API Key")
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
	bot, err := timatch.NewBot(logger, discordToken, steamKey)
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
