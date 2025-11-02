// Package discord hosts the SEV0 bot
package discord

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/bwmarrin/discordgo"
)

type DiscordBot struct {
	Session *discordgo.Session
}

func NewDiscordBot() (*DiscordBot, error) {
	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		slog.Error("Failed initializing discordgo", "err", err)
		return nil, err
	}

	bot := &DiscordBot{Session: dg}
	bot.Session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

	bot.Session.AddHandler(bot.messageIngest)

	return bot, nil
}

func (b *DiscordBot) Start() error {
	err := b.Session.Open()
	if err != nil {
		return fmt.Errorf("error opening connection: %w", err)
	}
	return nil
}

func (b *DiscordBot) Close() {
	if err := b.Session.Close(); err != nil {
		slog.Error("error closing discord session", "err", err)
	}
}

func (b *DiscordBot) messageIngest(
	s *discordgo.Session,
	m *discordgo.MessageCreate,
) {
	if m.GuildID == "" {
		// Ignore DMs
		return
	}

	if m.Author.Bot {
		return
	}

	if m.Author.ID == s.State.User.ID {
		// Ignore itself
		return
	}

	jsonData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		slog.Error("failed to marshal message to json", "err", err)
		return
	}

	slog.Info(string(jsonData))
}
