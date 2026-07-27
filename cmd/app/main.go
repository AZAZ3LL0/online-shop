// Command app is the single binary of the shop: web site, admin panel,
// webhooks and the background worker, plus the operational subcommands.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

const usage = `usage: app <command>

commands:
  serve            run the site, the admin panel, the webhooks and the worker
  migrate          apply database migrations
  seed             fill the database with the demo collection and 30 days of traffic
  admin-password   create or update an administrator password
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1], os.Args[2:]); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, command string, args []string) error {
	switch command {
	case "serve":
		return serve(ctx)
	case "migrate":
		return migrate(ctx)
	case "seed":
		return seed(ctx)
	case "admin-password":
		return adminPassword(ctx, args)
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", command)
	}
}

// newLogger writes JSON in production and human-readable text in development.
func newLogger(env string) *slog.Logger {
	if env == "prod" {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
