-- =============================================================================
-- GRC Platform — Audit Module Schema
-- Run AFTER shared.sql.
-- =============================================================================
--
-- Tables:
--   audit_team              — team unit that submits evidence for controls
--   user_audit_team         — many-to-many: which audit team(s) a user belongs to
--   audit_framework         — SOC2, ISO 27001, HIPAA, etc.
--   audit_product           — Asgardeo, Choreo, etc.
--   audit_framework_control — versioned, immutable control definition library
--   audit                   — one audit instance (framework × product × year)
--   audit_control           — control snapshot inside an audit
--   audit_population        — OE-type control population phase record
--   audit_evidence          — evidence submission for a control
--   audit_evidence_file     — files attached to evidence or population
--   audit_comment           — threaded comments on a control (population + evidence phases)
--   audit_ai_validation_log — async AI validation results (hints only, append-only)
--   audit_trail              — immutable event log for an audit
--   audit_notification       — send-log for every audit-module email (also the
--                              reminder job's de-dup mechanism)
-- =============================================================================

USE grc_platform;

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

-- =============================================================================
-- user_audit_team
-- Many-to-many junction: a user can belong to more than one audit team.
-- Composite PK (user_id, audit_team_id) enforces uniqueness. is_active allows
-- toggling a mapping without deleting it. CASCADE on both sides — unlike
-- role_privilege's RESTRICT — because a membership row is meaningless once
-- either the user or the team it points at is gone; there is no "in use"
-- concern to protect the way there is for roles/privileges.
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_audit_team (
  user_id       INT      NOT NULL,
  audit_team_id INT      NOT NULL,
  is_active     BOOLEAN  NOT NULL DEFAULT TRUE,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by    VARCHAR(255) NULL,
  updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by    VARCHAR(255) NULL,
  PRIMARY KEY (user_id, audit_team_id),
  KEY idx_uat_team (audit_team_id),
  CONSTRAINT fk_uat_user FOREIGN KEY (user_id)       REFERENCES `user`(id)    ON DELETE CASCADE,
  CONSTRAINT fk_uat_team FOREIGN KEY (audit_team_id) REFERENCES audit_team(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
-- Always a standalone copy: every row owns its full definition text, and is
-- never linked back to audit_framework_control by foreign key. A control may
-- optionally have been authored into the framework catalog at the same time
-- it was authored here (see audit_framework_control), but that's a one-time
-- write at creation, not a persisted relationship.
-- control_source tracks whether the control was added MANUALLY, COPIED from
-- a previous audit/framework, or imported via CSV.
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_control (
  id                   INT          NOT NULL AUTO_INCREMENT,
  audit_id             INT          NOT NULL,

  control_number       VARCHAR(60)  NOT NULL,
  description          TEXT         NOT NULL,
  evidence_requirement TEXT         NULL,
  requirement_type     ENUM('DESIGN','OE')              NOT NULL,
  control_type         ENUM('CONFIG','NON_CONFIG')       NOT NULL,
  scope                ENUM('COMMON','PRODUCT_SPECIFIC') NOT NULL,

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

  -- Status override marker: set whenever an admin backward-overrides this
  -- control's status instead of it advancing through the normal workflow.
  status_overridden    BOOLEAN      NOT NULL DEFAULT FALSE,
  overridden_by        VARCHAR(255) NULL,
  overridden_at        DATETIME     NULL,

  created_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by           VARCHAR(255) NULL,
  updated_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by           VARCHAR(255) NULL,
  PRIMARY KEY (id),
  -- Prevent duplicate control numbers within one audit
  UNIQUE KEY uq_manual_ctrl_per_audit (audit_id, control_number),
  KEY idx_control_audit_status (audit_id, status),
  KEY idx_control_team         (team_id),
  KEY idx_control_owner        (owner_id),
  KEY idx_control_auditor      (auditor_id),
  KEY idx_control_due          (due_date),
  CONSTRAINT fk_control_audit   FOREIGN KEY (audit_id)             REFERENCES audit(id)                   ON DELETE CASCADE,
  CONSTRAINT fk_control_owner   FOREIGN KEY (owner_id)             REFERENCES `user`(id)                  ON DELETE SET NULL,
  CONSTRAINT fk_control_team    FOREIGN KEY (team_id)              REFERENCES audit_team(id)              ON DELETE SET NULL,
  CONSTRAINT fk_control_auditor FOREIGN KEY (auditor_id)           REFERENCES `user`(id)                  ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Backfill the status-override columns onto an audit_control table created
-- before this feature existed. Deliberately not `ADD COLUMN IF NOT EXISTS`:
-- that syntax needs MySQL 8.0.29+, and nothing in this repo pins a MySQL
-- patch version — the information_schema guard below is portable to any
-- MySQL 8, matching the pattern in shared.sql's role table migration.
SET @control_has_status_overridden = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_control' AND COLUMN_NAME = 'status_overridden'
);
SET @add_status_overridden_sql = IF(@control_has_status_overridden = 0,
  'ALTER TABLE audit_control ADD COLUMN status_overridden BOOLEAN NOT NULL DEFAULT FALSE AFTER control_source',
  'SELECT 1');
PREPARE add_status_overridden_stmt FROM @add_status_overridden_sql;
EXECUTE add_status_overridden_stmt;
DEALLOCATE PREPARE add_status_overridden_stmt;

SET @control_has_overridden_by = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_control' AND COLUMN_NAME = 'overridden_by'
);
SET @add_overridden_by_sql = IF(@control_has_overridden_by = 0,
  'ALTER TABLE audit_control ADD COLUMN overridden_by VARCHAR(255) NULL AFTER status_overridden',
  'SELECT 1');
PREPARE add_overridden_by_stmt FROM @add_overridden_by_sql;
EXECUTE add_overridden_by_stmt;
DEALLOCATE PREPARE add_overridden_by_stmt;

SET @control_has_overridden_at = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_control' AND COLUMN_NAME = 'overridden_at'
);
SET @add_overridden_at_sql = IF(@control_has_overridden_at = 0,
  'ALTER TABLE audit_control ADD COLUMN overridden_at DATETIME NULL AFTER overridden_by',
  'SELECT 1');
PREPARE add_overridden_at_stmt FROM @add_overridden_at_sql;
EXECUTE add_overridden_at_stmt;
DEALLOCATE PREPARE add_overridden_at_stmt;

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
  attestation      TEXT         NULL COMMENT 'Written note standing in for population files',
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

-- Backfill the attestation column onto an audit_population table created
-- before fileless population completion existed (see audit_control's
-- status-override backfill above for why an information_schema guard instead
-- of `ADD COLUMN IF NOT EXISTS`).
SET @pop_has_attestation = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_population' AND COLUMN_NAME = 'attestation'
);
SET @add_pop_attestation_sql = IF(@pop_has_attestation = 0,
  'ALTER TABLE audit_population ADD COLUMN attestation TEXT NULL COMMENT ''Written note standing in for population files'' AFTER comments',
  'SELECT 1');
PREPARE add_pop_attestation_stmt FROM @add_pop_attestation_sql;
EXECUTE add_pop_attestation_stmt;
DEALLOCATE PREPARE add_pop_attestation_stmt;

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
  attestation             TEXT         NULL COMMENT 'Written justification for a round with no files',
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

-- Backfill the attestation column onto an audit_evidence table created before
-- fileless evidence completion existed (see audit_control's status-override
-- backfill above for why an information_schema guard instead of
-- `ADD COLUMN IF NOT EXISTS`).
SET @evidence_has_attestation = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_evidence' AND COLUMN_NAME = 'attestation'
);
SET @add_evidence_attestation_sql = IF(@evidence_has_attestation = 0,
  'ALTER TABLE audit_evidence ADD COLUMN attestation TEXT NULL COMMENT ''Written justification for a round with no files'' AFTER folder_path',
  'SELECT 1');
PREPARE add_evidence_attestation_stmt FROM @add_evidence_attestation_sql;
EXECUTE add_evidence_attestation_stmt;
DEALLOCATE PREPARE add_evidence_attestation_stmt;

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
-- audit_comment  (threaded comments on a control — one thread per control,
-- spanning both the population and evidence phases, available from the
-- moment the control is opened)
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_comment (
  id                INT          NOT NULL AUTO_INCREMENT,
  control_id        INT          NOT NULL,
  author_id         INT          NULL,
  parent_comment_id INT          NULL,
  content           TEXT         NOT NULL,
  is_internal       BOOLEAN      NOT NULL DEFAULT FALSE,
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by        VARCHAR(255) NULL,
  updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by        VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_comment_control (control_id),
  CONSTRAINT fk_comment_control FOREIGN KEY (control_id)        REFERENCES audit_control(id) ON DELETE CASCADE,
  CONSTRAINT fk_comment_author  FOREIGN KEY (author_id)         REFERENCES `user`(id)         ON DELETE SET NULL,
  CONSTRAINT fk_comment_parent  FOREIGN KEY (parent_comment_id) REFERENCES audit_comment(id)  ON DELETE SET NULL
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
                  'UPDATED','DELETED','OVERRIDDEN') NOT NULL,
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

-- Backfill the OVERRIDDEN action value onto an audit_trail table created
-- before status override existed (see audit_control's status-override
-- backfill above for why an information_schema guard instead of a bare
-- MODIFY COLUMN — this keeps the migration idempotent and avoids rewriting
-- the column on every deploy once it's already current).
SET @trail_has_overridden_action = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_trail' AND COLUMN_NAME = 'action'
    AND COLUMN_TYPE LIKE '%''OVERRIDDEN''%'
);
SET @add_trail_overridden_sql = IF(@trail_has_overridden_action = 0,
  'ALTER TABLE audit_trail MODIFY COLUMN action ENUM(''CREATED'',''UPLOADED'',''RESUBMITTED'',''APPROVED'',''REJECTED'',
  ''COMMENTED'',''ESCALATED'',''AI_VALIDATED'',''EXPORTED'',''UPDATED'',''DELETED'',''OVERRIDDEN'') NOT NULL',
  'SELECT 1');
PREPARE add_trail_overridden_stmt FROM @add_trail_overridden_sql;
EXECUTE add_trail_overridden_stmt;
DEALLOCATE PREPARE add_trail_overridden_stmt;

-- =============================================================================
-- audit_notification  (universal send-log for every audit-module email)
--
-- One row per email actually sent (not per event-trigger — a triggered event
-- with zero deliverable recipients, or that fails to send, writes no row).
-- Also the de-dup mechanism for the daily reminder job (REMINDER_DUE_10 /
-- REMINDER_DUE_5 / REMINDER_OVERDUE): the job atomically CLAIMS an item by
-- inserting its row *before* sending — the insert's success or failure IS the
-- de-dup decision — rather than checking-then-writing, which left a window
-- for two overlapping runs (e.g. two backend replicas both waking at the
-- daily reminder hour) to both observe "not sent yet" and both send. A
-- failed send deletes its claim
-- row so the item is retried on the next run.
--
-- reminder_dedup_key makes that insert collide correctly. A plain
-- UNIQUE KEY (recipient_id, type, control_id, population_id, due_date_snapshot)
-- would NOT work: control_id and population_id are mutually-exclusive
-- nullable columns, and MySQL's unique-index NULL semantics (NULL never
-- equals NULL) mean two rows that are both, say, control_id=NULL would never
-- collide. reminder_dedup_key sidesteps this by being a generated, non-NULL
-- string ONLY for the three REMINDER_* types (COALESCE folds whichever of
-- control_id/population_id is unset into '', so the two can't collide with
-- each other); for the other five notification types it's NULL, and MySQL's
-- unique index — unlike NULL-safe equality — ignores NULL for uniqueness, so
-- uq_notif_reminder_dedup imposes no constraint on them at all.
--
-- due_date_snapshot freezes the due_date a reminder was computed against, so
-- editing a control/population's due_date later correctly restarts its
-- reminder cycle (a new due_date_snapshot no longer matches old log rows).
--
-- MySQL does not allow a CHECK constraint on columns used in an ON DELETE SET
-- NULL FK action, so "exactly one of control_id/population_id is non-NULL" is
-- enforced at the application layer, not here.
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_notification (
  id                 BIGINT       NOT NULL AUTO_INCREMENT,
  recipient_id       INT          NULL,
  audit_id           INT          NULL,
  control_id         INT          NULL,
  population_id      INT          NULL,
  type               ENUM(
                         'OWNER_ASSIGNED_CONTROL',
                         'OWNER_ASSIGNED_POPULATION',
                         'AUDITOR_ASSIGNED_CONTROL',
                         'REMINDER_DUE_10',
                         'REMINDER_DUE_5',
                         'REMINDER_OVERDUE',
                         'RESUBMISSION_NEEDED',
                         'SAMPLE_SUBMITTED',
                         'EVIDENCE_INTERNAL_REVIEW',
                         'POPULATION_INTERNAL_REVIEW',
                         'EVIDENCE_UNDER_VALIDATION',
                         'POPULATION_UNDER_VALIDATION',
                         'POPULATION_COMPLETE_SAMPLE_NEEDED',
                         'CONTROL_COMPLETE',
                         'COMMENT_ADDED'
                       ) NOT NULL,
  channel            ENUM('EMAIL') NOT NULL DEFAULT 'EMAIL',
  due_date_snapshot  DATE         NULL,
  message            TEXT         NULL,
  created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by         VARCHAR(255) NULL,
  reminder_dedup_key VARCHAR(64) GENERATED ALWAYS AS (
                         CASE WHEN type IN ('REMINDER_DUE_10', 'REMINDER_DUE_5', 'REMINDER_OVERDUE')
                           THEN CONCAT_WS('|',
                                  recipient_id, type,
                                  COALESCE(control_id, ''), COALESCE(population_id, ''),
                                  due_date_snapshot)
                           ELSE NULL
                         END
                       ) STORED,
  PRIMARY KEY (id),
  KEY idx_notif_dedup (recipient_id, type, control_id, population_id, due_date_snapshot),
  UNIQUE KEY uq_notif_reminder_dedup (reminder_dedup_key),
  CONSTRAINT fk_notif_recipient  FOREIGN KEY (recipient_id)  REFERENCES `user`(id)           ON DELETE RESTRICT,
  CONSTRAINT fk_notif_audit      FOREIGN KEY (audit_id)      REFERENCES audit(id)            ON DELETE SET NULL,
  CONSTRAINT fk_notif_control    FOREIGN KEY (control_id)    REFERENCES audit_control(id)    ON DELETE SET NULL,
  CONSTRAINT fk_notif_population FOREIGN KEY (population_id) REFERENCES audit_population(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Backfill the seven status-reached/comment-added type values onto an
-- audit_notification table created before notifyControlStatusReached and
-- notifyCommentAdded existed (handler/notify.go) — same information_schema-guard
-- pattern as audit_trail's OVERRIDDEN backfill above. Without this, every log
-- write for those seven types fails against an ENUM that doesn't contain them.
SET @notif_has_comment_added_type = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_notification' AND COLUMN_NAME = 'type'
    AND COLUMN_TYPE LIKE '%''COMMENT_ADDED''%'
);
SET @add_notif_status_types_sql = IF(@notif_has_comment_added_type = 0,
  'ALTER TABLE audit_notification MODIFY COLUMN type ENUM(
     ''OWNER_ASSIGNED_CONTROL'',
     ''OWNER_ASSIGNED_POPULATION'',
     ''AUDITOR_ASSIGNED_CONTROL'',
     ''REMINDER_DUE_10'',
     ''REMINDER_DUE_5'',
     ''REMINDER_OVERDUE'',
     ''RESUBMISSION_NEEDED'',
     ''SAMPLE_SUBMITTED'',
     ''EVIDENCE_INTERNAL_REVIEW'',
     ''POPULATION_INTERNAL_REVIEW'',
     ''EVIDENCE_UNDER_VALIDATION'',
     ''POPULATION_UNDER_VALIDATION'',
     ''POPULATION_COMPLETE_SAMPLE_NEEDED'',
     ''CONTROL_COMPLETE'',
     ''COMMENT_ADDED''
   ) NOT NULL',
  'SELECT 1');
PREPARE add_notif_status_types_stmt FROM @add_notif_status_types_sql;
EXECUTE add_notif_status_types_stmt;
DEALLOCATE PREPARE add_notif_status_types_stmt;

-- Backfill reminder_dedup_key (+ its unique index) onto an audit_notification
-- table created before the atomic-claim de-dup mechanism existed — same
-- information_schema-guard pattern as the status-override columns above.
-- Column and index are added together: nothing in this schema ever adds one
-- without the other, so a single guard on the column's existence is enough.
SET @notif_has_dedup_key = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'audit_notification' AND COLUMN_NAME = 'reminder_dedup_key'
);
SET @add_notif_dedup_key_sql = IF(@notif_has_dedup_key = 0,
  'ALTER TABLE audit_notification
     ADD COLUMN reminder_dedup_key VARCHAR(64) GENERATED ALWAYS AS (
       CASE WHEN type IN (''REMINDER_DUE_10'', ''REMINDER_DUE_5'', ''REMINDER_OVERDUE'')
         THEN CONCAT_WS(''|'',
                recipient_id, type,
                COALESCE(control_id, ''''), COALESCE(population_id, ''''),
                due_date_snapshot)
         ELSE NULL
       END
     ) STORED AFTER created_by,
     ADD UNIQUE KEY uq_notif_reminder_dedup (reminder_dedup_key)',
  'SELECT 1');
PREPARE add_notif_dedup_key_stmt FROM @add_notif_dedup_key_sql;
EXECUTE add_notif_dedup_key_stmt;
DEALLOCATE PREPARE add_notif_dedup_key_stmt;

SET FOREIGN_KEY_CHECKS = 1;


