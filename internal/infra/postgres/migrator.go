package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuroraMigrationAdvisoryLockID is an arbitrary 64-bit int ("AURORAMG") used for advisory locking.
const AuroraMigrationAdvisoryLockID int64 = 0x4155524F52414D47

// Migration represents a single versioned SQL migration.
type Migration struct {
	Version int
	Name    string
	UpSQL   string
	DownSQL string
}

// Migrator handles versioned database migrations for Aurora with advisory locking.
type Migrator struct {
	pool           *pgxpool.Pool
	migrationsPath string
}

// NewMigrator creates a new Migrator instance.
func NewMigrator(pool *pgxpool.Pool, migrationsPath string) *Migrator {
	return &Migrator{
		pool:           pool,
		migrationsPath: migrationsPath,
	}
}

// EnsureSchemaTable creates the schema_migrations tracking table if not present.
func (m *Migrator) EnsureSchemaTable(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	_, err := m.pool.Exec(ctx, query)
	return err
}

// LoadMigrations reads and parses migration files from the migration directory.
func (m *Migrator) LoadMigrations() ([]Migration, error) {
	files, err := os.ReadDir(m.migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations dir: %w", err)
	}

	migrationMap := make(map[int]*Migration)

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}

		parts := strings.SplitN(f.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		if _, exists := migrationMap[version]; !exists {
			migrationMap[version] = &Migration{
				Version: version,
				Name:    f.Name(),
			}
		}

		content, err := os.ReadFile(filepath.Join(m.migrationsPath, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", f.Name(), err)
		}

		if strings.HasSuffix(f.Name(), ".up.sql") {
			migrationMap[version].UpSQL = string(content)
		} else if strings.HasSuffix(f.Name(), ".down.sql") {
			migrationMap[version].DownSQL = string(content)
		}
	}

	var migrations []Migration
	for _, mig := range migrationMap {
		migrations = append(migrations, *mig)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// CurrentVersion returns the highest applied migration version.
func (m *Migrator) CurrentVersion(ctx context.Context) (int, error) {
	if err := m.EnsureSchemaTable(ctx); err != nil {
		return 0, err
	}

	var currentVersion int
	query := `SELECT COALESCE(MAX(version), 0) FROM schema_migrations;`
	err := m.pool.QueryRow(ctx, query).Scan(&currentVersion)
	if err != nil {
		return 0, err
	}

	return currentVersion, nil
}

// AcquireAdvisoryLock acquires a PostgreSQL session-level advisory lock for migrations.
func (m *Migrator) AcquireAdvisoryLock(ctx context.Context) (pgx.Tx, error) {
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to begin lock transaction: %w", err)
	}

	var acquired bool
	err = tx.QueryRow(ctx, `SELECT pg_advisory_xact_lock($1), true;`, AuroraMigrationAdvisoryLockID).Scan(&acquired, &acquired)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("failed to acquire migration advisory lock: %w", err)
	}

	return tx, nil
}

// Up runs all unapplied migrations in ascending order protected by an advisory lock.
func (m *Migrator) Up(ctx context.Context) (int, error) {
	if m.pool == nil {
		return 0, fmt.Errorf("migrator pool is nil")
	}

	// Acquire advisory transaction lock ensuring single-instance migration execution
	lockTx, err := m.AcquireAdvisoryLock(ctx)
	if err != nil {
		return 0, fmt.Errorf("could not obtain migration lock: %w", err)
	}
	defer func() {
		_ = lockTx.Rollback(ctx)
	}()

	if err := m.EnsureSchemaTable(ctx); err != nil {
		return 0, fmt.Errorf("failed to initialize schema table: %w", err)
	}

	currentVersion, err := m.CurrentVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get current version: %w", err)
	}

	migrations, err := m.LoadMigrations()
	if err != nil {
		return 0, fmt.Errorf("failed to load migrations: %w", err)
	}

	appliedCount := 0
	for _, mig := range migrations {
		if mig.Version <= currentVersion {
			continue
		}

		if strings.TrimSpace(mig.UpSQL) == "" {
			return appliedCount, fmt.Errorf("migration %06d missing up sql", mig.Version)
		}

		tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return appliedCount, fmt.Errorf("failed to begin tx for migration %d: %w", mig.Version, err)
		}

		if _, err := tx.Exec(ctx, mig.UpSQL); err != nil {
			_ = tx.Rollback(ctx)
			return appliedCount, fmt.Errorf("failed to apply migration %d (%s): %w", mig.Version, mig.Name, err)
		}

		recordQuery := `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2);`
		if _, err := tx.Exec(ctx, recordQuery, mig.Version, time.Now()); err != nil {
			_ = tx.Rollback(ctx)
			return appliedCount, fmt.Errorf("failed to record migration %d: %w", mig.Version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return appliedCount, fmt.Errorf("failed to commit migration %d: %w", mig.Version, err)
		}

		appliedCount++
	}

	// Commit lock transaction
	_ = lockTx.Commit(ctx)

	return appliedCount, nil
}
