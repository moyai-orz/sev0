package discord

import (
	"context"
	"fmt"
	"time"

	"sev0/ent/discordmessage"
	"sev0/ent/discordmessageembedding"
	"sev0/ent/discorduser"

	"github.com/bwmarrin/discordgo"
)

func displayName(user *discordgo.User) string {
	if user.GlobalName != "" {
		return user.GlobalName
	}
	return user.Username
}

func (b *DiscordBot) storeMessage(ctx context.Context, m *discordgo.Message) error {
	if m == nil || m.Author == nil || m.GuildID == "" || m.ChannelID == "" || m.Author.Bot {
		return nil
	}
	if m.Content == "" {
		// An edit that removes text must not leave stale searchable content.
		return b.removeMessages(ctx, []string{m.ID})
	}
	if m.Timestamp.IsZero() {
		return fmt.Errorf("message timestamp missing")
	}
	if err := b.entClient.DiscordUser.Create().SetID(m.Author.ID).SetUsername(m.Author.Username).SetGlobalName(displayName(m.Author)).OnConflictColumns(discorduser.FieldID).UpdateNewValues().Exec(ctx); err != nil {
		return err
	}
	create := b.entClient.DiscordMessage.Create().SetID(m.ID).SetContent(m.Content).SetUserID(m.Author.ID).SetTimestamp(m.Timestamp).SetGuildID(m.GuildID).SetChannelID(m.ChannelID)
	if m.MessageReference != nil && m.MessageReference.Type == discordgo.MessageReferenceTypeDefault {
		create.SetReplyToID(m.MessageReference.MessageID)
	}
	if m.EditedTimestamp != nil {
		create.SetEditedTimestamp(*m.EditedTimestamp)
	}
	return create.OnConflictColumns(discordmessage.FieldID).UpdateNewValues().Exec(ctx)
}

func (b *DiscordBot) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.storeMessage(ctx, m.Message); err != nil {
		b.logger.Error("failed to store message", "err", err)
	}
}
func (b *DiscordBot) messageUpdate(s *discordgo.Session, event *discordgo.MessageUpdate) {
	if event.Message == nil || event.GuildID == "" {
		return
	}
	// Gateway updates can omit author/content. Fetch the complete current message
	// rather than overwriting stored text with omitted fields or dereferencing nil.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	message, err := s.ChannelMessage(event.ChannelID, event.ID, discordgo.WithContext(ctx))
	if err != nil {
		b.logger.Warn("failed to fetch edited message", "err", err)
		return
	}
	message.GuildID = event.GuildID
	b.conversations.invalidateChannel(event.ChannelID)
	if err := b.storeMessage(ctx, message); err != nil {
		b.logger.Error("failed to store edited message", "err", err)
	}
}
func (b *DiscordBot) deleteMessages(guildID, channelID string, ids []string) {
	if guildID == "" || channelID == "" || len(ids) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Discord snowflakes are globally unique; also remove legacy rows lacking scope.
	b.conversations.invalidateChannel(channelID)
	err := b.removeMessages(ctx, ids)
	if err != nil {
		b.logger.Error("failed to delete recorded messages", "err", err)
	}
}
func (b *DiscordBot) messageDelete(s *discordgo.Session, event *discordgo.MessageDelete) {
	if event.Message != nil {
		b.deleteMessages(event.GuildID, event.ChannelID, []string{event.ID})
	}
}
func (b *DiscordBot) messageDeleteBulk(s *discordgo.Session, event *discordgo.MessageDeleteBulk) {
	b.deleteMessages(event.GuildID, event.ChannelID, event.Messages)
}

// Fetch channel overwrites and current member roles for each request. Limit
// cross-channel retrieval to ordinary text/announcement channels; private and
// archived thread membership needs separate handling. The invoking thread can
// still supply its own recent history.
func historyChannels(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) ([]string, error) {
	channels, err := s.GuildChannels(i.GuildID, discordgo.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	guild, err := s.Guild(i.GuildID, discordgo.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return allowedHistoryChannels(guild, channels, i)
}

func allowedHistoryChannels(guild *discordgo.Guild, channels []*discordgo.Channel, i *discordgo.InteractionCreate) ([]string, error) {
	state := discordgo.NewState()
	guild.Channels = channels
	if err := state.GuildAdd(guild); err != nil {
		return nil, err
	}
	member := *i.Member
	member.GuildID = i.GuildID
	if err := state.MemberAdd(&member); err != nil {
		return nil, err
	}
	const required = discordgo.PermissionViewChannel | discordgo.PermissionReadMessageHistory
	allowed := []string{}
	if i.Member.Permissions&required == required {
		allowed = append(allowed, i.ChannelID)
	}
	for _, channel := range channels {
		if channel.ID == i.ChannelID || (channel.Type != discordgo.ChannelTypeGuildText && channel.Type != discordgo.ChannelTypeGuildNews) {
			continue
		}
		permissions, err := state.UserChannelPermissions(i.Member.User.ID, channel.ID)
		if err == nil && permissions&required == required {
			allowed = append(allowed, channel.ID)
		}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("no readable history channels")
	}
	return allowed, nil
}

func (b *DiscordBot) removeMessages(ctx context.Context, ids []string) error {
	tx, err := b.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.DiscordMessageEmbedding.Delete().Where(discordmessageembedding.MessageIDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	if _, err = tx.DiscordMessage.Delete().Where(discordmessage.IDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
