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

## Conversation history

`/ask` now receives recent messages from the current channel and can search
recorded text across other text/announcement channels in the same server that
the requester can view and read history from. The model can filter by literal
keywords, author, dates, or channel, page through results, and inspect nearby
messages and the direct reply parent. Historical claims are prompted to include
Discord message links, author names, and dates.

Answers are **ephemeral (visible only to the requester)**, because someone asking
in a public channel may also have access to restricted channels. Other threads
are excluded from cross-channel search; the thread where `/ask` was invoked is
included if its history is readable. Permission lookup failures stop retrieval.

Each user gets the last four successful exchanges in the same channel for
follow-up questions. This memory expires after 15 minutes, is capped at 128
conversations, resets on restart, and is discarded if their permitted channel
set changes. This is conversation continuity, not permanent factual memory.

New messages store server/channel IDs and reply references. Edits are refreshed
from Discord, and delete events remove stored messages and their embeddings.
The bot only records text messages it receives while running; there is no bulk
backfill or offline catch-up in this change. Existing rows without channel
metadata remain stored but are excluded from retrieval. Existing rows acquire
metadata if they are subsequently edited and successfully refreshed.

Database migration adds nullable metadata columns and channel/time indexes at
startup. No new credentials, embedding service, or environment variables are
required. Search uses bounded case-insensitive literal keyword matching (all
keywords must match), not semantic/vector search. Query deadlines, row limits,
excerpt budgets, and a six-turn tool budget constrain resource use. Large
archives may eventually warrant a dedicated text-search index.

Tests use an isolated SQLite database for message/user queries and permissions,
with the unused pgvector column represented as text. They do not replace a live
PostgreSQL migration check or a Discord/OpenRouter smoke test. Running tests
requires CGO and a C compiler for the SQLite test driver; the bot itself does not
use SQLite.
