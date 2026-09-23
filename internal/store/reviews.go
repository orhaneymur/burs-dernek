package store

import (
	"context"
	"database/sql"

	"github.com/lafed/burs/internal/model"
)

// SaveReview komisyon uyesinin puanini ekler veya gunceller.
func (s *Store) SaveReview(ctx context.Context, appID, reviewerID uint32, score float64, notes string) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO reviews (application_id, reviewer_id, score, notes) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE score = VALUES(score), notes = VALUES(notes)`,
		appID, reviewerID, score, notes)
	return err
}

func (s *Store) ReviewsFor(ctx context.Context, appID uint32) ([]*model.Review, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT r.id, r.application_id, r.reviewer_id, u.full_name, r.score, COALESCE(r.notes,''), r.created_at, r.updated_at
		 FROM reviews r JOIN users u ON u.id = r.reviewer_id
		 WHERE r.application_id = ? ORDER BY r.updated_at DESC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Review
	for rows.Next() {
		var r model.Review
		if err := rows.Scan(&r.ID, &r.ApplicationID, &r.ReviewerID, &r.ReviewerName,
			&r.Score, &r.Notes, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

func (s *Store) ReviewBy(ctx context.Context, appID, reviewerID uint32) (*model.Review, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT r.id, r.application_id, r.reviewer_id, u.full_name, r.score, COALESCE(r.notes,''), r.created_at, r.updated_at
		 FROM reviews r JOIN users u ON u.id = r.reviewer_id
		 WHERE r.application_id = ? AND r.reviewer_id = ?`, appID, reviewerID)
	var r model.Review
	err := row.Scan(&r.ID, &r.ApplicationID, &r.ReviewerID, &r.ReviewerName,
		&r.Score, &r.Notes, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &r, err
}

// AverageReview bir basvurunun komisyon ortalamasini dondurur.
func (s *Store) AverageReview(ctx context.Context, appID uint32) (sql.NullFloat64, int, error) {
	var avg sql.NullFloat64
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT AVG(score), COUNT(*) FROM reviews WHERE application_id = ?`, appID).Scan(&avg, &n)
	return avg, n, err
}

func (s *Store) DeleteReview(ctx context.Context, appID, reviewerID uint32) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM reviews WHERE application_id = ? AND reviewer_id = ?`, appID, reviewerID)
	return err
}

// PendingForReviewer komisyon uyesinin henuz puanlamadigi basvurulari sayar.
func (s *Store) PendingForReviewer(ctx context.Context, periodID, reviewerID uint32) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM applications a
		 WHERE a.period_id = ? AND a.status <> 'taslak'
		   AND NOT EXISTS (SELECT 1 FROM reviews r WHERE r.application_id = a.id AND r.reviewer_id = ?)`,
		periodID, reviewerID).Scan(&n)
	return n, err
}

// AuditTail son denetim kayitlarini getirir.
func (s *Store) AuditTail(ctx context.Context, limit int) ([]*model.AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, actor, action, entity, entity_id, COALESCE(detail,''), ip, created_at
		 FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.AuditEntry
	for rows.Next() {
		var e model.AuditEntry
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Entity, &e.EntityID, &e.Detail, &e.IP, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
