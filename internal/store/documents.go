package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lafed/burs/internal/model"
)

func (s *Store) SaveDocument(ctx context.Context, d *model.Document) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO documents (application_id, kind, original_name, stored_name, mime, size_bytes)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE original_name = VALUES(original_name), stored_name = VALUES(stored_name),
		   mime = VALUES(mime), size_bytes = VALUES(size_bytes), uploaded_at = NOW()`,
		d.ApplicationID, d.Kind, d.OriginalName, d.StoredName, d.Mime, d.SizeBytes)
	return err
}

func (s *Store) DocumentsFor(ctx context.Context, appID uint32) ([]*model.Document, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, application_id, kind, original_name, stored_name, mime, size_bytes, uploaded_at
		 FROM documents WHERE application_id = ? ORDER BY uploaded_at`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Document
	for rows.Next() {
		var d model.Document
		if err := rows.Scan(&d.ID, &d.ApplicationID, &d.Kind, &d.OriginalName,
			&d.StoredName, &d.Mime, &d.SizeBytes, &d.UploadedAt); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}

func (s *Store) DocumentByID(ctx context.Context, id uint32) (*model.Document, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT id, application_id, kind, original_name, stored_name, mime, size_bytes, uploaded_at
		 FROM documents WHERE id = ?`, id)
	var d model.Document
	err := row.Scan(&d.ID, &d.ApplicationID, &d.Kind, &d.OriginalName,
		&d.StoredName, &d.Mime, &d.SizeBytes, &d.UploadedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &d, err
}

func (s *Store) DeleteDocument(ctx context.Context, id uint32) (string, error) {
	d, err := s.DocumentByID(ctx, id)
	if err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM documents WHERE id = ?`, id)
	return d.StoredName, err
}

// DocumentMap belge turune gore haritalar; form ekraninda yuklu olani gostermek icin.
func (s *Store) DocumentMap(ctx context.Context, appID uint32) (map[string]*model.Document, error) {
	docs, err := s.DocumentsFor(ctx, appID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*model.Document, len(docs))
	for _, d := range docs {
		m[d.Kind] = d
	}
	return m, nil
}
