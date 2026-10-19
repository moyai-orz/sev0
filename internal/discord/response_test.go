package discord

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/bwmarrin/discordgo"
)

func TestDeliveryLimitsAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		name         string
		failures     []int
		wantCalls    int
		wantOriginal bool
	}{
		{"success", nil, 1, true},
		{"transient", []int{502}, 2, true},
		{"invalid payload", []int{400}, 2, false},
		{"repeated transient", []int{502, 502}, 3, false},
		{"expired interaction", []int{404}, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			original, err := deliverResponse(context.Background(), func(ctx context.Context, content string) error {
				calls++
				if len(utf16.Encode([]rune(content))) > 2000 {
					t.Fatal("oversized reply")
				}
				if calls <= len(tc.failures) {
					return &discordgo.RESTError{Response: &http.Response{StatusCode: tc.failures[calls-1]}, ResponseBody: []byte("failed")}
				}
				if !tc.wantOriginal && content != deliveryFallback {
					t.Fatalf("expected fallback: %q", content)
				}
				return nil
			}, strings.Repeat("😀", 2000))
			if calls != tc.wantCalls || original != tc.wantOriginal {
				t.Fatalf("calls=%d original=%v err=%v", calls, original, err)
			}
			if tc.name == "expired interaction" && err == nil {
				t.Fatal("missing terminal failure")
			}
		})
	}
}
func TestDeliveryFailureAndCancellation(t *testing.T) {
	calls := 0
	original, err := deliverResponse(context.Background(), func(context.Context, string) error { calls++; return errors.New("network unavailable") }, "reply")
	if original || err == nil || calls != 3 {
		t.Fatalf("unbounded or hidden failure: %v %v %d", original, err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = deliverResponse(ctx, func(context.Context, string) error { t.Fatal("called after cancellation"); return nil }, "reply")
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type responseTransport func(*http.Request) (*http.Response, error)

func (f responseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFinishAskUsesFreshContext(t *testing.T) {
	s, err := discordgo.New("Bot test")
	if err != nil {
		t.Fatal(err)
	}
	s.Client = &http.Client{Transport: responseTransport(func(r *http.Request) (*http.Response, error) {
		if r.Context().Err() != nil {
			t.Fatal("delivery context is already canceled")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("delivery has no deadline")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"reply","content":"timeout"}`)), Request: r}, nil
	})}
	b := &DiscordBot{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if !b.finishAsk(s, &discordgo.Interaction{ID: "interaction", AppID: "app", Token: "test"}, "The search timed out.") {
		t.Fatal("failed to deliver timeout notice")
	}
}

func TestPublicDeferralAndPanicCompletion(t *testing.T) {
	s, err := discordgo.New("Bot test")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.Client = &http.Client{Transport: responseTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if calls == 1 {
			if strings.Contains(string(body), `"flags":64`) {
				t.Fatal("reply is ephemeral")
			}
			if !strings.Contains(string(body), `"type":5`) {
				t.Fatal("missing deferred response")
			}
			return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		if !strings.Contains(string(body), "Something interrupted my reply") {
			t.Fatalf("missing terminal panic notice: %s", body)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"reply"}`)), Request: r}, nil
	})}
	// Nil analytics deliberately triggers a failure after deferral.
	b := &DiscordBot{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	b.handleAsk(context.Background(), s, &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{ID: "i", AppID: "app", Token: "test", GuildID: "g", Member: &discordgo.Member{User: &discordgo.User{ID: "u"}}}})
	if calls != 2 {
		t.Fatalf("deferred response left incomplete: %d requests", calls)
	}
}
