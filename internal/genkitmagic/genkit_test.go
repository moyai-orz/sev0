package genkitmagic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInitRequiresOpenRouterKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "  ")
	if _, err := Init(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("expected missing-key error, got %v", err)
	}
}

func TestOpenRouterGenerationWithTools(t *testing.T) {
	for _, model := range []string{"", "  ", "google/gemma-4-26b-a4b-it:free"} {
		t.Run("model="+model, func(t *testing.T) {
			t.Setenv("OPENROUTER_API_KEY", "test-router-key")
			t.Setenv("OPENROUTER_MODEL", model)
			t.Setenv("OPENAI_API_KEY", "")
			t.Setenv("GEMINI_API_KEY", "")
			expected := strings.TrimSpace(model)
			if expected == "" {
				expected = defaultModel
			}
			calls := 0
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })
			http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://openrouter.ai/api/v1/chat/completions" {
					t.Errorf("unexpected endpoint: %s", r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer test-router-key" {
					t.Error("wrong API key")
				}
				var body struct {
					Model    string            `json:"model"`
					Tools    []json.RawMessage `json:"tools"`
					Messages []struct {
						Role    string `json:"role"`
						Content any    `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.Model != expected {
					t.Errorf("model = %q, want %q", body.Model, expected)
				}
				if len(body.Tools) != 2 {
					t.Errorf("tools = %d, want 2", len(body.Tools))
				}
				response := `{"id":"chat-test","object":"chat.completion","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"probe","arguments":"{}"}}]}}]}`
				if calls == 2 {
					found := false
					for _, m := range body.Messages {
						if m.Role == "tool" && strings.Contains(m.Content.(string), "tool worked") {
							found = true
						}
					}
					if !found {
						t.Error("tool result missing from follow-up request")
					}
					response = `{"id":"chat-test-2","object":"chat.completion","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Hello from OpenRouter"}}]}`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
			})
			gm, err := Init(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			probe := genkit.DefineTool(gm.G, "probe", "A test tool", func(ctx *ai.ToolContext, input struct{}) (string, error) { return "tool worked", nil })
			got, err := genkit.GenerateText(context.Background(), gm.G, ai.WithPrompt("Hello"), ai.WithTools(probe, gm.RecentMessagesTool))
			if err != nil {
				t.Fatal(err)
			}
			if got != "Hello from OpenRouter" || calls != 2 {
				t.Fatalf("got %q after %d calls", got, calls)
			}
		})
	}
}
