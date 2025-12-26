package db

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

func InitDB(filepath string) *sql.DB {
	db, err := sql.Open("sqlite", filepath)
	if err != nil {
		log.Fatal(err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	// Enable WAL mode for concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("Failed to enable WAL mode: %v", err)
	}

	createTables(db)
	return db
}

func createTables(db *sql.DB) {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			created_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			public_key TEXT NOT NULL,
			name TEXT,
			last_seen DATETIME,
			ip TEXT,
			online INTEGER,
			claim_token TEXT,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS files (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			path TEXT NOT NULL,
			size INTEGER,
			hash TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		// ... existing chunk tables ...
		`CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			file_id TEXT,
			sequence INTEGER,
			hash TEXT,
			size INTEGER,
			FOREIGN KEY(file_id) REFERENCES files(id)
		);`,
		`CREATE TABLE IF NOT EXISTS chunk_locations (
			chunk_id TEXT,
			device_id TEXT,
			PRIMARY KEY (chunk_id, device_id),
			FOREIGN KEY(chunk_id) REFERENCES chunks(id),
			FOREIGN KEY(device_id) REFERENCES devices(id)
		);`,
		`CREATE TABLE IF NOT EXISTS deleted_files (
			id TEXT PRIMARY KEY, /* file_id usually */
			file_id TEXT,
			chunk_ids TEXT, /* JSON array */
			deleted_at DATETIME
		);`,
	}

	for _, query := range queries {
		_, err := db.Exec(query)
		if err != nil {
			// Log but don't fatal on "table exists" or duplicate column if we were smart,
			// but for now let's just log.Fatal as originally designed for fresh run.
			// OR Better: swallow error if "already exists"?
			log.Printf("DB Init Warning/Error: %v", err)
		}
	}

	// Migrations for existing tables (Idempotent-ish)
	// We add columns if they are missing.
	// SQLite: `ALTER TABLE devices ADD COLUMN user_id TEXT;`
	// We run these and ignore "duplicate column" errors.
	migrations := []string{
		"ALTER TABLE devices ADD COLUMN user_id TEXT",
		"ALTER TABLE devices ADD COLUMN claim_token TEXT",
		"ALTER TABLE files ADD COLUMN user_id TEXT",
	}
	for _, m := range migrations {
		db.Exec(m) // Ignore errors
	}
}
