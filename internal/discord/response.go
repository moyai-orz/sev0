package discord

import (
	"context"
	"errors"
	"time"

	"github.com/bwmarrin/discordgo"
)

const deliveryFallback = "I couldn't deliver the full reply. Please try /ask again."

// finishAsk uses a fresh context: a timed-out generation must still be able to
// replace Discord's deferred response. Only successful original replies enter
// conversation memory; a delivered fallback returns false.
func (b *DiscordBot) finishAsk(s *discordgo.Session, i *discordgo.Interaction, content string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	delivered, err := deliverResponse(ctx, func(ctx context.Context, text string) error {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, 5*time.Second)
		defer attemptCancel()
		_, err := s.InteractionResponseEdit(i, &discordgo.WebhookEdit{Content: &text, AllowedMentions: &discordgo.MessageAllowedMentions{}}, discordgo.WithContext(attemptCtx), discordgo.WithRestRetries(0), discordgo.WithRetryOnRatelimit(false))
		return err
	}, content)
	if err != nil {
		b.logger.Error("failed to complete deferred response", "interaction_id", i.ID, "err", err)
	} else if !delivered {
		b.logger.Warn("delivered fallback response", "interaction_id", i.ID)
	} else {
		b.logger.Info("completed ask response", "interaction_id", i.ID)
	}
	return delivered
}

func deliverResponse(ctx context.Context, edit func(context.Context, string) error, content string) (bool, error) {
	content = truncateDiscord(content, 2000)
	if content == "" {
		content = "The model returned an empty reply. Please try again."
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		err := edit(ctx, content)
		if err == nil {
			return true, nil
		}
		var restErr *discordgo.RESTError
		if errors.As(err, &restErr) && restErr.Response != nil {
			status := restErr.Response.StatusCode
			// Unknown/expired interactions and permission errors cannot be repaired by
			// changing content. Other client errors may reject the payload itself.
			if status == 401 || status == 403 || status == 404 {
				return false, err
			}
			if status >= 400 && status < 500 && status != 429 {
				break
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return false, edit(ctx, deliveryFallback)
}
