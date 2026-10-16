// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original local-file validation and shared connection policy.

// Package sqlitestore owns SQLite file and physical-connection policy.
package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	gosqlsqlite "github.com/italypaleale/go-sql-utils/sqlite"
	"github.com/libtnb/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

// Path resolves the only supported database URL without opening database files.
func Path(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "file" || u.Host != "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("DATABASE_URL must be a local file: URL; start with an empty SQLite database")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("invalid database URL options")
	}
	for k, values := range q {
		if len(values) != 1 || !((k == "mode" && values[0] == "rwc") || (k == "cache" && values[0] == "private")) {
			return "", fmt.Errorf("unsupported database URL option: %s", k)
		}
	}
	name := u.Path
	if u.Opaque != "" {
		name, err = url.PathUnescape(u.Opaque)
		if err != nil {
			return "", err
		}
	}
	if name == "" || name == ":memory:" || strings.ContainsRune(name, 0) {
		return "", errors.New("database must be a writable local file")
	}
	name, err = filepath.Abs(name)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return "", fmt.Errorf("create database directory: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(name))
	if err != nil {
		return "", err
	}
	name = filepath.Join(parent, filepath.Base(name))
	if err = ValidateFile(name); err != nil {
		return "", err
	}
	if err = checkLocalDirectory(parent, gosqlsqlite.IsNetworkedFileSystem); err != nil {
		return "", err
	}
	return name, nil
}

func checkLocalDirectory(path string, detect func(string) (bool, error)) error {
	network, err := detect(path)
	if err != nil {
		return fmt.Errorf("verify local database storage: %w", err)
	}
	if network {
		return errors.New("database requires local persistent storage; network and FUSE filesystems are unsupported")
	}
	return nil
}

func ValidateFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || hasMultipleLinks(info) {
		return errors.New("database and lock files must be regular files without symlink or hard-link aliases")
	}
	return nil
}

func FrancisPath(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + ".francis.db"
	}
	return strings.TrimSuffix(path, ext) + ".francis" + ext
}

func URL(path string) string { return (&url.URL{Scheme: "file", Path: path}).String() }

// Open configures every replacement connection through the driver's DSN.
// Francis deliberately omits hub time options: its STRICT integers are milliseconds.
func Open(ctx context.Context, path string, hub, readOnly bool) (*sql.DB, error) {
	if err := ValidateFile(path); err != nil {
		return nil, err
	}
	if err := checkLocalDirectory(filepath.Dir(path), gosqlsqlite.IsNetworkedFileSystem); err != nil {
		return nil, err
	}
	if !readOnly {
		// Create only a missing, unopened file; never open live SQLite files with os.Open.
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			err = f.Close()
		} else if errors.Is(err, os.ErrExist) {
			err = nil
		}
		if err != nil {
			return nil, err
		}
		if err = os.Chmod(path, 0600); err != nil {
			return nil, err
		}
	}
	u, _ := url.Parse(URL(path))
	q := u.Query()
	q.Set("mode", "rwc")
	if readOnly {
		q.Set("mode", "ro")
	}
	q.Set("cache", "private")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(2500)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Add("_pragma", "wal_autocheckpoint(1000)")
	if !readOnly {
		q.Add("_pragma", "journal_mode(WAL)")
		q.Set("_txlock", "immediate")
	}
	if hub {
		q.Set("_time_integer_format", "unix_micro")
		q.Set("_inttotime", "1")
	}
	u.RawQuery = q.Encode()
	pool, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	if err = Verify(ctx, pool); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return pool, nil
}

func Verify(ctx context.Context, pool *sql.DB) error {
	for _, setting := range []struct {
		name string
		want int
	}{{"foreign_keys", 1}, {"busy_timeout", 2500}, {"synchronous", 2}, {"wal_autocheckpoint", 1000}} {
		var value int
		if err := pool.QueryRowContext(ctx, "PRAGMA "+setting.name).Scan(&value); err != nil {
			return err
		}
		if value != setting.want {
			return fmt.Errorf("SQLite connection policy %s was not established", setting.name)
		}
	}
	var mode string
	if err := pool.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return err
	}
	if mode != "wal" {
		return errors.New("SQLite WAL mode was not established")
	}
	return nil
}

func Dialector(pool *sql.DB) gorm.Dialector { return sqlite.New(sqlite.Config{Conn: pool}) }
