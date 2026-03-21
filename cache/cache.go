package cache

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Entry struct {
	URL        string
	HTML       string
	Markdown   string
	CreatedAt  time.Time
	AccessedAt time.Time
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS cache (
			url TEXT PRIMARY KEY,
			html TEXT NOT NULL,
			markdown TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			accessed_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Get(url string) (*Entry, bool, error) {
	var e Entry
	err := s.db.QueryRow(
		`SELECT url, html, markdown, created_at, accessed_at FROM cache WHERE url = ?`, url,
	).Scan(&e.URL, &e.HTML, &e.Markdown, &e.CreatedAt, &e.AccessedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	s.db.Exec(`UPDATE cache SET accessed_at = datetime('now') WHERE url = ?`, url)
	return &e, true, nil
}

func (s *Store) Set(url, html, markdown string) error {
	_, err := s.db.Exec(`
		INSERT INTO cache (url, html, markdown, created_at, accessed_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(url) DO UPDATE SET
			html = excluded.html,
			markdown = excluded.markdown,
			accessed_at = datetime('now')
	`, url, html, markdown)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
