# sev0

A Discord bot with a `/ask` command and a recent-message history tool.

## Configuration

Copy `.env.example` to `.env` and set your Discord credentials, database URL,
and `OPENROUTER_API_KEY` from https://openrouter.ai/settings/keys.
Existing deployments should add `OPENROUTER_API_KEY`; Gemini and OpenAI keys are
no longer needed. The unused OpenAI embedding client has been removed.

Replies default to [`google/gemma-4-31b-it:free`](https://openrouter.ai/google/gemma-4-31b-it:free)
through OpenRouter's OpenAI-compatible API. This model supports tool calling,
so the bot can still retrieve recent messages. Free endpoints have rate limits
and availability can change; there is no automatic paid-model fallback.

Optionally set `OPENROUTER_MODEL` to another OpenRouter model ID that supports
tool calling (for example, another `:free` model). Use the raw model ID without
an `openrouter/` prefix. A blank value uses the default above. Selecting a paid
model incurs that model's charges.

## Run

With Go 1.25.3 or newer and PostgreSQL with pgvector available:

```sh
docker compose up -d db
go run ./cmd/sev0
```

The existing PostHog integration uses `POSTHOG_KEY` when configured.

## Check

```sh
go test ./...
go vet ./...
```
