package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/qzq-kiim/shop/internal/auth"
	"github.com/qzq-kiim/shop/internal/config"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
)

// minPasswordRunes counts runes, not bytes: an admin password may be cyrillic.
const minPasswordRunes = 12

func adminPassword(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: app admin-password <login>")
	}
	login := strings.TrimSpace(args[0])
	if login == "" {
		return fmt.Errorf("login must not be empty")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.Env)

	password, err := readPassword()
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(password) < minPasswordRunes {
		return fmt.Errorf("password must be at least %d characters", minPasswordRunes)
	}

	hash, err := auth.Hash(password)
	if err != nil {
		return err
	}

	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	id, err := postgres.NewAdminRepo(store).Upsert(ctx, login, hash)
	if err != nil {
		return err
	}
	log.Info("admin password set", slog.String("login", login), slog.String("id", id.String()))
	return nil
}

// readPassword takes the password from the terminal without echoing it, and
// from stdin when there is no terminal (CI, docker exec with a pipe). It is
// never taken from an argument, which would land in the shell history.
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, "password: ")
		raw, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return string(raw), nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return "", fmt.Errorf("no password on stdin")
	}
	return strings.TrimRight(scanner.Text(), "\r\n"), nil
}
