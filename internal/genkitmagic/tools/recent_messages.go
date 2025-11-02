package tools

import (
	"time"

	"sev0/ent"
	"sev0/ent/discordmessage"

	"entgo.io/ent/dialect/sql"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/samber/lo"
)

type RecentMessagesInput struct {
	Limit int `json:"limit,omitempty"`
}

type RecentMessagesOutput struct {
	Messages []Message `json:"messages"`
}

type Message struct {
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	Timestamp time.Time `json:"timestamp"`
}

func DefineRecentMessagesTool(
	g *genkit.Genkit,
	entClient *ent.Client,
) ai.Tool {
	return genkit.DefineTool(
		g,
		"recentMessages",
		"Get the recent messages, it also includes author's name and message timestamp",
		func(ctx *ai.ToolContext, input RecentMessagesInput) (*RecentMessagesOutput, error) {
			messages, err := entClient.DiscordMessage.Query().
				Order(discordmessage.ByTimestamp(sql.OrderDesc())).
				Limit(input.Limit).WithUser().All(ctx)
			if err != nil {
				return nil, err
			}

			output := lo.Map(
				messages,
				(func(item *ent.DiscordMessage, index int) Message {
					return Message{
						Content:   item.Content,
						Author:    item.Edges.User.GlobalName,
						Timestamp: item.Timestamp,
					}
				}))

			return &RecentMessagesOutput{
					Messages: output,
				},
				nil
		},
	)
}
