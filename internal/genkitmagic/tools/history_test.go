package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"sev0/ent"
	"sev0/internal/testutil"
)

func seed(t *testing.T, c *ent.Client, id, guild, channel, author, content string, when time.Time) {
	t.Helper()
	err := c.DiscordMessage.Create().SetID(id).SetGuildID(guild).SetChannelID(channel).SetUserID(author).SetTimestamp(when).SetContent(content).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}
func TestSearchScopeAndFilters(t *testing.T) {
	c := testutil.Database(t)
	ctx := context.Background()
	c.DiscordUser.Create().SetID("alice").SetUsername("alice123").SetGlobalName("Alice").SaveX(ctx)
	c.DiscordUser.Create().SetID("bob").SetUsername("bob").SetGlobalName("Bob").SaveX(ctx)
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seed(t, c, "1", "guild", "general", "alice", "Production broke because deploy failed", when)
	seed(t, c, "2", "guild", "engineering", "alice", "Production rollback fixed it", when.Add(time.Hour))
	seed(t, c, "3", "guild", "secret", "alice", "Production secret", when)
	seed(t, c, "4", "other", "general", "alice", "Production another server", when)
	seed(t, c, "5", "guild", "general", "bob", "Production was fine", when.Add(2*time.Hour))
	// Old rows cannot be attributed to a channel.
	c.DiscordMessage.Create().SetID("legacy").SetUserID("alice").SetTimestamp(when).SetContent("Production legacy").SaveX(ctx)
	scoped := WithScope(ctx, "guild", "general", "general", "engineering")
	got, err := Search(scoped, c, SearchInput{Query: "production", Author: "ali", After: "2026-01-01", Before: "2026-01-02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 || got.Messages[0].ID != "1" || got.Messages[1].ChannelID != "engineering" {
		t.Fatalf("unexpected matches: %+v", got)
	}
	if got.Messages[1].URL != "https://discord.com/channels/guild/engineering/2" {
		t.Fatal(got.Messages[1].URL)
	}
	got, err = Search(scoped, c, SearchInput{Query: "production rollback"})
	if err != nil || len(got.Messages) != 1 || got.Messages[0].ID != "2" {
		t.Fatalf("keyword intersection: %+v %v", got, err)
	}
	got, err = Search(scoped, c, SearchInput{ChannelID: "secret"})
	if err != nil || len(got.Messages) != 0 {
		t.Fatalf("scope bypass: %+v %v", got, err)
	}
	if _, err = Search(ctx, c, SearchInput{}); err == nil {
		t.Fatal("missing scope accepted")
	}
	if _, err = Context(scoped, c, "3"); err == nil {
		t.Fatal("restricted context returned")
	}
	for _, input := range []SearchInput{{After: "bad"}, {After: "2026-02-01", Before: "2026-01-01"}, {Query: strings.Repeat("a ", 9)}} {
		if _, err = Search(scoped, c, input); err == nil {
			t.Fatalf("invalid input accepted: %+v", input)
		}
	}
}
func TestBoundsAndConversationContext(t *testing.T) {
	c := testutil.Database(t)
	ctx := WithScope(context.Background(), "g", "c", "c", "elsewhere")
	c.DiscordUser.Create().SetID("a").SetUsername("alice").SetGlobalName("Alice").SaveX(ctx)
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 60; i++ {
		seed(t, c, fmt.Sprintf("%03d", i), "g", "c", "a", strings.Repeat("猫", 1300), when.Add(time.Duration(i)*time.Minute))
	}
	seed(t, c, "other-channel", "g", "elsewhere", "a", "unrelated", when.Add(30*time.Minute))
	got, err := Search(ctx, c, SearchInput{ChannelID: "c", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 || got.Messages[0].ID != "058" || !got.HasMore || !got.Messages[0].Truncated || len([]rune(got.Messages[0].Content)) != 1200 {
		t.Fatalf("bad bounds: %+v", got)
	}
	got, err = Search(ctx, c, SearchInput{ChannelID: "c", Limit: 100000})
	if err != nil || len(got.Messages) != 40 {
		t.Fatalf("uncapped: %v %v", len(got.Messages), err)
	}
	c.DiscordMessage.UpdateOneID("030").SetReplyToID("000").SaveX(ctx)
	got, err = Context(ctx, c, "030")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 17 || got.Messages[0].ID != "000" {
		t.Fatalf("missing context/reply: %+v", got)
	}
	for _, m := range got.Messages {
		if m.ChannelID != "c" {
			t.Fatal("mixed surrounding conversation channels")
		}
	}
}

func TestPaginationTiesAndLiteralKeywords(t *testing.T) {
	c := testutil.Database(t)
	ctx := WithScope(context.Background(), "g", "c")
	c.DiscordUser.Create().SetID("a").SetUsername("alice").SetGlobalName("Alice").SaveX(ctx)
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 45; i++ {
		seed(t, c, fmt.Sprintf("%03d", i), "g", "c", "a", "hello", when)
	}
	first, err := Search(ctx, c, SearchInput{Limit: 40})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Search(ctx, c, SearchInput{Limit: 40, Before: first.NextBefore, BeforeID: first.NextBeforeID})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Messages) != 40 || len(second.Messages) != 5 || second.HasMore {
		t.Fatal("pagination skipped tied timestamps")
	}
	got, err := Search(ctx, c, SearchInput{Query: "%"})
	if err != nil || len(got.Messages) != 0 {
		t.Fatalf("wildcard was not literal: %+v %v", got, err)
	}
	seed(t, c, "long", "g", "c", "a", strings.Repeat("猫", 1500)+"needle"+strings.Repeat("猫", 1000), when)
	got, err = Search(ctx, c, SearchInput{Query: "needle"})
	if err != nil || len(got.Messages) != 1 || !strings.Contains(got.Messages[0].Content, "needle") {
		t.Fatal("search excerpt lost the match", err)
	}
}

func TestMigrationPreservesUnscopedHistory(t *testing.T) {
	c := testutil.LegacyDatabase(t)
	ctx := context.Background()
	c.DiscordUser.Create().SetID("a").SetUsername("alice").SetGlobalName("Alice").SaveX(ctx)
	c.DiscordMessage.Create().SetID("legacy").SetUserID("a").SetTimestamp(time.Now()).SetContent("old history").SaveX(ctx)
	testutil.Migrate(t, c)
	row := c.DiscordMessage.GetX(ctx, "legacy")
	if row.Content != "old history" || row.GuildID != "" || row.ChannelID != "" {
		t.Fatal("legacy row changed unexpectedly")
	}
	got, err := Search(WithScope(ctx, "g", "c"), c, SearchInput{})
	if err != nil || len(got.Messages) != 0 {
		t.Fatal("unscoped legacy row exposed", err)
	}
}
