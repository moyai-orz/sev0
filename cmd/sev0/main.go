package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"sev0/ent"
	"sev0/internal/contextkeys"
	"sev0/internal/discord"
	"sev0/internal/genkitmagic"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/posthog/posthog-go"
)

func main() {
	ctx := context.Background()

	err := godotenv.Load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("failed loading .env file", "err", err)
		return
	}
	slog.Info("starting sev0")

	phc, err := posthog.NewWithConfig(
		os.Getenv("POSTHOG_KEY"),
		posthog.Config{Endpoint: "https://us.i.posthog.com"},
	)
	if err != nil {
		slog.Error("failed to initialize analytics", "err", err)
		return
	}
	defer phc.Close()

	baseLogHandler := slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelInfo},
	)
	logger := slog.New(posthog.NewSlogCaptureHandler(
		baseLogHandler,
		phc,
		posthog.WithMinCaptureLevel(slog.LevelWarn),
		posthog.WithDistinctIDFn(
			func(ctx context.Context, r slog.Record) string {
				if userID, ok := ctx.Value(contextkeys.UserIDKey).(string); ok {
					return userID
				}
				// Fallback if no user ID is in the context
				return "anonymous"
			},
		),
	))

	logger.Info("connecting to database")
	dbConfig, err := databaseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Error("unable to connect to db", "err", err)
		return
	}

	db := stdlib.OpenDB(*dbConfig)
	defer db.Close()
	dbCtx, cancelDB := context.WithTimeout(ctx, 30*time.Second)
	defer cancelDB()
	if err := db.PingContext(dbCtx); err != nil {
		logger.Error("database connection failed; check DATABASE_URL and database availability", "err", err)
		return
	}
	logger.Info("enabling pgvector extension")
	// Enable pgvector extension within the database startup deadline.
	if _, err := db.ExecContext(dbCtx, "CREATE EXTENSION IF NOT EXISTS vector;"); err != nil {
		logger.Error("failed to enable pgvector extension", "err", err)
		return
	}
	cancelDB()
	drv := entsql.OpenDB(dialect.Postgres, db)
	entClient := ent.NewClient(ent.Driver(drv))

	logger.Info("initializing OpenRouter")
	gm, err := genkitmagic.Init(ctx, entClient)
	if err != nil {
		logger.Error("failed to initialize genkit", "err", err)
		return
	}

	logger.Info("running database migrations")
	migrationCtx, cancelMigration := context.WithTimeout(ctx, 30*time.Second)
	defer cancelMigration()
	if err := entClient.Schema.Create(migrationCtx); err != nil {
		logger.Error("failed creating schema resources", "err", err)
		return
	}

	cancelMigration()
	bot, err := discord.NewDiscordBot(entClient, gm, phc, logger)
	if err != nil {
		logger.Error("failed to create discord bot", "err", err)
		return
	}

	go startHTTPServer(logger)

	logger.Info("connecting to Discord")
	if err := bot.Start(); err != nil {
		logger.Error("failed to start discord bot", "err", err)
		return
	}
	defer bot.Close()

	logger.Info("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func startHTTPServer(logger *slog.Logger) {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info("starting http server", "port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Error("failed to start http server", "err", err)
	}
}

// databaseConfig bounds each address attempt so an unreachable IPv4 address
// cannot prevent pgx from trying the hostname's IPv6 address for minutes.
func databaseConfig(databaseURL string) (*pgx.ConnConfig, error) {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = 5 * time.Second
	}

	// Railway private networking can publish an unreachable IPv4 route alongside
	// working IPv6. Prefer IPv6 without removing IPv4 fallback or caching DNS.
	if strings.HasSuffix(config.Host, ".railway.internal") {
		lookup := config.LookupFunc
		config.LookupFunc = func(ctx context.Context, host string) ([]string, error) {
			addresses, err := lookup(ctx, host)
			if err != nil {
				return nil, err
			}
			return preferIPv6(addresses), nil
		}
	}
	return config, nil
}

func preferIPv6(addresses []string) []string {
	addresses = append([]string(nil), addresses...)
	sort.SliceStable(addresses, func(i, j int) bool {
		a, b := net.ParseIP(addresses[i]), net.ParseIP(addresses[j])
		return a != nil && a.To4() == nil && !(b != nil && b.To4() == nil)
	})
	return addresses
}
