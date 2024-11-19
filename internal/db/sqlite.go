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

	// 创建 tasks 表
	createTasksTableQuery := `
	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY,
		cron_expression TEXT NOT NULL,
		command TEXT NOT NULL,
		output TEXT,
		status TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(createTasksTableQuery)
	if err != nil {
		return nil, err
	}

	// 创建 task_execution_logs 表
	createTaskExecutionLogsTableQuery := `
	CREATE TABLE IF NOT EXISTS task_execution_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL,
		command TEXT NOT NULL,
		output TEXT,
		start_time TEXT,
		end_time TEXT,
		status TEXT,
		error TEXT,
		FOREIGN KEY (task_id) REFERENCES tasks (id)
	);
	`
	_, err = db.Exec(createTaskExecutionLogsTableQuery)
	if err != nil {
		return nil, err
	}

	return &SQLiteDB{DB: db}, nil
}

func (s *SQLiteDB) Close() error {
	return s.DB.Close()
}
