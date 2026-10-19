package main

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatabaseAddressFallback(t *testing.T) {
	config, err := databaseConfig("postgres://user:password@database.test/db?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnectTimeout != 5*time.Second {
		t.Fatalf("unexpected timeout: %v", config.ConnectTimeout)
	}
	// Simulate an unresponsive IPv4 route without using the network.
	config.ConnectTimeout = 10 * time.Millisecond
	config.LookupFunc = func(context.Context, string) ([]string, error) { return []string{"192.0.2.1", "2001:db8::1"}, nil }
	var attempted []string
	config.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		attempted = append(attempted, address)
		if len(attempted) == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if ctx.Err() != nil {
			t.Error("fallback inherited the expired address deadline")
		}
		return nil, errors.New("end of simulated connection attempts")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = pgconn.ConnectConfig(ctx, &config.Config)
	if err == nil {
		t.Fatal("expected simulated connection error")
	}
	if len(attempted) != 2 || attempted[1] != "[2001:db8::1]:5432" {
		t.Fatalf("did not fall back to IPv6: %v", attempted)
	}
}

func TestDatabaseExplicitTimeout(t *testing.T) {
	config, err := databaseConfig("postgres://user:password@database.test/db?connect_timeout=12")
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnectTimeout != 12*time.Second {
		t.Fatalf("explicit timeout overwritten: %v", config.ConnectTimeout)
	}
}
