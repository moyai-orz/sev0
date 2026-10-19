package genkitmagic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"sev0/internal/genkitmagic/tools"
	"sev0/internal/testutil"
)

// Exercise Genkit's actual tool loop and context propagation, not just the
// search function, without sending Discord history to an external provider.
func TestHistoryToolScopeThroughModel(t *testing.T) {
	for _, tool := range []string{"search_history", "recent_messages", "numeric_channel"} {
		t.Run(tool, func(t *testing.T) { testHistoryToolScopeThroughModel(t, tool) })
	}
}
func testHistoryToolScopeThroughModel(t *testing.T, tool string) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_MODEL", "")
	c := testutil.Database(t)
	ctx := tools.WithScope(context.Background(), "g", "current", "current", "other")
	c.DiscordUser.Create().SetID("a").SetUsername("alice").SetGlobalName("Alice").SaveX(ctx)
	for _, channel := range []string{"other", "restricted"} {
		c.DiscordMessage.Create().SetID(channel).SetGuildID("g").SetChannelID(channel).SetUserID("a").SetContent("deploy incident in " + channel).SetTimestamp(time.Now()).SaveX(ctx)
	}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	calls := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Tools) != 3 {
			t.Fatalf("missing history tools: %d", len(body.Tools))
		}
		response := `{"id":"one","object":"chat.completion","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call1","type":"function","function":{"name":"search_history","arguments":"{\"query\":\"deploy\"}"}}]}}]}`
		if tool == "recent_messages" {
			response = strings.ReplaceAll(response, "search_history", "recent_messages")
			response = strings.ReplaceAll(response, `\"query\":\"deploy\"`, `\"channel_id\":\"other\"`)
		}

		if tool == "numeric_channel" && calls == 1 {
			response = strings.ReplaceAll(response, `\"query\":\"deploy\"`, `\"channel_id\":1561569042278649936`)
		}
		if tool == "numeric_channel" && calls == 2 {
			found := false
			for _, m := range body.Messages {
				if m.Role == "tool" {
					content, _ := m.Content.(string)
					if strings.Contains(content, "must be a quoted string") && strings.Contains(content, "No search was performed") {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("numeric ID did not produce a recoverable correction")
			}
		}
		if (tool != "numeric_channel" && calls == 2) || (tool == "numeric_channel" && calls == 3) {
			found := false
			for _, m := range body.Messages {
				if m.Role == "tool" {
					content, _ := m.Content.(string)
					if strings.Contains(content, "restricted") {
						t.Fatal("unauthorized message reached model")
					}
					if strings.Contains(content, "https://discord.com/channels/g/other/other") {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("cross-channel evidence missing from model request")
			}
			response = `{"id":"two","object":"chat.completion","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Found the incident."}}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
	})
	gm, err := Init(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := genkit.GenerateText(ctx, gm.G, ai.WithPrompt("Find the deploy incident"), ai.WithTools(gm.RecentMessagesTool, gm.SearchHistoryTool, gm.MessageContextTool), ai.WithMaxTurns(3))
	expectedCalls := 2
	if tool == "numeric_channel" {
		expectedCalls = 3
	}
	if err != nil || got != "Found the incident." || calls != expectedCalls {
		t.Fatalf("tool roundtrip failed: %q %v (%d calls)", got, err, calls)
	}
}
