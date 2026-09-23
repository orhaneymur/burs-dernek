// Package store veritabani erisim katmanidir (MariaDB / MySQL).
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/lafed/burs/internal/security"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	DB   *sql.DB
	Keys *security.Keyring
}

func Open(dsn string, keys *security.Keyring) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Veritabani pod'u uygulamadan sonra hazir olabilir; kisa sure bekle.
	var lastErr error
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		lastErr = db.PingContext(ctx)
		cancel()
		if lastErr == nil {
			break
		}
		log.Printf("veritabani bekleniyor (%d/30): %v", i+1, lastErr)
		time.Sleep(2 * time.Second)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("veritabanina baglanilamadi: %w", lastErr)
	}

	return &Store{DB: db, Keys: keys}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// Migrate migrations/ altindaki .sql dosyalarini sirayla, bir kez uygular.
func (s *Store) Migrate(ctx context.Context) error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
		name VARCHAR(190) NOT NULL PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	if _, err := s.DB.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("gocmen tablosu olusturulamadi: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&exists)
		if err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("%s uygulanamadi: %w", name, err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
			return err
		}
		log.Printf("gocme uygulandi: %s", name)
	}
	return nil
}

// ---------------------------------------------------------------- Ayarlar

func (s *Store) Setting(ctx context.Context, key, def string) string {
	var v string
	err := s.DB.QueryRowContext(ctx, "SELECT `value` FROM settings WHERE `key` = ?", key).Scan(&v)
	if err != nil || v == "" {
		return def
	}
	return v
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx,
		"INSERT INTO settings (`key`, `value`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `value` = VALUES(`value`)",
		key, value)
	return err
}

// ---------------------------------------------------------------- Denetim kaydi

func (s *Store) Audit(ctx context.Context, actor, action, entity, entityID, detail, ip string) {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO audit_log (actor, action, entity, entity_id, detail, ip) VALUES (?, ?, ?, ?, ?, ?)`,
		actor, action, entity, entityID, detail, ip)
	if err != nil {
		log.Printf("denetim kaydi yazilamadi: %v", err)
	}
}

func nullTime(t sql.NullTime) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

func timeOrNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
