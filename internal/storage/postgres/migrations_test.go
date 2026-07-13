package postgres

import "testing"

func TestMigrationSection(t *testing.T) {
	content := `-- +migrate Up
CREATE TABLE example (id BIGINT);
-- +migrate Down
DROP TABLE example;`

	up, err := migrationSection(content, MigrationUp)
	if err != nil {
		t.Fatalf("migrationSection(up) error = %v", err)
	}
	if up != "CREATE TABLE example (id BIGINT);" {
		t.Fatalf("up = %q", up)
	}

	down, err := migrationSection(content, MigrationDown)
	if err != nil {
		t.Fatalf("migrationSection(down) error = %v", err)
	}
	if down != "DROP TABLE example;" {
		t.Fatalf("down = %q", down)
	}
}
