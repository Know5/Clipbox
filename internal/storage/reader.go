package storage

import (
	"database/sql"
	"net/url"
	"path/filepath"
)

// OpenReader is exclusively for UI image delivery. It cannot migrate or write
// application data; all mutations remain in the resident service.
func OpenReader(dataDir string) (*Store, error) {
	path := filepath.Join(dataDir, "clipbox.db")
	uri := url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(path), RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA query_only=ON; PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, dataDir: dataDir}, nil
}
