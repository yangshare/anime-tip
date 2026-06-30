package store

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS animes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			vod_id INTEGER NOT NULL UNIQUE,
			name TEXT NOT NULL,
			cover TEXT NOT NULL DEFAULT '',
			current_remarks TEXT NOT NULL DEFAULT '',
			last_notified_remarks TEXT NOT NULL DEFAULT '',
			last_notified_episode INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		);
	`)
	if err != nil {
		return err
	}

	// 老库补列（幂等）：SQLite 无 ADD COLUMN IF NOT EXISTS，靠捕获 duplicate column 错误忽略。
	_, err = db.Exec(`ALTER TABLE animes ADD COLUMN last_notified_episode INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	return nil
}
