// Package discord hosts the SEV0 bot
package discord

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"sev0/ent"
	"sev0/ent/discordmessage"
	"sev0/ent/discorduser"

	"github.com/bwmarrin/discordgo"
)

type DiscordBot struct {
	Session   *discordgo.Session
	entClient *ent.Client
}

func NewDiscordBot(entClient *ent.Client) (*DiscordBot, error) {
	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		slog.Error("Failed initializing discordgo", "err", err)
		return nil, err
	}

	bot := &DiscordBot{Session: dg, entClient: entClient}
	bot.Session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

	bot.Session.AddHandler(bot.messageCreate)
	bot.Session.AddHandler(bot.messageUpdate)

	return bot, nil
}

func (b *DiscordBot) Start() error {
	err := b.Session.Open()
	if err != nil {
		slog.Error("error opening connection", "err", err)
		return err

	}
	return nil
}

func (b *DiscordBot) Close() {
	if err := b.Session.Close(); err != nil {
		slog.Error("error closing discord session", "err", err)
	}
}

func (b *DiscordBot) messageCreate(
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

	if m.Content == "" {
		// TODO: maybe handle non-text files later?
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := b.entClient.DiscordUser.Create().
		SetGlobalName(m.Author.GlobalName).
		SetUsername(m.Author.Username).
		SetID(m.Author.ID).
		OnConflictColumns(discorduser.FieldID).
		UpdateNewValues().
		ID(ctx)
	if err != nil {
		slog.Error("failed to create discord user: ", "err", err)
	}

	create := b.entClient.DiscordMessage.Create().
		SetID(m.ID).
		SetContent(m.Content).
		SetUserID(userID).
		SetTimestamp(m.Timestamp)

	if m.EditedTimestamp != nil {
		create.SetEditedTimestamp(*m.EditedTimestamp)
	}

	err = create.OnConflictColumns(discordmessage.FieldID).
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		slog.Error("failed to create discord message: ", "err", err)
	}

	jsonData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		slog.Error("failed to marshal message to json", "err", err)
		return
	}

	slog.Info(string(jsonData))
}

func (b *DiscordBot) messageUpdate(
	s *discordgo.Session,
	m *discordgo.MessageUpdate,
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

	if m.Content == "" {
		// TODO: maybe handle non-text files later?
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := b.entClient.DiscordUser.Create().
		SetGlobalName(m.Author.GlobalName).
		SetUsername(m.Author.Username).
		SetID(m.Author.ID).
		OnConflictColumns(discorduser.FieldID).
		UpdateNewValues().
		ID(ctx)
	if err != nil {
		slog.Error("failed to create discord user: ", "err", err)
	}

	create := b.entClient.DiscordMessage.Create().
		SetID(m.ID).
		SetContent(m.Content).
		SetUserID(userID).
		SetTimestamp(m.Timestamp)

	if m.EditedTimestamp != nil {
		create.SetEditedTimestamp(*m.EditedTimestamp)
	}

	err = create.OnConflictColumns(discordmessage.FieldID).
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		slog.Error("failed to create discord message: ", "err", err)
	}

	// jsonData, err := json.MarshalIndent(m, "", "  ")
	// if err != nil {
	// 	slog.Error("failed to marshal message to json", "err", err)
	// 	return
	// }
	//
	// slog.Info(string(jsonData))
}
