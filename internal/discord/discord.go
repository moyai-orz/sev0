// Package discord hosts the SEV0 bot
package discord

import (
	"context"
	"log/slog"
	"os"
	"time"

	"sev0/ent"
	"sev0/ent/discordmessage"
	"sev0/ent/discorduser"
	"sev0/internal/contextkeys"
	"sev0/internal/genkitmagic"

	"github.com/bwmarrin/discordgo"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/posthog/posthog-go"
)

type DiscordBot struct {
	session         *discordgo.Session
	entClient       *ent.Client
	embedder        ai.Embedder
	gm              genkitmagic.GenkitMagic
	phc             posthog.Client
	logger          *slog.Logger
	commandHandlers map[string]func(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate)
}

func NewDiscordBot(
	entClient *ent.Client,
	embedder ai.Embedder,
	genkitMagic genkitmagic.GenkitMagic,
	phc posthog.Client,
	logger *slog.Logger,
) (*DiscordBot, error) {
	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		logger.Error("Failed initializing discordgo", "err", err)
		return nil, err
	}

	bot := &DiscordBot{
		session:   dg,
		entClient: entClient,
		embedder:  embedder,
		gm:        genkitMagic,
		phc:       phc,
		logger:    logger,
	}

	bot.commandHandlers = map[string]func(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate){
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
		b.logger.Error("error opening connection", "err", err)
		return err
	}

	guildID := os.Getenv("DISCORD_GUILD_ID")

	registeredCommands, err := b.session.ApplicationCommands(
		b.session.State.User.ID,
		guildID,
	)
	if err != nil {
		b.logger.Error("Could not fetch registered commands", "err", err)
		return err
	}

	localCommands := make(map[string]bool)
	for _, v := range commands {
		localCommands[v.Name] = true
	}

	b.logger.Info("Checking for commands to unregister...")
	for _, v := range registeredCommands {
		b.logger.Info("Unregistering command", "name", v.Name, "id", v.ID)
		err := b.session.ApplicationCommandDelete(
			b.session.State.User.ID,
			guildID,
			v.ID,
		)
		if err != nil {
			b.logger.Error("Cannot delete command", "name", v.Name, "err", err)
		}
	}

	b.logger.Info("Registering commands")
	for _, v := range commands {
		_, err := b.session.ApplicationCommandCreate(
			b.session.State.User.ID,
			guildID,
			v,
		)
		if err != nil {
			b.logger.Error(
				"Cannot create command",
				"command",
				v.Name,
				"err",
				err,
			)
		}
	}

	return nil
}

func (b *DiscordBot) Close() {
	if err := b.session.Close(); err != nil {
		b.logger.Error("error closing discord session", "err", err)
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
		b.logger.Error("failed to create discord user: ", "err", err)
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
		b.logger.Error("failed to create discord message: ", "err", err)
	}

	// jsonData, err := json.MarshalIndent(m, "", "  ")
	// if err != nil {
	// 	logger.Error("failed to marshal message to json", "err", err)
	// 	return
	// }
	//
	// logger.Info(string(jsonData))
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
	// Create a context with the user ID for logging and tracing.
	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else {
		userID = i.User.ID
	}
	ctx := context.WithValue(
		context.Background(),
		contextkeys.UserIDKey,
		userID,
	)

	if h, ok := b.commandHandlers[i.ApplicationCommandData().Name]; ok {
		h(ctx, s, i)
	}
}

func (b *DiscordBot) handleAsk(
	ctx context.Context,
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
) {
	b.phc.Enqueue(posthog.Capture{
		DistinctId: i.Member.User.ID,
		Event:      "ask",
		Properties: posthog.NewProperties().
			Set("global_name", i.Member.User.GlobalName),
	})

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		b.logger.Error("failed to defer interaction", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	options := i.ApplicationCommandData().Options
	var question string
	for _, opt := range options {
		if opt.Name == "question" {
			question = opt.StringValue()
			break
		}
	}

	b.logger.Info("Handling ask command", "question", question)

	config := map[string]any{
		"safetySettings": []map[string]any{
			{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "BLOCK_NONE"},
			{
				"category":  "HARM_CATEGORY_HATE_SPEECH",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_SEXUALLY_EXPLICIT",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_DANGEROUS_CONTENT",
				"threshold": "BLOCK_NONE",
			},
		},
	}

	resp, err := genkit.GenerateText(
		ctx,
		b.gm.G,
		ai.WithPrompt(question),
		ai.WithTools(b.gm.RecentMessagesTool),
		ai.WithSystem(
			"You are a funny & troll Discord bot that lives in this server. You have access to a searchable database of all past messages from this server — use it to recall context, patterns, and memorable moments when replying. Please do not ask any follow up questions, just answer to the best of your ability with the information you have. You should also act like ThePrimeagen.",
		),
		ai.WithConfig(config),
	)

	var content string
	if err != nil {
		b.logger.Error("failed to generate text", "err", err)
		content = "I'm sorry, I encountered an error and couldn't process your question."
	} else if resp == "" {
		content = "The model chose not to provide a response."
	} else {
		content = resp
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	if err != nil {
		b.logger.Error("failed to edit interaction response", "err", err)
	}
}
