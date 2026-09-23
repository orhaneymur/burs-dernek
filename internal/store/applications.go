package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lafed/burs/internal/model"
	"github.com/lafed/burs/internal/security"
)

const appCols = `id, period_id, tracking_code, draft_token, status,
	national_id_enc, COALESCE(national_id_hash,''), first_name, last_name, birth_date, gender,
	phone, email, city, district, COALESCE(address,''),
	university, faculty, department, class_year, student_no, education_type, COALESCE(gpa,0), gpa_scale,
	COALESCE(household_income,0), COALESCE(household_size,0), COALESCE(income_per_capita,0),
	father_status, father_job, mother_status, mother_job, owns_property, owns_vehicle,
	housing_type, COALESCE(housing_cost,0),
	sibling_count, student_sibling_count, parents_status, disability, disability_note, martyr_relative,
	other_scholarship, other_scholarship_name, COALESCE(other_scholarship_amount,0),
	COALESCE(volunteer_text,''), sports_club, COALESCE(motivation_text,''),
	kvkk_accepted, kvkk_accepted_at, submit_ip,
	COALESCE(auto_score,0), COALESCE(auto_breakdown,''), COALESCE(committee_score,0),
	COALESCE(final_score,0), COALESCE(rank_no,0), COALESCE(admin_note,''),
	submitted_at, created_at, updated_at`

func (s *Store) scanApp(sc interface{ Scan(...any) error }) (*model.Application, error) {
	var a model.Application
	var enc []byte
	var birth, kvkkAt, submitted sql.NullTime

	err := sc.Scan(&a.ID, &a.PeriodID, &a.TrackingCode, &a.DraftToken, &a.Status,
		&enc, &a.NationalIDHash, &a.FirstName, &a.LastName, &birth, &a.Gender,
		&a.Phone, &a.Email, &a.City, &a.District, &a.Address,
		&a.University, &a.Faculty, &a.Department, &a.ClassYear, &a.StudentNo, &a.EducationType, &a.GPA, &a.GPAScale,
		&a.HouseholdIncome, &a.HouseholdSize, &a.IncomePerCapita,
		&a.FatherStatus, &a.FatherJob, &a.MotherStatus, &a.MotherJob, &a.OwnsProperty, &a.OwnsVehicle,
		&a.HousingType, &a.HousingCost,
		&a.SiblingCount, &a.StudentSiblingCount, &a.ParentsStatus, &a.Disability, &a.DisabilityNote, &a.MartyrRelative,
		&a.OtherScholarship, &a.OtherScholarshipName, &a.OtherScholarshipAmount,
		&a.VolunteerText, &a.SportsClub, &a.MotivationText,
		&a.KVKKAccepted, &kvkkAt, &a.SubmitIP,
		&a.AutoScore, &a.AutoBreakdown, &a.CommitteeScore, &a.FinalScore, &a.RankNo, &a.AdminNote,
		&submitted, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.BirthDate = nullTime(birth)
	a.KVKKAcceptedAt = nullTime(kvkkAt)
	a.SubmittedAt = nullTime(submitted)

	if len(enc) > 0 && s.Keys != nil {
		if tc, err := s.Keys.Decrypt(enc); err == nil {
			a.NationalID = tc
		}
	}
	return &a, nil
}

// CreateDraft yeni bos bir basvuru taslagi acar.
func (s *Store) CreateDraft(ctx context.Context, periodID uint32) (*model.Application, error) {
	for attempt := 0; attempt < 5; attempt++ {
		code := security.TrackingCode()
		token := security.RandomToken(24)
		res, err := s.DB.ExecContext(ctx,
			`INSERT INTO applications (period_id, tracking_code, draft_token, status) VALUES (?, ?, ?, 'taslak')`,
			periodID, code, token)
		if err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				continue // takip kodu cakismasi, yeniden dene
			}
			return nil, err
		}
		id, _ := res.LastInsertId()
		return s.AppByID(ctx, uint32(id))
	}
	return nil, errors.New("takip kodu uretilemedi")
}

func (s *Store) AppByID(ctx context.Context, id uint32) (*model.Application, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+appCols+` FROM applications WHERE id = ?`, id)
	a, err := s.scanApp(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) AppByDraftToken(ctx context.Context, token string) (*model.Application, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	row := s.DB.QueryRowContext(ctx, `SELECT `+appCols+` FROM applications WHERE draft_token = ?`, token)
	a, err := s.scanApp(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) AppByTracking(ctx context.Context, code string) (*model.Application, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	row := s.DB.QueryRowContext(ctx, `SELECT `+appCols+` FROM applications WHERE tracking_code = ?`, code)
	a, err := s.scanApp(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// AppByNationalID mukerrer basvuru kontrolu ve taslaga geri donus icin kullanilir.
func (s *Store) AppByNationalID(ctx context.Context, periodID uint32, nationalID string) (*model.Application, error) {
	hash := s.Keys.Fingerprint(nationalID)
	row := s.DB.QueryRowContext(ctx,
		`SELECT `+appCols+` FROM applications WHERE period_id = ? AND national_id_hash = ?`, periodID, hash)
	a, err := s.scanApp(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// SaveApplication taslak formunun tum alanlarini gunceller.
func (s *Store) SaveApplication(ctx context.Context, a *model.Application) error {
	var enc []byte
	var hash any
	if a.NationalID != "" {
		var err error
		enc, err = s.Keys.Encrypt(a.NationalID)
		if err != nil {
			return err
		}
		hash = s.Keys.Fingerprint(a.NationalID)
	}

	if a.HouseholdSize > 0 {
		a.IncomePerCapita = a.HouseholdIncome / a.HouseholdSize
	} else {
		a.IncomePerCapita = 0
	}

	_, err := s.DB.ExecContext(ctx, `UPDATE applications SET
		national_id_enc = ?, national_id_hash = ?, first_name = ?, last_name = ?, birth_date = ?, gender = ?,
		phone = ?, email = ?, city = ?, district = ?, address = ?,
		university = ?, faculty = ?, department = ?, class_year = ?, student_no = ?, education_type = ?, gpa = ?, gpa_scale = ?,
		household_income = ?, household_size = ?, income_per_capita = ?,
		father_status = ?, father_job = ?, mother_status = ?, mother_job = ?, owns_property = ?, owns_vehicle = ?,
		housing_type = ?, housing_cost = ?,
		sibling_count = ?, student_sibling_count = ?, parents_status = ?, disability = ?, disability_note = ?, martyr_relative = ?,
		other_scholarship = ?, other_scholarship_name = ?, other_scholarship_amount = ?,
		volunteer_text = ?, sports_club = ?, motivation_text = ?,
		kvkk_accepted = ?, kvkk_accepted_at = ?
		WHERE id = ?`,
		enc, hash, a.FirstName, a.LastName, timeOrNil(a.BirthDate), a.Gender,
		a.Phone, a.Email, a.City, a.District, a.Address,
		a.University, a.Faculty, a.Department, a.ClassYear, a.StudentNo, a.EducationType, a.GPA, a.GPAScale,
		a.HouseholdIncome, a.HouseholdSize, a.IncomePerCapita,
		a.FatherStatus, a.FatherJob, a.MotherStatus, a.MotherJob, a.OwnsProperty, a.OwnsVehicle,
		a.HousingType, a.HousingCost,
		a.SiblingCount, a.StudentSiblingCount, a.ParentsStatus, a.Disability, a.DisabilityNote, a.MartyrRelative,
		a.OtherScholarship, a.OtherScholarshipName, a.OtherScholarshipAmount,
		a.VolunteerText, a.SportsClub, a.MotivationText,
		a.KVKKAccepted, timeOrNil(a.KVKKAcceptedAt),
		a.ID)

	if err != nil && strings.Contains(err.Error(), "uq_app_period_tc") {
		return ErrDuplicateApplication
	}
	return err
}

var ErrDuplicateApplication = errors.New("bu T.C. kimlik numarasiyla bu donemde zaten bir basvuru var")

// Submit basvuruyu kesinlestirir ve puanlarini yazar.
func (s *Store) Submit(ctx context.Context, id uint32, ip string, auto float64, breakdown string, final float64) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE applications SET status = 'basvuruldu', submitted_at = NOW(), submit_ip = ?,
			auto_score = ?, auto_breakdown = ?, final_score = ?
		 WHERE id = ? AND status = 'taslak'`,
		ip, auto, breakdown, final, id)
	return err
}

// UpdateScores otomatik/komisyon/nihai puanlari gunceller.
func (s *Store) UpdateScores(ctx context.Context, id uint32, auto float64, breakdown string, committee sql.NullFloat64, final float64) error {
	var c any
	if committee.Valid {
		c = committee.Float64
	}
	_, err := s.DB.ExecContext(ctx,
		`UPDATE applications SET auto_score = ?, auto_breakdown = ?, committee_score = ?, final_score = ? WHERE id = ?`,
		auto, breakdown, c, final, id)
	return err
}

func (s *Store) SetAppStatus(ctx context.Context, id uint32, status string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE applications SET status = ? WHERE id = ?`, status, id)
	return err
}

func (s *Store) SetAdminNote(ctx context.Context, id uint32, note string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE applications SET admin_note = ? WHERE id = ?`, note, id)
	return err
}

func (s *Store) DeleteApp(ctx context.Context, id uint32) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM applications WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------- Listeleme

type Filter struct {
	PeriodID   uint32
	Status     string
	Query      string
	University string
	MinScore   float64
	OnlyFlagged bool
	Sort       string // puan | tarih | ad
	Limit      int
	Offset     int
}

func (s *Store) ListApplications(ctx context.Context, f Filter) ([]*model.Application, int, error) {
	where := []string{"a.period_id = ?"}
	args := []any{f.PeriodID}

	if f.Status != "" {
		where = append(where, "a.status = ?")
		args = append(args, f.Status)
	} else {
		where = append(where, "a.status <> 'taslak'")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, "(a.first_name LIKE ? OR a.last_name LIKE ? OR a.tracking_code LIKE ? OR a.university LIKE ? OR a.email LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like, like, like, like)
	}
	if f.University != "" {
		where = append(where, "a.university = ?")
		args = append(args, f.University)
	}
	if f.MinScore > 0 {
		where = append(where, "a.final_score >= ?")
		args = append(args, f.MinScore)
	}

	clause := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM applications a`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := "a.final_score DESC, a.auto_score DESC, a.submitted_at ASC"
	switch f.Sort {
	case "tarih":
		order = "a.submitted_at DESC"
	case "ad":
		order = "a.first_name ASC, a.last_name ASC"
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT ` + appCols + `,
		(SELECT COUNT(*) FROM reviews r WHERE r.application_id = a.id) AS review_count,
		(SELECT COUNT(*) FROM documents d WHERE d.application_id = a.id) AS doc_count
		FROM applications a` + clause + ` ORDER BY ` + order + fmt.Sprintf(" LIMIT %d OFFSET %d", limit, f.Offset)

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*model.Application
	for rows.Next() {
		a, err := s.scanAppWithCounts(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

// scanAppWithCounts appCols + iki sayac kolonunu okur.
func (s *Store) scanAppWithCounts(rows *sql.Rows) (*model.Application, error) {
	var a model.Application
	var enc []byte
	var birth, kvkkAt, submitted sql.NullTime

	err := rows.Scan(&a.ID, &a.PeriodID, &a.TrackingCode, &a.DraftToken, &a.Status,
		&enc, &a.NationalIDHash, &a.FirstName, &a.LastName, &birth, &a.Gender,
		&a.Phone, &a.Email, &a.City, &a.District, &a.Address,
		&a.University, &a.Faculty, &a.Department, &a.ClassYear, &a.StudentNo, &a.EducationType, &a.GPA, &a.GPAScale,
		&a.HouseholdIncome, &a.HouseholdSize, &a.IncomePerCapita,
		&a.FatherStatus, &a.FatherJob, &a.MotherStatus, &a.MotherJob, &a.OwnsProperty, &a.OwnsVehicle,
		&a.HousingType, &a.HousingCost,
		&a.SiblingCount, &a.StudentSiblingCount, &a.ParentsStatus, &a.Disability, &a.DisabilityNote, &a.MartyrRelative,
		&a.OtherScholarship, &a.OtherScholarshipName, &a.OtherScholarshipAmount,
		&a.VolunteerText, &a.SportsClub, &a.MotivationText,
		&a.KVKKAccepted, &kvkkAt, &a.SubmitIP,
		&a.AutoScore, &a.AutoBreakdown, &a.CommitteeScore, &a.FinalScore, &a.RankNo, &a.AdminNote,
		&submitted, &a.CreatedAt, &a.UpdatedAt,
		&a.ReviewCount, &a.DocCount)
	if err != nil {
		return nil, err
	}
	a.BirthDate = nullTime(birth)
	a.KVKKAcceptedAt = nullTime(kvkkAt)
	a.SubmittedAt = nullTime(submitted)
	if len(enc) > 0 && s.Keys != nil {
		if tc, err := s.Keys.Decrypt(enc); err == nil {
			a.NationalID = tc
		}
	}
	return &a, nil
}

// AllForPeriod disa aktarim ve siralama icin tum basvurulari getirir.
func (s *Store) AllForPeriod(ctx context.Context, periodID uint32, includeDrafts bool) ([]*model.Application, error) {
	q := `SELECT ` + appCols + ` FROM applications WHERE period_id = ?`
	if !includeDrafts {
		q += ` AND status <> 'taslak'`
	}
	q += ` ORDER BY final_score DESC, auto_score DESC, submitted_at ASC`

	rows, err := s.DB.QueryContext(ctx, q, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Application
	for rows.Next() {
		a, err := s.scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Universities filtre kutusunu doldurur.
func (s *Store) Universities(ctx context.Context, periodID uint32) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT DISTINCT university FROM applications WHERE period_id = ? AND university <> '' ORDER BY university`, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetRank siralama numarasini yazar.
func (s *Store) SetRank(ctx context.Context, id uint32, rank int) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE applications SET rank_no = ? WHERE id = ?`, rank, id)
	return err
}
