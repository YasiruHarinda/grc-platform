-- =============================================================================
-- GRC Platform — Audit Module Schema
-- Run AFTER shared.sql.
-- =============================================================================
--
-- Tables:
--   audit_team              — team unit that submits evidence for controls
--   audit_framework         — SOC2, ISO 27001, HIPAA, etc.
--   audit_product           — Asgardeo, Choreo, etc.
--   audit_framework_control — versioned, immutable control definition library
--   audit                   — one audit instance (framework × product × year)
--   audit_control           — control snapshot inside an audit
--   audit_population        — OE-type control population phase record
--   audit_evidence          — evidence submission for a control
--   audit_evidence_file     — files attached to evidence or population
--   audit_comment           — threaded comments on an evidence submission
--   audit_ai_validation_log — async AI validation results (hints only, append-only)
--   audit_notification      — in-app and email notifications
--   audit_trail             — immutable event log for an audit
-- =============================================================================

SET FOREIGN_KEY_CHECKS = 0;

-- =============================================================================
-- audit_team
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_team (
  id          INT          NOT NULL AUTO_INCREMENT,
  name        VARCHAR(150) NOT NULL,
  status      ENUM('ACTIVE','INACTIVE') NOT NULL DEFAULT 'ACTIVE',
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by  VARCHAR(255) NULL,
  updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by  VARCHAR(255) NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Wire the FK from user.audit_team_id now that audit_team exists.
ALTER TABLE `user`
  ADD CONSTRAINT fk_user_audit_team FOREIGN KEY (audit_team_id) REFERENCES audit_team(id) ON DELETE SET NULL;

-- =============================================================================
-- audit_framework and audit_product
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_framework (
  id          INT          NOT NULL AUTO_INCREMENT,
  name        VARCHAR(100) NOT NULL,
  status      ENUM('ACTIVE','INACTIVE') NOT NULL DEFAULT 'ACTIVE',
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by  VARCHAR(255) NULL,
  updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by  VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_framework_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS audit_product (
  id          INT          NOT NULL AUTO_INCREMENT,
  name        VARCHAR(100) NOT NULL,
  status      ENUM('ACTIVE','INACTIVE') NOT NULL DEFAULT 'ACTIVE',
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by  VARCHAR(255) NULL,
  updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by  VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_product_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit_framework_control  (versioned, immutable control library)
--
-- Rows are never updated. When a control definition changes a new row is
-- inserted with version = old.version + 1 and is_current = TRUE; the
-- previous row is flipped to is_current = FALSE. audit_control rows reference
-- a specific version id so past audits are never affected by control changes.
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_framework_control (
  id                   INT          NOT NULL AUTO_INCREMENT,
  framework_id         INT          NOT NULL,
  control_number       VARCHAR(60)  NOT NULL,
  description          TEXT         NOT NULL,
  evidence_requirement TEXT         NULL,
  requirement_type     ENUM('DESIGN','OE')              NOT NULL,
  control_type         ENUM('CONFIG','NON_CONFIG')       NOT NULL,
  scope                ENUM('COMMON','PRODUCT_SPECIFIC') NOT NULL,
  version              INT          NOT NULL DEFAULT 1,
  is_current           BOOLEAN      NOT NULL DEFAULT TRUE,
  created_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by           VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_fwctl_framework (framework_id),
  KEY idx_fwctl_number    (framework_id, control_number),
  KEY idx_fwctl_current   (framework_id, control_number, is_current),
  CONSTRAINT fk_fwctl_framework FOREIGN KEY (framework_id) REFERENCES audit_framework(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit  (one audit instance: framework × product × year)
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit (
  id                   INT          NOT NULL AUTO_INCREMENT,
  name                 VARCHAR(200) NOT NULL,
  framework_id         INT          NOT NULL,
  product_id           INT          NOT NULL,
  period_start         DATE         NOT NULL,
  period_end           DATE         NOT NULL,
  status               ENUM('ACTIVE','COMPLETED','ARCHIVED','REMOVED') NOT NULL DEFAULT 'ACTIVE',
  scope_description    TEXT         NULL,
  copied_from_audit_id INT          NULL,
  created_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by           VARCHAR(255) NULL,
  updated_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by           VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_audit_framework (framework_id),
  KEY idx_audit_product   (product_id),
  KEY idx_audit_status    (status),
  CONSTRAINT fk_audit_framework FOREIGN KEY (framework_id)         REFERENCES audit_framework(id) ON DELETE RESTRICT,
  CONSTRAINT fk_audit_product   FOREIGN KEY (product_id)           REFERENCES audit_product(id)   ON DELETE RESTRICT,
  CONSTRAINT fk_audit_copied    FOREIGN KEY (copied_from_audit_id) REFERENCES audit(id)           ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit_control  (control snapshot inside an audit)
--
-- framework_control_id links to the versioned template row at creation time,
-- snapshotting the version. Definition columns (control_number, description,
-- etc.) are NULL when a template is linked; reads use COALESCE(fc.col, c.col)
-- to resolve definitions from the template automatically.
-- control_source tracks whether the control was added MANUALLY, COPIED from
-- a previous audit, or imported via CSV.
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_control (
  id                   INT          NOT NULL AUTO_INCREMENT,
  audit_id             INT          NOT NULL,
  framework_control_id INT          NULL,     -- NULL = manually added control with no template

  -- Definition columns: only stored when framework_control_id IS NULL.
  -- Reads use COALESCE(framework_control.col, audit_control.col).
  control_number       VARCHAR(60)  NULL,
  description          TEXT         NULL,
  evidence_requirement TEXT         NULL,
  requirement_type     ENUM('DESIGN','OE')              NULL,
  control_type         ENUM('CONFIG','NON_CONFIG')       NULL,
  scope                ENUM('COMMON','PRODUCT_SPECIFIC') NULL,

  -- Instance data (always present)
  owner_id             INT          NULL,
  team_id              INT          NULL,
  auditor_id           INT          NULL,
  due_date             DATE         NULL,
  status               ENUM(
                           'POPULATION_PENDING',
                           'POPULATION_INTERNAL_REVIEW',
                           'POPULATION_UNDER_VALIDATION',
                           'POPULATION_NEED_CLARIFICATION',
                           'POPULATION_COMPLETE',
                           'AWAITING_SAMPLE',
                           'SUBMITTED_SAMPLE',
                           'EVIDENCE_PENDING',
                           'EVIDENCE_INTERNAL_REVIEW',
                           'EVIDENCE_UNDER_VALIDATION',
                           'EVIDENCE_NEED_CLARIFICATION',
                           'COMPLETE'
                         ) NOT NULL DEFAULT 'EVIDENCE_PENDING',
  sample_reference     VARCHAR(255) NULL,
  comments             TEXT         NULL,
  control_source       ENUM('MANUAL','COPIED','CSV') NOT NULL DEFAULT 'MANUAL',
  created_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by           VARCHAR(255) NULL,
  updated_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by           VARCHAR(255) NULL,
  PRIMARY KEY (id),
  -- Prevent same template control appearing twice in one audit
  UNIQUE KEY uq_fwctl_per_audit       (audit_id, framework_control_id),
  -- Prevent duplicate manual control numbers within one audit (NULLs exempt)
  UNIQUE KEY uq_manual_ctrl_per_audit (audit_id, control_number),
  KEY idx_control_audit_status (audit_id, status),
  KEY idx_control_fwctl        (framework_control_id),
  KEY idx_control_team         (team_id),
  KEY idx_control_owner        (owner_id),
  KEY idx_control_auditor      (auditor_id),
  KEY idx_control_due          (due_date),
  CONSTRAINT fk_control_audit   FOREIGN KEY (audit_id)             REFERENCES audit(id)                   ON DELETE CASCADE,
  CONSTRAINT fk_control_fwctl   FOREIGN KEY (framework_control_id) REFERENCES audit_framework_control(id) ON DELETE RESTRICT,
  CONSTRAINT fk_control_owner   FOREIGN KEY (owner_id)             REFERENCES `user`(id)                  ON DELETE SET NULL,
  CONSTRAINT fk_control_team    FOREIGN KEY (team_id)              REFERENCES audit_team(id)              ON DELETE SET NULL,
  CONSTRAINT fk_control_auditor FOREIGN KEY (auditor_id)           REFERENCES `user`(id)                  ON DELETE SET NULL,
  -- Either template reference OR all 5 definition columns must be present
  CONSTRAINT chk_control_definition CHECK (
    framework_control_id IS NOT NULL OR (
      control_number   IS NOT NULL AND
      description      IS NOT NULL AND
      requirement_type IS NOT NULL AND
      control_type     IS NOT NULL AND
      scope            IS NOT NULL
    )
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit_population  (OE-type control population phase record)
--
-- audit_id removed: derivable via control_id → audit_control.audit_id.
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_population (
  id               INT          NOT NULL AUTO_INCREMENT,
  control_id       INT          NOT NULL,
  owner_id         INT          NULL,
  team_id          INT          NULL,
  reference_number INT          NULL,
  description      TEXT         NULL,
  status           ENUM(
                     'PENDING',
                     'SUBMITTED',
                     'COMPLIANCE_APPROVED',
                     'COMPLIANCE_REJECTED',
                     'APPROVED',
                     'AUDITOR_REJECTED'
                   ) NOT NULL DEFAULT 'PENDING',
  due_date         DATE         NULL,
  comments         TEXT         NULL,
  created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by       VARCHAR(255) NULL,
  updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by       VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_pop_control (control_id),
  CONSTRAINT fk_pop_control FOREIGN KEY (control_id) REFERENCES audit_control(id) ON DELETE CASCADE,
  CONSTRAINT fk_pop_owner   FOREIGN KEY (owner_id)   REFERENCES `user`(id)        ON DELETE SET NULL,
  CONSTRAINT fk_pop_team    FOREIGN KEY (team_id)    REFERENCES audit_team(id)    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit_evidence  (evidence submission for a control)
--
-- reused_from_evidence_id supports cross-certification evidence reuse:
-- the same evidence can satisfy the same control across SOC2 and ISO 27001
-- for the same product in the same year without re-uploading files.
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_evidence (
  id                      INT          NOT NULL AUTO_INCREMENT,
  control_id              INT          NOT NULL,
  submitted_by            INT          NULL,
  status                  ENUM(
                            'SUBMITTED',
                            'COMPLIANCE_APPROVED',
                            'COMPLIANCE_REJECTED',
                            'APPROVED',
                            'AUDITOR_REJECTED'
                          ) NOT NULL DEFAULT 'SUBMITTED',
  reused_from_evidence_id INT          NULL,
  folder_path             VARCHAR(500) NULL,
  created_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by              VARCHAR(255) NULL,
  updated_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by              VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_evidence_control (control_id),
  KEY idx_evidence_status  (status),
  CONSTRAINT fk_evidence_control   FOREIGN KEY (control_id)              REFERENCES audit_control(id)  ON DELETE CASCADE,
  CONSTRAINT fk_evidence_submitter FOREIGN KEY (submitted_by)            REFERENCES `user`(id)         ON DELETE SET NULL,
  CONSTRAINT fk_evidence_reused    FOREIGN KEY (reused_from_evidence_id) REFERENCES audit_evidence(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit_evidence_file  (files attached to evidence or population)
--
-- Exactly one of evidence_id / population_id must be set (chk_file_owner).
-- file_kind is required only for population files (chk_file_kind).
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_evidence_file (
  id            INT          NOT NULL AUTO_INCREMENT,
  evidence_id   INT          NULL,
  population_id INT          NULL,
  file_kind     ENUM('POPULATION','SAMPLE') NULL,
  uploaded_by   INT          NULL,
  file_name     VARCHAR(255) NOT NULL,
  file_path     TEXT         NOT NULL,
  file_type     VARCHAR(100) NULL,
  file_size     BIGINT       NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by    VARCHAR(255) NULL,
  updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by    VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_file_evidence   (evidence_id),
  KEY idx_file_population (population_id),
  CONSTRAINT fk_file_evidence   FOREIGN KEY (evidence_id)   REFERENCES audit_evidence(id)   ON DELETE CASCADE,
  CONSTRAINT fk_file_population FOREIGN KEY (population_id) REFERENCES audit_population(id) ON DELETE CASCADE,
  CONSTRAINT fk_file_uploader   FOREIGN KEY (uploaded_by)   REFERENCES `user`(id)           ON DELETE SET NULL,
  CONSTRAINT chk_file_owner CHECK ((evidence_id IS NOT NULL) <> (population_id IS NOT NULL)),
  CONSTRAINT chk_file_kind  CHECK ((population_id IS NULL) = (file_kind IS NULL))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit_comment  (threaded comments on an evidence submission)
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_comment (
  id                INT          NOT NULL AUTO_INCREMENT,
  evidence_id       INT          NOT NULL,
  author_id         INT          NULL,
  parent_comment_id INT          NULL,
  content           TEXT         NOT NULL,
  is_internal       BOOLEAN      NOT NULL DEFAULT FALSE,
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by        VARCHAR(255) NULL,
  updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by        VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_comment_evidence (evidence_id),
  CONSTRAINT fk_comment_evidence FOREIGN KEY (evidence_id)       REFERENCES audit_evidence(id) ON DELETE CASCADE,
  CONSTRAINT fk_comment_author   FOREIGN KEY (author_id)         REFERENCES `user`(id)         ON DELETE SET NULL,
  CONSTRAINT fk_comment_parent   FOREIGN KEY (parent_comment_id) REFERENCES audit_comment(id)  ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit_ai_validation_log  (async AI validation results, append-only)
--
-- Rows are hints only — they never block or change evidence status.
-- The AI service writes here after async analysis of the evidence file.
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_ai_validation_log (
  id               BIGINT       NOT NULL AUTO_INCREMENT,
  evidence_id      INT          NOT NULL,
  control_id       INT          NOT NULL,
  -- PASS/FAIL/UNCERTAIN are verdicts; PENDING (job started) and ERROR (job
  -- failed) are lifecycle rows appended by the validation agent (append-only,
  -- no UPDATEs — the UI reads the latest row per evidence).
  result           ENUM('PASS','FAIL','UNCERTAIN','PENDING','ERROR') NOT NULL,
  gaps_found       TEXT         NULL,     -- JSON array of gap objects
  feedback         TEXT         NULL,     -- JSON array of submitter-facing action strings
  summary          TEXT         NULL,
  confidence_score DECIMAL(5,4) NULL,
  created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by       VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_ai_evidence (evidence_id),
  KEY idx_ai_control  (control_id),
  CONSTRAINT fk_ai_evidence FOREIGN KEY (evidence_id) REFERENCES audit_evidence(id) ON DELETE CASCADE,
  CONSTRAINT fk_ai_control  FOREIGN KEY (control_id)  REFERENCES audit_control(id)  ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit_notification
--
-- NOTE: MySQL does not allow a CHECK constraint on columns that are used in an
-- ON DELETE SET NULL FK action. Enforce "at least one of audit_id/control_id/
-- evidence_id is non-NULL" at the application layer.
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_notification (
  id           BIGINT       NOT NULL AUTO_INCREMENT,
  recipient_id INT          NOT NULL,
  audit_id     INT          NULL,
  control_id   INT          NULL,
  evidence_id  INT          NULL,
  type         ENUM('REMINDER','ESCALATION','APPROVAL','REJECTION','COMMENT') NOT NULL,
  channel      ENUM('EMAIL','IN_APP') NOT NULL,
  message      TEXT         NULL,
  is_read      BOOLEAN      NOT NULL DEFAULT FALSE,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by   VARCHAR(255) NULL,
  updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by   VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_notif_recipient_unread (recipient_id, is_read),
  CONSTRAINT fk_notif_recipient FOREIGN KEY (recipient_id) REFERENCES `user`(id)         ON DELETE CASCADE,
  CONSTRAINT fk_notif_audit     FOREIGN KEY (audit_id)     REFERENCES audit(id)          ON DELETE SET NULL,
  CONSTRAINT fk_notif_control   FOREIGN KEY (control_id)   REFERENCES audit_control(id)  ON DELETE SET NULL,
  CONSTRAINT fk_notif_evidence  FOREIGN KEY (evidence_id)  REFERENCES audit_evidence(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =============================================================================
-- audit_trail  (immutable event log, append-only)
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_trail (
  id          BIGINT       NOT NULL AUTO_INCREMENT,
  actor_id    INT          NULL,
  audit_id    INT          NULL,
  control_id  INT          NULL,
  evidence_id INT          NULL,
  action      ENUM('CREATED','UPLOADED','RESUBMITTED','APPROVED','REJECTED',
                  'COMMENTED','ESCALATED','AI_VALIDATED','EXPORTED',
                  'UPDATED','DELETED') NOT NULL,
  details     JSON         NULL,
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by  VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_trail_audit_time (audit_id, created_at),
  KEY idx_trail_actor      (actor_id),
  CONSTRAINT fk_trail_actor    FOREIGN KEY (actor_id)    REFERENCES `user`(id)         ON DELETE SET NULL,
  CONSTRAINT fk_trail_audit    FOREIGN KEY (audit_id)    REFERENCES audit(id)          ON DELETE SET NULL,
  CONSTRAINT fk_trail_control  FOREIGN KEY (control_id)  REFERENCES audit_control(id)  ON DELETE SET NULL,
  CONSTRAINT fk_trail_evidence FOREIGN KEY (evidence_id) REFERENCES audit_evidence(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS = 1;


