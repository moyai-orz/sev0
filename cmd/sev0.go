package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"sev0/internal/discord"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		slog.Error("Failed loading *.env file")
	}

	bot, err := discord.NewDiscordBot()
	if err != nil {
		slog.Error("Failed to create discord bot", "err", err)
		return
	}

	if err := bot.Start(); err != nil {
		slog.Error("Failed to start discord bot", "err", err)
		return
	}
	defer bot.Close()

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
