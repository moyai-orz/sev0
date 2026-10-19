package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

func TestNumericIDsRequestCorrectionWithoutRunningQuery(t *testing.T) {
	for _, field := range []string{"channel_id", "before_id", "author"} {
		t.Run(field, func(t *testing.T) {
			g := genkit.Init(context.Background())
			called := false
			tool := defineHistoryTool(g, "test", "test", func(_ *ai.ToolContext, input SearchInput) (*RecentMessagesOutput, error) {
				called = true
				if field == "channel_id" && input.ChannelID != "1561569042278649936" {
					t.Fatalf("ID changed: %s", input.ChannelID)
				}
				return &RecentMessagesOutput{}, nil
			})
			result, err := tool.RunRaw(context.Background(), map[string]any{field: float64(1561569042278649936)})
			data, _ := json.Marshal(result)
			if err != nil || called || !strings.Contains(string(data), "No search was performed") {
				t.Fatalf("numeric ID executed or aborted: %s %v", data, err)
			}
			_, err = tool.RunRaw(context.Background(), map[string]any{field: "1561569042278649936"})
			if err != nil || !called {
				t.Fatalf("string ID failed: %v", err)
			}
		})
	}
}
