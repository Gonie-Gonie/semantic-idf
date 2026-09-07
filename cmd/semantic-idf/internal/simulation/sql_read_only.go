package simulation

import (
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Read-only mode prevents an absent/deleted SQL path from becoming a new empty
// database. Do not use immutable=1: legitimate WAL observations must be read.
func openSimulationSQLiteReadOnly(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("simulation SQL is not a regular file")
	}
	// SQLite mode=ro can still create or update shared-memory files for WAL.
	// Refuse that live/snapshot-ambiguous state before opening the database;
	// never checkpoint it here, and never hide pending observations with immutable.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(absolute + suffix); err == nil {
			return nil, fmt.Errorf("simulation SQL has an active or uncheckpointed %s sidecar; finish and close the simulation, then checkpoint/close the database before reading it", suffix)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	file, err := os.Open(absolute)
	if err != nil {
		return nil, err
	}
	header := make([]byte, 20)
	_, headerErr := io.ReadFull(file, header)
	file.Close()
	// Even a clean, closed WAL-mode database can recreate shared-memory files
	// on its next reader. Require a checkpointed non-WAL snapshot instead.
	if headerErr == nil && string(header[:16]) == "SQLite format 3\x00" && (header[18] == 2 || header[19] == 2) {
		return nil, fmt.Errorf("simulation SQL is in WAL mode; finish and close the simulation, then provide a checkpointed non-WAL database snapshot")
	}
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro&_pragma=query_only%281%29"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
