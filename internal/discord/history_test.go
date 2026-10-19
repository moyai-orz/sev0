package discord

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
	"github.com/pgvector/pgvector-go"
	"sev0/internal/testutil"
)

func TestStoreEditDelete(t *testing.T) {
	c := testutil.Database(t)
	b := &DiscordBot{entClient: c}
	ctx := context.Background()
	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &discordgo.Message{ID: "m", GuildID: "g", ChannelID: "c", Author: &discordgo.User{ID: "u", Username: "username"}, Content: "original", Timestamp: original, MessageReference: &discordgo.MessageReference{MessageID: "parent"}}
	if err := b.storeMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	row := c.DiscordMessage.GetX(ctx, "m")
	if row.GuildID != "g" || row.ChannelID != "c" || row.ReplyToID != "parent" {
		t.Fatalf("missing scope/reply: %+v", row)
	}
	if c.DiscordUser.GetX(ctx, "u").GlobalName != "username" {
		t.Fatal("missing display-name fallback")
	}
	edited := original.Add(time.Hour)
	m.Content = "edited"
	m.EditedTimestamp = &edited
	if err := b.storeMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	row = c.DiscordMessage.GetX(ctx, "m")
	if row.Content != "edited" || !row.Timestamp.Equal(original) {
		t.Fatal("bad edit")
	}
	// Existing embeddings must not block deletion via their foreign key.
	c.DiscordMessageEmbedding.Create().SetID("embedding").SetMessageID("m").SetModel("old").SetEmbedding(pgvector.NewVector([]float32{1, 2})).SaveX(ctx)
	m.Content = ""
	if err := b.storeMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	if c.DiscordMessage.Query().CountX(ctx) != 0 || c.DiscordMessageEmbedding.Query().CountX(ctx) != 0 {
		t.Fatal("deleted text survived")
	}
	if err := b.storeMessage(ctx, &discordgo.Message{GuildID: "g", ChannelID: "c"}); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryChannelPermissions(t *testing.T) {
	const read = discordgo.PermissionViewChannel | discordgo.PermissionReadMessageHistory
	guild := &discordgo.Guild{ID: "g", OwnerID: "owner", Roles: []*discordgo.Role{{ID: "g", Permissions: read}}}
	channels := []*discordgo.Channel{
		{ID: "current", GuildID: "g", Type: discordgo.ChannelTypeGuildText},
		{ID: "public", GuildID: "g", Type: discordgo.ChannelTypeGuildText},
		{ID: "secret", GuildID: "g", Type: discordgo.ChannelTypeGuildText, PermissionOverwrites: []*discordgo.PermissionOverwrite{{ID: "g", Type: discordgo.PermissionOverwriteTypeRole, Deny: discordgo.PermissionViewChannel}}},
		{ID: "no-history", GuildID: "g", Type: discordgo.ChannelTypeGuildText, PermissionOverwrites: []*discordgo.PermissionOverwrite{{ID: "g", Type: discordgo.PermissionOverwriteTypeRole, Deny: discordgo.PermissionReadMessageHistory}}},
		{ID: "private-thread", GuildID: "g", Type: discordgo.ChannelTypeGuildPrivateThread},
	}
	i := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{GuildID: "g", ChannelID: "current", Member: &discordgo.Member{User: &discordgo.User{ID: "u"}, Permissions: read}}}
	allowed, err := allowedHistoryChannels(guild, channels, i)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(allowed, ",") != "current,public" {
		t.Fatalf("wrong accessible channels: %v", allowed)
	}
	i.ChannelID = "private-thread"
	allowed, err = allowedHistoryChannels(guild, channels, i)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(allowed, ",") != "private-thread,current,public" {
		t.Fatalf("current thread unavailable: %v", allowed)
	}
}

func TestTruncateDiscordUnicode(t *testing.T) {
	input := strings.Repeat("猫😀", 1000)
	result := truncateDiscord(input, 2000)
	if !utf8.ValidString(result) || len(utf16.Encode([]rune(result))) > 2000 || !strings.HasSuffix(result, "(response shortened)") {
		t.Fatal("invalid Discord output")
	}
	if truncateDiscord("short", 2000) != "short" {
		t.Fatal("short output changed")
	}
}
