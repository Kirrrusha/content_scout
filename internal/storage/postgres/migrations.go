package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type MigrationDirection string

const (
	MigrationUp   MigrationDirection = "up"
	MigrationDown MigrationDirection = "down"
)

func RunMigrations(ctx context.Context, db *sql.DB, dir string, direction MigrationDirection) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)
	if direction == MigrationDown {
		reverse(files)
	}

	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		applied, err := migrationApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if direction == MigrationUp && applied {
			continue
		}
		if direction == MigrationDown && !applied {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}
		statement, err := migrationSection(string(content), direction)
		if err != nil {
			return fmt.Errorf("parse migration %s: %w", file, err)
		}
		if strings.TrimSpace(statement) == "" {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", version, err)
		}
		if direction == MigrationUp {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
		} else {
			_, err = tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version)
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
	}
	return nil
}

func migrationApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return exists, nil
}

func migrationSection(content string, direction MigrationDirection) (string, error) {
	upMarker := "-- +migrate Up"
	downMarker := "-- +migrate Down"
	up := strings.Index(content, upMarker)
	down := strings.Index(content, downMarker)
	if up == -1 || down == -1 || down < up {
		return "", errors.New("migration must contain up and down sections")
	}
	if direction == MigrationUp {
		return strings.TrimSpace(content[up+len(upMarker) : down]), nil
	}
	return strings.TrimSpace(content[down+len(downMarker):]), nil
}

func reverse(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
