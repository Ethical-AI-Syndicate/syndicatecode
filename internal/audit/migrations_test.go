package audit

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestApplyMigrations_Bead_l3d_17_1(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()
	if err := applyMigrations(db); err != nil {
		t.Fatalf("applyMigrations failed: %v", err)
	}
}
