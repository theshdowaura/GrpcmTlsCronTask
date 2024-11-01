// internal/db/sqlite.go

package db

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteDB struct {
	DB *sql.DB
}

func NewSQLiteDB(dbPath string) (*SQLiteDB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY,
		cron_expression TEXT NOT NULL,
		command TEXT NOT NULL,
		next_run TEXT,
		status TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(createTableQuery)
	if err != nil {
		return nil, err
	}

	return &SQLiteDB{DB: db}, nil
}

func (s *SQLiteDB) Close() error {
	return s.DB.Close()
}
