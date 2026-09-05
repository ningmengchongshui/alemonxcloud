package cloud

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type schemaMigration struct {
	version  int
	name     string
	contents string
	checksum string
}

// initializeSchemaMigrations is deliberately the only schema-changing entry
// point. MySQL DDL may auto-commit, so every statement is idempotent and a
// migration is recorded only after all of its statements have succeeded.
func initializeSchemaMigrations(ctx context.Context) error {
	if instanceDB == nil {
		return nil
	}
	if _, err := instanceDB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS xcloud_schema_migrations (version INT PRIMARY KEY, name VARCHAR(255) NOT NULL, checksum CHAR(64) NOT NULL, applied_at DATETIME NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("create migration registry: %w", err)
	}
	migrations, err := embeddedMigrations()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		var checksum string
		err := instanceDB.QueryRowContext(ctx, `SELECT checksum FROM xcloud_schema_migrations WHERE version=?`, migration.version).Scan(&checksum)
		if err == nil {
			if checksum != migration.checksum {
				return fmt.Errorf("migration %03d checksum changed", migration.version)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read migration %03d: %w", migration.version, err)
		}
		for _, statement := range splitMigrationSQL(migration.contents) {
			if _, err := instanceDB.ExecContext(ctx, statement); err != nil && !isDuplicateMigration(err) {
				return fmt.Errorf("apply migration %03d %s: %w", migration.version, migration.name, err)
			}
		}
		if _, err := instanceDB.ExecContext(ctx, `INSERT INTO xcloud_schema_migrations (version,name,checksum,applied_at) VALUES (?,?,?,?)`, migration.version, migration.name, migration.checksum, time.Now()); err != nil {
			return fmt.Errorf("record migration %03d: %w", migration.version, err)
		}
	}
	if err := normalizeImageSources(ctx); err != nil {
		return fmt.Errorf("normalize image sources: %w", err)
	}
	if _, err := instanceDB.ExecContext(ctx, `ALTER TABLE xcloud_images ADD UNIQUE KEY uq_xcloud_image_ref (image_ref)`); err != nil && !isDuplicateMigration(err) {
		return fmt.Errorf("add image source uniqueness: %w", err)
	}
	return removeBootstrapData(ctx)
}

func embeddedMigrations() ([]schemaMigration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	items := make([]schemaMigration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".sql" {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("invalid migration name %q", entry.Name())
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("invalid migration version %q", entry.Name())
		}
		contents, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(contents)
		items = append(items, schemaMigration{version: version, name: entry.Name(), contents: string(contents), checksum: fmt.Sprintf("%x", sum)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].version < items[j].version })
	for index := 1; index < len(items); index++ {
		if items[index-1].version == items[index].version {
			return nil, fmt.Errorf("duplicate migration version %03d", items[index].version)
		}
	}
	return items, nil
}

func splitMigrationSQL(contents string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(contents, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		lines = append(lines, line)
	}
	parts := strings.Split(strings.Join(lines, "\n"), ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		if statement := strings.TrimSpace(part); statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}
