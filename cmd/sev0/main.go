package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"sev0/ent"
	"sev0/internal/discord"
	"sev0/internal/genkitmagic"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/posthog/posthog-go"
)

func main() {
	ctx := context.Background()

	err := godotenv.Load()
	if err != nil {
		slog.Error("failed loading *.env file")
	}

	phc, _ := posthog.NewWithConfig(
		os.Getenv("POSTHOG_KEY"),
		posthog.Config{Endpoint: "https://us.i.posthog.com"},
	)
	defer phc.Close()

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("unable to connect to db", "err", err)
		return
	}

	// Enable pgvector extension
	if _, err := db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector;"); err != nil {
		slog.Error("failed to enable pgvector extension", "err", err)
		return
	}
	drv := entsql.OpenDB(dialect.Postgres, db)
	entClient := ent.NewClient(ent.Driver(drv))

	gm, err := genkitmagic.Init(ctx, entClient)
	if err != nil {
		slog.Error("failed to initialize genkit", "err", err)
		return
	}

	embedder := gm.OAI.Embedder(gm.G, "text-embedding-3-small")

	// Run the auto migration tool.
	if err := entClient.Schema.Create(context.Background()); err != nil {
		slog.Error("failed creating schema resources", "err", err)
		return
	}

	bot, err := discord.NewDiscordBot(entClient, embedder, gm, phc)
	if err != nil {
		slog.Error("failed to create discord bot", "err", err)
		return
	}

	go startHTTPServer()

	if err := bot.Start(); err != nil {
		slog.Error("failed to start discord bot", "err", err)
		return
	}
	defer bot.Close()

	slog.Info("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func startHTTPServer() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	slog.Info("starting http server", "port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		slog.Error("failed to start http server", "err", err)
	}
}
