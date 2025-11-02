package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"sev0/ent"
	"sev0/internal/discord"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		slog.Error("failed loading *.env file")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("unable to connect to db", "err", err)
		return
	}
	drv := entsql.OpenDB(dialect.Postgres, db)

	entClient := ent.NewClient(ent.Driver(drv))

	// Run the auto migration tool.
	if err := entClient.Schema.Create(context.Background()); err != nil {
		slog.Error("failed creating schema resources", "err", err)
		return
	}

	bot, err := discord.NewDiscordBot(entClient)
	if err != nil {
		slog.Error("failed to create discord bot", "err", err)
		return
	}

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
