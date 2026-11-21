package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// openDB opens the shared SQLite database. WAL lets the Go and Python processes read concurrently;
// every transaction is BEGIN IMMEDIATE (_txlock) so writers queue on busy_timeout instead of failing.
func openDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return db, db.Ping()
}

var stmtSplit = regexp.MustCompile(`;[ \t]*(\r?\n|$)`)

// splitSQL splits a migration file into statements (";" at the end of a line), dropping comment-only chunks.
// The Python migrator uses the same rule.
func splitSQL(src string) []string {
	var out []string
	for _, chunk := range stmtSplit.Split(src, -1) {
		var lines []string
		for _, ln := range strings.Split(chunk, "\n") {
			if t := strings.TrimSpace(ln); t != "" && !strings.HasPrefix(t, "--") {
				lines = append(lines, ln)
			}
		}
		if s := strings.TrimSpace(strings.Join(lines, "\n")); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// migrate applies numbered migrations from dir, each in its own IMMEDIATE transaction, so two backends
// starting at the same time apply every migration exactly once.
func migrate(ctx context.Context, db *sql.DB, dir string) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no migrations in %s", dir)
	}
	sort.Strings(files)
	for _, f := range files {
		version := strings.TrimSuffix(filepath.Base(f), ".sql")
		src, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := applyMigration(ctx, db, version, string(src)); err != nil {
			return fmt.Errorf("%s: %w", version, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, version, src string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, st := range splitSQL(src) {
		if _, err := tx.ExecContext(ctx, st); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, ts(time.Now())); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- small shared helpers ----

const tsLayout = "2006-01-02T15:04:05.000Z"

func ts(t time.Time) string { return t.UTC().Format(tsLayout) }

const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func newID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return prefix + string(b)
}
