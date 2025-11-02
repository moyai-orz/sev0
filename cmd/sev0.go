package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"sev0/ent"
	"sev0/internal/discord"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		slog.Error("failed loading *.env file")
	}

	entClient, err := ent.Open(
		"postgres",
		os.Getenv("DATABASE_URL"),
	)
	if err != nil {
		log.Fatalf("failed opening connection to postgres: %v", err)
	}
	defer entClient.Close()
	// Run the auto migration tool.
	if err := entClient.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}

	bot, err := discord.NewDiscordBot(entClient)
	if err != nil {
		slog.Error("failed to create discord bot", "err", err)
		return
	}

	if err := bot.Start(); err != nil {
		slog.Error("failed to start discord bot", "err", err)
		return
	}
	defer bot.Close()

	slog.Info("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
