-- LAFED Burs Sistemi - ilk sema (MariaDB 10.6+ / MySQL 8.0+)

CREATE TABLE IF NOT EXISTS settings (
  `key`        VARCHAR(64)  NOT NULL PRIMARY KEY,
  `value`      TEXT         NOT NULL,
  updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS users (
  id            INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  username      VARCHAR(64)  NOT NULL UNIQUE,
  full_name     VARCHAR(128) NOT NULL,
  email         VARCHAR(190) NOT NULL DEFAULT '',
  password_hash VARCHAR(255) NOT NULL,
  role          ENUM('admin','komisyon') NOT NULL DEFAULT 'komisyon',
  is_active     TINYINT(1)   NOT NULL DEFAULT 1,
  must_change   TINYINT(1)   NOT NULL DEFAULT 0,
  last_login_at DATETIME     NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sessions (
  token      CHAR(64)     NOT NULL PRIMARY KEY,
  user_id    INT UNSIGNED NOT NULL,
  ip         VARCHAR(45)  NOT NULL DEFAULT '',
  user_agent VARCHAR(255) NOT NULL DEFAULT '',
  expires_at DATETIME     NOT NULL,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  INDEX idx_sessions_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS periods (
  id               INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  slug             VARCHAR(64)  NOT NULL UNIQUE,
  name             VARCHAR(160) NOT NULL,
  description      TEXT         NULL,
  opens_at         DATETIME     NOT NULL,
  closes_at        DATETIME     NOT NULL,
  quota            INT UNSIGNED NOT NULL DEFAULT 0,
  reserve_quota    INT UNSIGNED NOT NULL DEFAULT 0,
  status           ENUM('taslak','acik','kapali','ilan') NOT NULL DEFAULT 'taslak',
  auto_weight      DECIMAL(4,3) NOT NULL DEFAULT 0.600,
  committee_weight DECIMAL(4,3) NOT NULL DEFAULT 0.400,
  criteria_json    LONGTEXT     NULL,
  require_docs     TINYINT(1)   NOT NULL DEFAULT 1,
  result_note      TEXT         NULL,
  created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS applications (
  id                  INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  period_id           INT UNSIGNED NOT NULL,
  tracking_code       VARCHAR(16)  NOT NULL UNIQUE,
  draft_token         CHAR(48)     NOT NULL UNIQUE,
  status              ENUM('taslak','basvuruldu','incelemede','asil','yedek','red') NOT NULL DEFAULT 'taslak',

  -- Kimlik (TC sifreli saklanir; hash yalnizca mukerrer kontrolu icin)
  national_id_enc     VARBINARY(255) NULL,
  national_id_hash    CHAR(64)     NULL,
  first_name          VARCHAR(80)  NOT NULL DEFAULT '',
  last_name           VARCHAR(80)  NOT NULL DEFAULT '',
  birth_date          DATE         NULL,
  gender              ENUM('','kadin','erkek','belirtmek_istemiyorum') NOT NULL DEFAULT '',
  phone               VARCHAR(24)  NOT NULL DEFAULT '',
  email               VARCHAR(190) NOT NULL DEFAULT '',
  city                VARCHAR(64)  NOT NULL DEFAULT '',
  district            VARCHAR(64)  NOT NULL DEFAULT '',
  address             TEXT         NULL,

  -- Egitim
  university          VARCHAR(160) NOT NULL DEFAULT '',
  faculty             VARCHAR(160) NOT NULL DEFAULT '',
  department          VARCHAR(160) NOT NULL DEFAULT '',
  class_year          VARCHAR(24)  NOT NULL DEFAULT '',
  student_no          VARCHAR(40)  NOT NULL DEFAULT '',
  education_type      ENUM('','orgun','ikinci_ogretim','acik') NOT NULL DEFAULT '',
  gpa                 DECIMAL(5,2) NULL,
  gpa_scale           ENUM('4','100') NOT NULL DEFAULT '4',

  -- Ekonomik durum
  household_income    INT UNSIGNED NULL,
  household_size      TINYINT UNSIGNED NULL,
  income_per_capita   INT UNSIGNED NULL,
  father_status       ENUM('','calisiyor','calismiyor','emekli','vefat','ayri') NOT NULL DEFAULT '',
  father_job          VARCHAR(120) NOT NULL DEFAULT '',
  mother_status       ENUM('','calisiyor','calismiyor','emekli','vefat','ayri') NOT NULL DEFAULT '',
  mother_job          VARCHAR(120) NOT NULL DEFAULT '',
  owns_property       TINYINT(1)   NOT NULL DEFAULT 0,
  owns_vehicle        TINYINT(1)   NOT NULL DEFAULT 0,
  housing_type        ENUM('','aile_yani','kira','yurt_devlet','yurt_ozel','akraba') NOT NULL DEFAULT '',
  housing_cost        INT UNSIGNED NULL,

  -- Aile
  sibling_count         TINYINT UNSIGNED NOT NULL DEFAULT 0,
  student_sibling_count TINYINT UNSIGNED NOT NULL DEFAULT 0,
  parents_status      ENUM('','birlikte','bosanmis','anne_vefat','baba_vefat','ikisi_vefat') NOT NULL DEFAULT '',
  disability          TINYINT(1)   NOT NULL DEFAULT 0,
  disability_note     VARCHAR(255) NOT NULL DEFAULT '',
  martyr_relative     TINYINT(1)   NOT NULL DEFAULT 0,

  -- Diger burslar
  other_scholarship        TINYINT(1)   NOT NULL DEFAULT 0,
  other_scholarship_name   VARCHAR(160) NOT NULL DEFAULT '',
  other_scholarship_amount INT UNSIGNED NULL,

  -- Sosyal / beyan
  volunteer_text      TEXT         NULL,
  sports_club         VARCHAR(160) NOT NULL DEFAULT '',
  motivation_text     TEXT         NULL,

  -- KVKK ve iz
  kvkk_accepted       TINYINT(1)   NOT NULL DEFAULT 0,
  kvkk_accepted_at    DATETIME     NULL,
  submit_ip           VARCHAR(45)  NOT NULL DEFAULT '',

  -- Puanlama
  auto_score          DECIMAL(6,2) NULL,
  auto_breakdown      LONGTEXT     NULL,
  committee_score     DECIMAL(6,2) NULL,
  final_score         DECIMAL(6,2) NULL,
  rank_no             INT UNSIGNED NULL,
  admin_note          TEXT         NULL,

  submitted_at        DATETIME     NULL,
  created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  CONSTRAINT fk_app_period FOREIGN KEY (period_id) REFERENCES periods(id) ON DELETE CASCADE,
  UNIQUE KEY uq_app_period_tc (period_id, national_id_hash),
  INDEX idx_app_status (period_id, status),
  INDEX idx_app_score (period_id, final_score)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS documents (
  id             INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  application_id INT UNSIGNED NOT NULL,
  kind           VARCHAR(40)  NOT NULL,
  original_name  VARCHAR(255) NOT NULL,
  stored_name    VARCHAR(160) NOT NULL,
  mime           VARCHAR(100) NOT NULL DEFAULT '',
  size_bytes     INT UNSIGNED NOT NULL DEFAULT 0,
  uploaded_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fk_doc_app FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE,
  UNIQUE KEY uq_doc_app_kind (application_id, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS reviews (
  id             INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  application_id INT UNSIGNED NOT NULL,
  reviewer_id    INT UNSIGNED NOT NULL,
  score          DECIMAL(5,2) NOT NULL,
  notes          TEXT         NULL,
  created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_rev_app  FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE,
  CONSTRAINT fk_rev_user FOREIGN KEY (reviewer_id)   REFERENCES users(id) ON DELETE CASCADE,
  UNIQUE KEY uq_review (application_id, reviewer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS audit_log (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  actor      VARCHAR(80)  NOT NULL DEFAULT 'sistem',
  action     VARCHAR(80)  NOT NULL,
  entity     VARCHAR(40)  NOT NULL DEFAULT '',
  entity_id  VARCHAR(40)  NOT NULL DEFAULT '',
  detail     TEXT         NULL,
  ip         VARCHAR(45)  NOT NULL DEFAULT '',
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_audit_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
