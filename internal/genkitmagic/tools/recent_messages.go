package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sev0/ent"
	"sev0/ent/discordmessage"
	"sev0/ent/discorduser"
	"sev0/ent/predicate"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

const maxMessages = 40
const maxContentRunes = 1200

type scopeKey struct{}
type Scope struct {
	GuildID, ChannelID string
	AllowedChannelIDs  []string
}

// WithScope binds retrieval to the authorized server and channels. Models cannot override it.
func WithScope(ctx context.Context, guildID, channelID string, allowedChannelIDs ...string) context.Context {
	if len(allowedChannelIDs) == 0 {
		allowedChannelIDs = []string{channelID}
	}
	return context.WithValue(ctx, scopeKey{}, Scope{GuildID: guildID, ChannelID: channelID, AllowedChannelIDs: append([]string(nil), allowedChannelIDs...)})
}

func scopedQuery(ctx context.Context, client *ent.Client) (*ent.DiscordMessageQuery, error) {
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	if !ok || scope.GuildID == "" || scope.ChannelID == "" {
		return nil, fmt.Errorf("message history requires a server and channel")
	}
	return client.DiscordMessage.Query().Where(discordmessage.GuildID(scope.GuildID), discordmessage.ChannelIDIn(scope.AllowedChannelIDs...)), nil
}

type SearchInput struct {
	ChannelID string `json:"channel_id,omitempty" jsonschema:"description=Optional channel ID to narrow the search. Omit to search all permitted channels in this server."`
	Query     string `json:"query,omitempty" jsonschema:"description=Space-separated literal keywords. All must match. Use short distinctive words; empty means browse history."`
	Author    string `json:"author,omitempty" jsonschema:"description=Author Discord ID or part of their username or display name."`
	After     string `json:"after,omitempty" jsonschema:"description=Inclusive UTC date (YYYY-MM-DD) or RFC3339 timestamp."`
	Before    string `json:"before,omitempty" jsonschema:"description=Exclusive UTC date or RFC3339 timestamp. For paging use next_before and next_before_id together."`
	BeforeID  string `json:"before_id,omitempty" jsonschema:"description=Paging tie-breaker. Copy next_before_id from the previous result along with next_before."`
	Limit     int    `json:"limit,omitempty"`
}

type RecentMessagesInput struct {
	Limit int `json:"limit,omitempty"`
}
type ContextInput struct {
	MessageID string `json:"message_id"`
}
type Message struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	AuthorID  string    `json:"author_id"`
	Timestamp time.Time `json:"timestamp"`
	URL       string    `json:"url"`
	ReplyToID string    `json:"reply_to_id,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
}
type RecentMessagesOutput struct {
	Messages     []Message `json:"messages"`
	HasMore      bool      `json:"has_more"`
	Coverage     string    `json:"coverage"`
	NextBefore   string    `json:"next_before,omitempty"`
	NextBeforeID string    `json:"next_before_id,omitempty"`
}

const coverage = "Only recorded text messages in permitted channels of this server are available. Messages missed while offline and legacy rows without channel metadata are not included. An empty result is not proof an event never happened."

func boundedLimit(n int) int {
	if n <= 0 {
		return 20
	}
	return min(n, maxMessages)
}
func parseTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q; use YYYY-MM-DD or RFC3339", value)
}

// Search uses literal, case-insensitive keyword matching, including non-English
// text. Channel/time indexes narrow the scan; limits and deadlines bound results.
func Search(ctx context.Context, client *ent.Client, input SearchInput) (*RecentMessagesOutput, error) {
	query, err := scopedQuery(ctx, client)
	if err != nil {
		return nil, err
	}
	if input.ChannelID != "" {
		query.Where(discordmessage.ChannelID(input.ChannelID))
	}
	if len([]rune(input.Query)) > 200 || len([]rune(input.Author)) > 100 {
		return nil, fmt.Errorf("search query or author is too long")
	}
	terms := strings.Fields(input.Query)
	if len(terms) > 8 {
		return nil, fmt.Errorf("use at most 8 search keywords")
	}
	for _, term := range terms {
		query.Where(discordmessage.ContentContainsFold(term))
	}
	if author := strings.TrimSpace(input.Author); author != "" {
		author = strings.Trim(author, "<@!>")
		query.Where(discordmessage.Or(discordmessage.AuthorID(author), discordmessage.HasUserWith(discorduser.Or(discorduser.UsernameContainsFold(author), discorduser.GlobalNameContainsFold(author)))))
	}
	if input.BeforeID != "" && input.Before == "" {
		return nil, fmt.Errorf("before_id requires before")
	}
	var after, before time.Time
	if input.After != "" {
		after, err = parseTime(input.After)
		if err != nil {
			return nil, err
		}
		query.Where(discordmessage.TimestampGTE(after))
	}
	if input.Before != "" {
		before, err = parseTime(input.Before)
		if err != nil {
			return nil, err
		}
		if input.BeforeID == "" {
			query.Where(discordmessage.TimestampLT(before))
		} else {
			query.Where(discordmessage.Or(discordmessage.TimestampLT(before), discordmessage.And(discordmessage.TimestampEQ(before), discordmessage.IDLT(input.BeforeID))))
		}
	}
	if !after.IsZero() && !before.IsZero() && !after.Before(before) {
		return nil, fmt.Errorf("after must be earlier than before")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	limit := boundedLimit(input.Limit)
	rows, err := query.Order(ent.Desc(discordmessage.FieldTimestamp), ent.Desc(discordmessage.FieldID)).Limit(limit + 1).WithUser().All(ctx)
	if err != nil {
		return nil, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	// Read conversations in the order they happened, even though selection is newest-first.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	out := pack(rows, more, terms...)
	if more && len(rows) > 0 {
		out.NextBefore = rows[0].Timestamp.Format(time.RFC3339Nano)
		out.NextBeforeID = rows[0].ID
	}
	return out, nil
}

func pack(rows []*ent.DiscordMessage, more bool, terms ...string) *RecentMessagesOutput {
	out := &RecentMessagesOutput{Messages: make([]Message, 0, len(rows)), HasMore: more, Coverage: coverage}
	budget := maxContentRunes
	if len(rows) > 0 {
		budget = min(budget, 12000/len(rows))
	}
	for _, row := range rows {
		name := row.AuthorID
		if row.Edges.User != nil {
			name = row.Edges.User.GlobalName
			if name == "" {
				name = row.Edges.User.Username
			}
		}
		content := []rune(row.Content)
		truncated := len(content) > budget
		if truncated {
			start := 0
			for _, term := range terms {
				if index := strings.Index(strings.ToLower(row.Content), strings.ToLower(term)); index >= 0 {
					start = max(0, len([]rune(strings.ToLower(row.Content)[:index]))-budget/3)
					break
				}
			}
			start = min(start, len(content)-budget)
			content = content[start : start+budget]
		}
		out.Messages = append(out.Messages, Message{ID: row.ID, ChannelID: row.ChannelID, Content: string(content), Author: name, AuthorID: row.AuthorID, Timestamp: row.Timestamp, URL: fmt.Sprintf("https://discord.com/channels/%s/%s/%s", row.GuildID, row.ChannelID, row.ID), ReplyToID: row.ReplyToID, Truncated: truncated})
	}
	return out
}

func Context(ctx context.Context, client *ent.Client, id string) (*RecentMessagesOutput, error) {
	query, err := scopedQuery(ctx, client)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	anchor, err := query.Where(discordmessage.ID(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("message not found in permitted recorded history")
	}
	if err != nil {
		return nil, err
	}
	get := func(p predicate.DiscordMessage, descending bool) ([]*ent.DiscordMessage, error) {
		q, err := scopedQuery(ctx, client)
		if err != nil {
			return nil, err
		}
		q.Where(discordmessage.ChannelID(anchor.ChannelID), p).WithUser().Limit(8)
		if descending {
			q.Order(ent.Desc(discordmessage.FieldTimestamp), ent.Desc(discordmessage.FieldID))
		} else {
			q.Order(ent.Asc(discordmessage.FieldTimestamp), ent.Asc(discordmessage.FieldID))
		}
		return q.All(ctx)
	}
	previous, err := get(discordmessage.Or(discordmessage.TimestampLT(anchor.Timestamp), discordmessage.And(discordmessage.TimestampEQ(anchor.Timestamp), discordmessage.IDLTE(id))), true)
	if err != nil {
		return nil, err
	}
	next, err := get(discordmessage.Or(discordmessage.TimestampGT(anchor.Timestamp), discordmessage.And(discordmessage.TimestampEQ(anchor.Timestamp), discordmessage.IDGT(id))), false)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(previous)-1; i < j; i, j = i+1, j-1 {
		previous[i], previous[j] = previous[j], previous[i]
	}
	rows := append(previous, next...)
	// Include the direct reply parent even if it is outside the nearby window.
	if anchor.ReplyToID != "" {
		found := false
		for _, row := range rows {
			if row.ID == anchor.ReplyToID {
				found = true
			}
		}
		if !found {
			q, _ := scopedQuery(ctx, client)
			parent, err := q.Where(discordmessage.ChannelID(anchor.ChannelID), discordmessage.ID(anchor.ReplyToID)).WithUser().Only(ctx)
			if err != nil && !ent.IsNotFound(err) {
				return nil, err
			}
			if err == nil {
				rows = append([]*ent.DiscordMessage{parent}, rows...)
			}
		}
	}
	return pack(rows, false), nil
}

func DefineRecentMessagesTool(g *genkit.Genkit, client *ent.Client) ai.Tool {
	return genkit.DefineTool(g, "recent_messages", "Read recent recorded messages in the current channel, in chronological order. Maximum 40 messages.", func(ctx *ai.ToolContext, input RecentMessagesInput) (*RecentMessagesOutput, error) {
		scope, _ := ctx.Value(scopeKey{}).(Scope)
		return Search(ctx, client, SearchInput{Limit: input.Limit, ChannelID: scope.ChannelID})
	})
}
func DefineSearchTool(g *genkit.Genkit, client *ent.Client) ai.Tool {
	return genkit.DefineTool(g, "search_history", "Search recorded history across permitted channels in this server by literal keywords, author and dates. Search short keywords, retry synonyms if empty. Results include source links; use message_context before drawing conclusions from isolated remarks.", func(ctx *ai.ToolContext, input SearchInput) (*RecentMessagesOutput, error) {
		return Search(ctx, client, input)
	})
}
func DefineContextTool(g *genkit.Genkit, client *ent.Client) ai.Tool {
	return genkit.DefineTool(g, "message_context", "Read surrounding messages and the recorded reply parent for a message ID returned by history search, within that message's source channel.", func(ctx *ai.ToolContext, input ContextInput) (*RecentMessagesOutput, error) {
		return Context(ctx, client, input.MessageID)
	})
}
