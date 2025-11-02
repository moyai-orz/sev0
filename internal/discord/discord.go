// Package discord hosts the SEV0 bot
package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"sev0/ent"
	"sev0/ent/discordmessage"
	"sev0/ent/discorduser"

	"github.com/bwmarrin/discordgo"
	"github.com/firebase/genkit/go/ai"
)

type DiscordBot struct {
	session         *discordgo.Session
	entClient       *ent.Client
	embedder        ai.Embedder
	commandHandlers map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)
}

func NewDiscordBot(
	entClient *ent.Client,
	embedder ai.Embedder,
) (*DiscordBot, error) {
	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		slog.Error("Failed initializing discordgo", "err", err)
		return nil, err
	}

	bot := &DiscordBot{session: dg, entClient: entClient, embedder: embedder}

	bot.commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"ask": bot.handleAsk,
	}
	bot.session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

	bot.session.AddHandler(bot.messageCreate)
	bot.session.AddHandler(bot.messageUpdate)
	bot.session.AddHandler(bot.interactionCreate)

	return bot, nil
}

func (b *DiscordBot) Start() error {
	err := b.session.Open()
	if err != nil {
		slog.Error("error opening connection", "err", err)
		return err
	}

	guildID := os.Getenv("DISCORD_GUILD_ID")

	registeredCommands, err := b.session.ApplicationCommands(
		b.session.State.User.ID,
		guildID,
	)
	if err != nil {
		slog.Error("Could not fetch registered commands", "err", err)
		return err
	}

	localCommands := make(map[string]bool)
	for _, v := range commands {
		localCommands[v.Name] = true
	}

	slog.Info("Checking for commands to unregister...")
	for _, v := range registeredCommands {
		if !localCommands[v.Name] {
			slog.Info("Unregistering command", "name", v.Name, "id", v.ID)
			err := b.session.ApplicationCommandDelete(
				b.session.State.User.ID,
				guildID,
				v.ID,
			)
			if err != nil {
				slog.Error("Cannot delete command", "name", v.Name, "err", err)
			}
		}
	}

	slog.Info("Registering commands")
	for _, v := range commands {
		_, err := b.session.ApplicationCommandCreate(
			b.session.State.User.ID,
			guildID,
			v,
		)
		if err != nil {
			slog.Error("Cannot create command", "command", v.Name, "err", err)
		}
	}

	return nil
}

func (b *DiscordBot) Close() {
	if err := b.session.Close(); err != nil {
		slog.Error("error closing discord session", "err", err)
	}
}

func (b *DiscordBot) messageCreate(
	s *discordgo.Session,
	m *discordgo.MessageCreate,
) {
	b.messageCreateOrUpdate(s, m.Message)
}

func (b *DiscordBot) messageUpdate(
	s *discordgo.Session,
	m *discordgo.MessageUpdate,
) {
	b.messageCreateOrUpdate(s, m.Message)
}

func (b *DiscordBot) messageCreateOrUpdate(
	s *discordgo.Session,
	m *discordgo.Message,
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

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "ask",
		Description: "Ask the bot a question",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "question",
				Description: "The question you want to ask",
				Required:    true,
			},
		},
	},
}

func (b *DiscordBot) interactionCreate(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
) {
	if h, ok := b.commandHandlers[i.ApplicationCommandData().Name]; ok {
		h(s, i)
	}
}

func (b *DiscordBot) handleAsk(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
) {
	options := i.ApplicationCommandData().Options
	var question string
	for _, opt := range options {
		if opt.Name == "question" {
			question = opt.StringValue()
			break
		}
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("You asked: \"%s\"", question),
		},
	})
	if err != nil {
		slog.Error("failed to respond to interaction", "err", err)
		return
	}
}
