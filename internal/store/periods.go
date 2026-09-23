package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lafed/burs/internal/model"
)

const periodCols = `id, slug, name, COALESCE(description,''), opens_at, closes_at, quota, reserve_quota,
	status, auto_weight, committee_weight, COALESCE(criteria_json,''), require_docs,
	COALESCE(result_note,''), created_at, updated_at`

func scanPeriod(sc interface{ Scan(...any) error }) (*model.Period, error) {
	var p model.Period
	err := sc.Scan(&p.ID, &p.Slug, &p.Name, &p.Description, &p.OpensAt, &p.ClosesAt,
		&p.Quota, &p.ReserveQuota, &p.Status, &p.AutoWeight, &p.CommitteeWeight,
		&p.CriteriaJSON, &p.RequireDocs, &p.ResultNote, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

var ErrNotFound = errors.New("kayit bulunamadi")

func (s *Store) PeriodBySlug(ctx context.Context, slug string) (*model.Period, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+periodCols+` FROM periods WHERE slug = ?`, slug)
	p, err := scanPeriod(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) PeriodByID(ctx context.Context, id uint32) (*model.Period, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+periodCols+` FROM periods WHERE id = ?`, id)
	p, err := scanPeriod(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) Periods(ctx context.Context) ([]*model.Period, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+periodCols+` FROM periods ORDER BY opens_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Period
	for rows.Next() {
		p, err := scanPeriod(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ActivePeriod anasayfada gosterilecek donemi secer: once basvuruya acik olan,
// yoksa sonuclari ilan edilmis en yeni donem.
func (s *Store) ActivePeriod(ctx context.Context) (*model.Period, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+periodCols+` FROM periods
		WHERE status IN ('acik','kapali','ilan')
		ORDER BY FIELD(status,'acik','ilan','kapali'), closes_at DESC LIMIT 1`)
	p, err := scanPeriod(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) CreatePeriod(ctx context.Context, p *model.Period) (uint32, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO periods (slug, name, description, opens_at, closes_at, quota, reserve_quota,
			status, auto_weight, committee_weight, criteria_json, require_docs, result_note)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Slug, p.Name, p.Description, p.OpensAt, p.ClosesAt, p.Quota, p.ReserveQuota,
		p.Status, p.AutoWeight, p.CommitteeWeight, p.CriteriaJSON, p.RequireDocs, p.ResultNote)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint32(id), nil
}

func (s *Store) UpdatePeriod(ctx context.Context, p *model.Period) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE periods SET slug = ?, name = ?, description = ?, opens_at = ?, closes_at = ?,
			quota = ?, reserve_quota = ?, status = ?, auto_weight = ?, committee_weight = ?,
			criteria_json = ?, require_docs = ?, result_note = ? WHERE id = ?`,
		p.Slug, p.Name, p.Description, p.OpensAt, p.ClosesAt, p.Quota, p.ReserveQuota,
		p.Status, p.AutoWeight, p.CommitteeWeight, p.CriteriaJSON, p.RequireDocs, p.ResultNote, p.ID)
	return err
}

func (s *Store) SetPeriodStatus(ctx context.Context, id uint32, status string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE periods SET status = ? WHERE id = ?`, status, id)
	return err
}

// PeriodStats yonetim panelindeki ozet kartlari besler.
type PeriodStats struct {
	Drafts     int
	Submitted  int
	Reviewing  int
	Accepted   int
	Reserve    int
	Rejected   int
	Total      int
	Reviewed   int // en az bir komisyon puani almis basvuru
	AvgFinal   float64
}

func (s *Store) PeriodStats(ctx context.Context, periodID uint32) (PeriodStats, error) {
	var st PeriodStats
	rows, err := s.DB.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM applications WHERE period_id = ? GROUP BY status`, periodID)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return st, err
		}
		switch status {
		case model.StatusDraft:
			st.Drafts = n
		case model.StatusSubmitted:
			st.Submitted = n
		case model.StatusReview:
			st.Reviewing = n
		case model.StatusAccepted:
			st.Accepted = n
		case model.StatusReserve:
			st.Reserve = n
		case model.StatusRejected:
			st.Rejected = n
		}
		if status != model.StatusDraft {
			st.Total += n
		}
	}
	if err := rows.Err(); err != nil {
		return st, err
	}

	_ = s.DB.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT r.application_id) FROM reviews r
		 JOIN applications a ON a.id = r.application_id WHERE a.period_id = ?`, periodID).Scan(&st.Reviewed)

	var avg sql.NullFloat64
	_ = s.DB.QueryRowContext(ctx,
		`SELECT AVG(final_score) FROM applications WHERE period_id = ? AND status <> 'taslak'`, periodID).Scan(&avg)
	if avg.Valid {
		st.AvgFinal = avg.Float64
	}
	return st, nil
}
