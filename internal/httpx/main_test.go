package httpx_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/qzq-kiim/shop/internal/storage/postgres"
	"github.com/qzq-kiim/shop/migrations"
)

// templateDB is migrated and seeded once, then cloned per test. Postgres copies
// a template database on disk, which is far cheaper than replaying the
// migrations and the demo dataset for every case.
const templateDB = "shop_template"

// pg is the one Postgres this package runs against. Starting a container per
// test used to exhaust Docker long before the suite finished.
var pg struct {
	admin *sql.DB // connected to the maintenance database, creates the clones
	dsn   *url.URL
	err   error // set when no container could be started; every test skips
	seq   atomic.Uint64
	once  sync.Once
}

func TestMain(m *testing.M) {
	shutdown := startPostgres()
	code := m.Run()
	shutdown()
	os.Exit(code)
}

// startPostgres brings up the shared container and prepares the template. A
// failure is recorded rather than fatal: a machine without Docker still runs
// every test that does not need a database.
func startPostgres() func() {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("shop"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("app"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		pg.err = fmt.Errorf("postgres container unavailable: %w", err)
		return func() {}
	}
	terminate := func() { _ = testcontainers.TerminateContainer(container) }

	raw, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pg.err = fmt.Errorf("connection string: %w", err)
		return terminate
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		pg.err = fmt.Errorf("parse dsn: %w", err)
		return terminate
	}
	pg.dsn = parsed

	admin, err := sql.Open("pgx", raw)
	if err != nil {
		pg.err = fmt.Errorf("open maintenance connection: %w", err)
		return terminate
	}
	pg.admin = admin

	if err := prepareTemplate(ctx); err != nil {
		pg.err = err
		return func() { _ = admin.Close(); terminate() }
	}
	return func() { _ = admin.Close(); terminate() }
}

// prepareTemplate migrates and seeds the database every test is cloned from.
// It has to leave no connection behind: Postgres refuses to copy a template
// that anybody is still attached to.
func prepareTemplate(ctx context.Context) error {
	if _, err := pg.admin.ExecContext(ctx, `CREATE DATABASE `+templateDB); err != nil {
		return fmt.Errorf("create template database: %w", err)
	}

	dsn := dsnFor(templateDB)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open template: %w", err)
	}
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		_ = db.Close()
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		_ = db.Close()
		return fmt.Errorf("goose up: %w", err)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close template migrations: %w", err)
	}

	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open template store: %w", err)
	}
	if err := postgres.NewSeeder(store).Run(ctx, time.Now().UTC()); err != nil {
		store.Close()
		return fmt.Errorf("seed template: %w", err)
	}
	store.Close()
	return nil
}

// newTestDatabase clones the template and returns a DSN nobody else writes to,
// so tests stay as isolated as they were with a container each.
func newTestDatabase(t *testing.T) string {
	t.Helper()
	if pg.err != nil {
		t.Skipf("%v", pg.err)
	}

	name := fmt.Sprintf("shop_test_%d", pg.seq.Add(1))
	// The clone is serialised by Postgres itself; the template is never
	// connected to, which is the condition that would make this fail.
	if _, err := pg.admin.ExecContext(t.Context(), fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, templateDB)); err != nil {
		t.Fatalf("clone template into %s: %v", name, err)
	}
	// The database is not dropped: the container carries every clone away when
	// the package finishes, and dropping costs time the suite does not have.
	return dsnFor(name)
}

// dsnFor points the container's connection string at another database.
func dsnFor(name string) string {
	u := *pg.dsn
	u.Path = "/" + name
	return u.String()
}
