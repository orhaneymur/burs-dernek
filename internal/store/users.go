package store

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/lafed/burs/internal/model"
	"github.com/lafed/burs/internal/security"
)

const userCols = `id, username, full_name, email, password_hash, role, is_active, must_change, last_login_at, created_at`

func scanUser(sc interface{ Scan(...any) error }) (*model.User, error) {
	var u model.User
	var last sql.NullTime
	err := sc.Scan(&u.ID, &u.Username, &u.FullName, &u.Email, &u.PasswordHash,
		&u.Role, &u.IsActive, &u.MustChange, &last, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	u.LastLoginAt = nullTime(last)
	return &u, nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (*model.User, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username = ?`, strings.ToLower(strings.TrimSpace(username)))
	return scanUser(row)
}

func (s *Store) UserByID(ctx context.Context, id uint32) (*model.User, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) Users(ctx context.Context) ([]*model.User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY role, full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, username, fullName, email, password, role string) (uint32, error) {
	hash, err := security.HashPassword(password)
	if err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO users (username, full_name, email, password_hash, role) VALUES (?, ?, ?, ?, ?)`,
		strings.ToLower(strings.TrimSpace(username)), fullName, email, hash, role)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint32(id), nil
}

func (s *Store) UpdateUser(ctx context.Context, id uint32, fullName, email, role string, active bool) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE users SET full_name = ?, email = ?, role = ?, is_active = ? WHERE id = ?`,
		fullName, email, role, active, id)
	return err
}

func (s *Store) SetPassword(ctx context.Context, id uint32, password string) error {
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE users SET password_hash = ?, must_change = 0 WHERE id = ?`, hash, id)
	return err
}

func (s *Store) DeleteUser(ctx context.Context, id uint32) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// EnsureBootstrapAdmin ilk calistirmada yonetici hesabini olusturur.
func (s *Store) EnsureBootstrapAdmin(ctx context.Context, username, password string) error {
	var count int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if password == "" {
		password = security.RandomToken(6)
		log.Printf("!!! Ilk yonetici sifresi uretildi -> kullanici: %s  sifre: %s", username, password)
		log.Printf("!!! Giris yaptiktan sonra sifreyi mutlaka degistirin.")
	}
	id, err := s.CreateUser(ctx, username, "Federasyon Yöneticisi", "", password, "admin")
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE users SET must_change = 1 WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------- Oturumlar

const sessionTTL = 8 * time.Hour

func (s *Store) CreateSession(ctx context.Context, userID uint32, ip, ua string) (string, error) {
	token := security.RandomToken(32)
	if len(ua) > 250 {
		ua = ua[:250]
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, ip, user_agent, expires_at) VALUES (?, ?, ?, ?, ?)`,
		token, userID, ip, ua, time.Now().Add(sessionTTL))
	if err != nil {
		return "", err
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = ?`, userID)
	return token, nil
}

var ErrNoSession = errors.New("oturum bulunamadi")

func (s *Store) UserBySession(ctx context.Context, token string) (*model.User, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	row := s.DB.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.full_name, u.email, u.password_hash, u.role, u.is_active, u.must_change, u.last_login_at, u.created_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ? AND s.expires_at > NOW() AND u.is_active = 1`, token)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
}

func (s *Store) PurgeExpiredSessions(ctx context.Context) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
}
