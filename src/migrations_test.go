package cloud

import "testing"

func TestEmbeddedMigrationsAreOrderedAndNonEmpty(t *testing.T) {
	migrations, err := embeddedMigrations()
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	if len(migrations) < 2 {
		t.Fatalf("expected baseline and upgrade migrations, got %d", len(migrations))
	}
	for index, migration := range migrations {
		if migration.version < 1 || migration.contents == "" || len(migration.checksum) != 64 {
			t.Fatalf("invalid migration: %#v", migration)
		}
		if index > 0 && migrations[index-1].version >= migration.version {
			t.Fatalf("migrations out of order: %d then %d", migrations[index-1].version, migration.version)
		}
	}
}

func TestSplitMigrationSQLSkipsComments(t *testing.T) {
	items := splitMigrationSQL("-- title\nCREATE TABLE one (id INT);\n\nALTER TABLE one ADD COLUMN name TEXT;")
	if len(items) != 2 || items[0] != "CREATE TABLE one (id INT)" {
		t.Fatalf("unexpected statements: %#v", items)
	}
}
