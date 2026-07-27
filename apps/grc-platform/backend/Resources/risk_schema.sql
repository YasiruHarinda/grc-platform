-- =============================================================================
-- GRC Platform — Risk Module — MySQL Schema
-- Run AFTER shared.sql.
-- =============================================================================
--
-- Conventions (aligned with audit module schema):
--   • Primary keys: AUTO_INCREMENT surrogate INT for entity tables,
--     BIGINT for high-volume append-only tables (risk_change_log,
--     risk_notification — same pattern as audit_trail / audit_notification).
--   • All FK columns are INT matching their referenced PK type.
--   • created_by / updated_by are VARCHAR(255) NULLable — store actor email.
--   • Roles are NOT stored in the DB — they come from Asgardeo JWT claims.
--   • ENUM values use UPPERCASE throughout this module.
--
-- Shared tables (user, role, privilege, role_privilege) are defined in
-- shared.sql and must already exist before running this file.
--
-- FK ON DELETE policy
--   • Identity refs (user, risk_team)                     ............... RESTRICT
--   • Owning parents (risk → action_plan/step/evidence/…) ............... CASCADE
--   • Optional associations (scores, approvers, plans)    ............... SET NULL
--   • Audit trail creator                                 ............... RESTRICT
--   • Junction tables (risk_compliance_reference,
--     user_risk_team)                                     ............... CASCADE
-- =============================================================================

USE grc_platform;

SET FOREIGN_KEY_CHECKS = 0;

-- -----------------------------------------------------------------------------
-- risk_team
-- Represents both source registers (risk origin) and assignment teams.
-- team_type controls how a team appears in UI dropdowns:
--   SOURCE_REGISTER → only in "source register" picker
--   ASSIGNMENT      → only in "assign to team" picker
--   BOTH            → appears in both pickers
-- code is NULL for teams that are never used as source registers (e.g. Legal, HR).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_team (
  id          INT          NOT NULL AUTO_INCREMENT,
  name        VARCHAR(255) NOT NULL,
  code        VARCHAR(50)  NULL COMMENT 'Short abbreviation used in risk codes, e.g. ASG, CHO, CC; NULL for assignment-only teams',
  description TEXT         NULL,
  team_type   ENUM('SOURCE_REGISTER','ASSIGNMENT','BOTH') NOT NULL,
  status      ENUM('ACTIVE','INACTIVE','REMOVED')         NOT NULL DEFAULT 'ACTIVE',
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by  VARCHAR(255) NULL,
  updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by  VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_risk_team_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_score
-- Lookup table for all 9 likelihood × impact combinations (1–3 each).
-- Seeded once at schema init via risk_module_data_schema.sql.
-- Levels: rating 1–3 → LOW, 4–6 → MEDIUM, 7–9 → HIGH.
-- Gross score is stored as FK on `risk`. Residual scores are stored in
-- risk_assessment (one row per reassessment, with FK to this table).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_score (
  id          INT         NOT NULL AUTO_INCREMENT,
  likelihood  TINYINT     NOT NULL CHECK (likelihood BETWEEN 1 AND 3),
  impact      TINYINT     NOT NULL CHECK (impact BETWEEN 1 AND 3),
  risk_rating TINYINT     NOT NULL COMMENT 'likelihood × impact (1–9)',
  risk_level  ENUM('LOW','MEDIUM','HIGH') NOT NULL,
  color_code  VARCHAR(20) NOT NULL COMMENT 'Hex colour: #00B050 green / #FF9900 orange / #FF0000 red',
  created_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by  VARCHAR(255) NULL,
  updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by  VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_risk_score_lh_im (likelihood, impact)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_security_compliance_reference
-- Reference frameworks a risk can be tagged against.
-- Examples: ISO 27001, SOC 2, PCI DSS, HIPAA, Security, Business.
-- Linked to risks via the risk_compliance_reference junction table (many-to-many).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_security_compliance_reference (
  id          INT          NOT NULL AUTO_INCREMENT,
  name        VARCHAR(255) NOT NULL,
  description TEXT         NULL,
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by  VARCHAR(255) NULL,
  updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by  VARCHAR(255) NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_register_sequence
-- One row per source register team; tracks the ever-increasing sequence number
-- used to generate risk codes (format: YEAR-TEAMCODE-QUARTER-NNNN).
-- The counter never resets — it increments across years and quarters so every
-- risk code for a given team is globally unique.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_register_sequence (
  risk_team_id         INT NOT NULL COMMENT 'FK to risk_team (source register)',
  last_sequence_number INT NOT NULL DEFAULT 0,
  PRIMARY KEY (risk_team_id),
  CONSTRAINT fk_register_seq_team FOREIGN KEY (risk_team_id) REFERENCES risk_team(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_action_plan
-- Action plan attached to a risk. A risk may have multiple plans:
--   STANDARD   → created by Risk Assigner during registration
--   MANAGEMENT → created by Management as part of an escalation decision
-- action_owner_id is nullable: Compliance assigns the Action Owner after plan
-- creation; it is not required at plan-creation time.
-- NOTE: The FK risk_id → risk(id) is added via ALTER TABLE below to break
-- the circular dependency (risk references risk_action_plan and vice versa).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_action_plan (
  id              INT          NOT NULL AUTO_INCREMENT,
  risk_id         INT          NOT NULL,
  action_owner_id INT          NULL,
  description     TEXT         NULL,
  status          ENUM('PENDING','IN_PROGRESS','COMPLETED') NOT NULL DEFAULT 'PENDING',
  completed_date  DATE         NULL,
  plan_type       ENUM('STANDARD','MANAGEMENT') NOT NULL DEFAULT 'STANDARD',
  created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by      VARCHAR(255) NULL,
  updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by      VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_action_plan_risk (risk_id),
  CONSTRAINT fk_action_plan_owner FOREIGN KEY (action_owner_id) REFERENCES `user`(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk
-- Core entity. Each risk belongs to a source register and is assigned to a team.
--
-- workflow_status is the single source of truth for risk state:
--   DRAFT                              → not yet submitted (reserved)
--   PENDING_RISK_OWNER_APPROVAL        → new risk submitted; awaiting initial Risk Owner approval
--   PENDING_MANAGEMENT_APPROVAL        → Accept+HIGH risk; awaiting Management approval (after Owner)
--   PENDING_COMPLIANCE_REVIEW          → awaiting Compliance team approval (after Owner/Management)
--   IN_REMEDIATION                     → Compliance approved; assigner executing action plan
--   PENDING_OWNER_COMPLETION_APPROVAL  → assigner submitted completion; awaiting Risk Owner sign-off
--   PENDING_COMPLIANCE_CLOSURE         → Risk Owner approved completion; awaiting Compliance closure
--   PENDING_AMENDMENT                  → restricted field edited on IN_REMEDIATION risk; restarts approval chain
--   PENDING_REVISION                   → rejected at any approval stage; back with assigner for rework
--   ESCALATED                          → escalated to Management (future)
--   CLOSED                             → fully closed
--
-- compliance_approval_by / compliance_approval_date record WHO approved and WHEN
-- (workflow_status alone does not carry this audit detail).
--
-- action_plan_id is nullable — set once the Risk Assigner creates the action plan.
-- Gross score is stored as FK to risk_score (never changes after creation).
-- Residual scores are stored in risk_assessment (one row per reassessment).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk (
  id                       INT           NOT NULL AUTO_INCREMENT,
  risk_year                INT           NOT NULL,
  source_register_id       INT           NOT NULL,
  risk_quarter             ENUM('Q1','Q2','Q3','Q4') NOT NULL,
  risk_code                VARCHAR(50)   NOT NULL COMMENT 'Auto-generated: YEAR-TEAMCODE-QUARTER-NNN, e.g. 2026-ASG-Q2-001',
  risk_title               VARCHAR(500)  NOT NULL,
  risk_description         TEXT          NULL,
  risk_identified_date     DATE          NULL,
  identified_by_type       ENUM('EMPLOYEE','EXTERNAL_PERSON','TOOL') NULL,
  identified_by_name       VARCHAR(255)  NULL,
  assigner_id              INT           NOT NULL,
  owner_id                 INT           NOT NULL,
  impact_description       TEXT          NULL,
  gross_score_id           INT           NULL,
  treatment_strategy       ENUM('REMEDIATE','ACCEPT','TRANSFER','VOID') NULL,
  action_plan_id           INT           NULL,
  assignment_team_id       INT           NOT NULL,
  progress                 TEXT          NULL,
  implementation_date      DATE          NULL COMMENT 'Completion deadline; changes trigger notification',
  reassessment_date        DATE          NULL COMMENT 'Date of next scheduled reassessment',
  compliance_approval_by   INT           NULL,
  compliance_approval_date DATE          NULL,
  git_issue_url            VARCHAR(1000) NULL,
  email_subject            VARCHAR(500)  NULL,
  remarks                  TEXT          NULL,
  workflow_status          ENUM(
                               'DRAFT',
                               'PENDING_RISK_OWNER_APPROVAL',
                               'PENDING_MANAGEMENT_APPROVAL',
                               'PENDING_COMPLIANCE_REVIEW',
                               'IN_REMEDIATION',
                               'PENDING_OWNER_COMPLETION_APPROVAL',
                               'PENDING_COMPLIANCE_CLOSURE',
                               'PENDING_AMENDMENT',
                               'PENDING_REVISION',
                               'ESCALATED',
                               'CLOSED',
                               'CANCELLED'
                           ) NOT NULL DEFAULT 'PENDING_RISK_OWNER_APPROVAL',
  risk_type                ENUM('NEW','UPDATED') NOT NULL DEFAULT 'NEW',
  rejection_comment        TEXT          NULL,
  rejection_stage          VARCHAR(50)   NULL,
  owner_first_approved_at  DATETIME      NULL COMMENT 'Set once when risk owner approves for the first time; never cleared',
  created_at               DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by               VARCHAR(255)  NULL,
  updated_at               DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by               VARCHAR(255)  NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_risk_code (risk_code),
  KEY idx_risk_status    (workflow_status),
  KEY idx_risk_assigner  (assigner_id),
  KEY idx_risk_owner     (owner_id),
  KEY idx_risk_source    (source_register_id),
  KEY idx_risk_team      (assignment_team_id),
  CONSTRAINT fk_risk_source_register     FOREIGN KEY (source_register_id)     REFERENCES risk_team(id)        ON DELETE RESTRICT,
  CONSTRAINT fk_risk_assignment_team     FOREIGN KEY (assignment_team_id)     REFERENCES risk_team(id)        ON DELETE RESTRICT,
  CONSTRAINT fk_risk_assigner            FOREIGN KEY (assigner_id)            REFERENCES `user`(id)           ON DELETE RESTRICT,
  CONSTRAINT fk_risk_owner               FOREIGN KEY (owner_id)               REFERENCES `user`(id)           ON DELETE RESTRICT,
  CONSTRAINT fk_risk_compliance_approver FOREIGN KEY (compliance_approval_by)  REFERENCES `user`(id)           ON DELETE SET NULL,
  CONSTRAINT fk_risk_gross_score         FOREIGN KEY (gross_score_id)         REFERENCES risk_score(id)       ON DELETE SET NULL,
  CONSTRAINT fk_risk_action_plan         FOREIGN KEY (action_plan_id)         REFERENCES risk_action_plan(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Resolve circular dependency: risk_action_plan ↔ risk
ALTER TABLE risk_action_plan
  ADD CONSTRAINT fk_action_plan_risk FOREIGN KEY (risk_id) REFERENCES risk(id) ON DELETE CASCADE;


-- -----------------------------------------------------------------------------
-- risk_action_step
-- Individual numbered steps within an action plan.
-- step_no ordering is managed by the application.
-- Action Owner marks steps as completed.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_action_step (
  id             INT          NOT NULL AUTO_INCREMENT,
  plan_id        INT          NOT NULL,
  step_no        INT          NOT NULL,
  description    TEXT         NULL,
  status         ENUM('PENDING','IN_PROGRESS','COMPLETED') NOT NULL DEFAULT 'PENDING',
  completed_date DATE         NULL,
  created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by     VARCHAR(255) NULL,
  updated_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by     VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_step_plan (plan_id),
  CONSTRAINT fk_step_plan FOREIGN KEY (plan_id) REFERENCES risk_action_plan(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_evidence
-- Files uploaded as evidence for action plan progress or final Risk Owner
-- approval. file_path is the Azure Blob Storage object key (not full URL);
-- the full URL is constructed by the application at read time.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_evidence (
  id            INT          NOT NULL AUTO_INCREMENT,
  risk_id       INT          NOT NULL,
  file_name     VARCHAR(500) NOT NULL,
  file_path     TEXT         NOT NULL COMMENT 'Azure Blob object key',
  note          TEXT         NULL,
  evidence_type ENUM('ACTION_PLAN_ATTACHMENT','FINAL_APPROVAL_ATTACHMENT') NOT NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by    VARCHAR(255) NULL,
  updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by    VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_risk_evidence_risk (risk_id),
  CONSTRAINT fk_risk_evidence_risk FOREIGN KEY (risk_id) REFERENCES risk(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_escalation
-- Created automatically by the daily overdue-risk job (compliance-entity
-- internal/job) when an IN_REMEDIATION risk passes its implementation_date
-- deadline — there is no human-supplied target or reason at creation time.
-- Resolved (status -> RESOLVED) when the linked MANAGEMENT action plan
-- completes, which also reverts the risk from ESCALATED back to IN_REMEDIATION.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_escalation (
  id                     INT          NOT NULL AUTO_INCREMENT,
  risk_id                INT          NOT NULL,
  new_treatment_strategy VARCHAR(100) NULL,
  action_plan_id         INT          NULL,
  decision               TEXT         NULL,
  status                 ENUM('OPEN','RESOLVED') NOT NULL DEFAULT 'OPEN',
  created_at             DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by             VARCHAR(255) NULL,
  updated_at             DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by             VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_escalation_risk (risk_id),
  CONSTRAINT fk_escalation_risk         FOREIGN KEY (risk_id)        REFERENCES risk(id)             ON DELETE CASCADE,
  CONSTRAINT fk_escalation_action_plan  FOREIGN KEY (action_plan_id) REFERENCES risk_action_plan(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_compliance_reference  (junction table)
-- Many-to-many between risk and risk_security_compliance_reference.
-- A single risk can be tagged against multiple compliance frameworks.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_compliance_reference (
  risk_id      INT      NOT NULL,
  reference_id INT      NOT NULL,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (risk_id, reference_id),
  CONSTRAINT fk_rcr_risk      FOREIGN KEY (risk_id)      REFERENCES risk(id)                              ON DELETE CASCADE,
  CONSTRAINT fk_rcr_reference FOREIGN KEY (reference_id) REFERENCES risk_security_compliance_reference(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_change_log
-- Field-level audit trail for all changes to a risk record. APPEND-ONLY.
-- HIGH-VOLUME → BIGINT id (same pattern as audit_trail).
-- When restricted fields (implementation_date, treatment_strategy,
-- assignment_team_id, action_steps) are edited on an approved risk, a row is
-- written here with old_value/new_value JSON so diffs can be shown later.
-- created_by is NOT NULL — every audit entry must be attributable to an actor email.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_change_log (
  id            BIGINT       NOT NULL AUTO_INCREMENT,
  risk_id       INT          NOT NULL,
  created_by    VARCHAR(255) NOT NULL,
  action        ENUM('CREATE','UPDATE','DELETE') NOT NULL,
  field_changed VARCHAR(255) NULL,
  old_value     JSON         NULL,
  new_value     JSON         NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by    VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_change_log_risk      (risk_id),
  KEY idx_change_log_risk_time (risk_id, created_at),
  CONSTRAINT fk_change_log_risk FOREIGN KEY (risk_id) REFERENCES risk(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_notification
-- In-app and email notifications for risk module events.
-- HIGH-VOLUME → BIGINT id (same pattern as audit_notification).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_notification (
  id           BIGINT       NOT NULL AUTO_INCREMENT,
  recipient_id INT          NOT NULL,
  risk_id      INT          NULL,
  type         ENUM('REMINDER','ESCALATION','STATUS_CHANGE','APPROVAL','REASSESSMENT','REJECTION') NOT NULL,
  channel      ENUM('EMAIL','IN_APP') NOT NULL DEFAULT 'IN_APP',
  message      TEXT         NOT NULL,
  is_read      BOOLEAN      NOT NULL DEFAULT FALSE,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by   VARCHAR(255) NULL,
  updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by   VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_risk_notif_recipient_unread (recipient_id, is_read),
  CONSTRAINT fk_risk_notif_recipient FOREIGN KEY (recipient_id) REFERENCES `user`(id) ON DELETE CASCADE,
  CONSTRAINT fk_risk_notif_risk      FOREIGN KEY (risk_id)      REFERENCES risk(id)   ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- user_risk_team  (junction table)
-- Many-to-many between `user` (shared.sql) and risk_team: a user may belong to
-- zero or more risk teams. Replaces the old single-valued user.risk_team_id
-- column. Both FKs CASCADE — a membership row has no meaning independent of
-- either side, unlike risk_team's other references (risk, risk_register_sequence)
-- which RESTRICT deletion.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_risk_team (
  user_id      INT          NOT NULL,
  risk_team_id INT          NOT NULL,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by   VARCHAR(255) NULL,
  PRIMARY KEY (user_id, risk_team_id),
  CONSTRAINT fk_urt_user FOREIGN KEY (user_id)      REFERENCES `user`(id)  ON DELETE CASCADE,
  CONSTRAINT fk_urt_team FOREIGN KEY (risk_team_id) REFERENCES risk_team(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- risk_assessment
-- One row per residual risk reassessment. Records the updated score, progress
-- notes, and next reassessment date each time a risk is reassessed while in
-- IN_REMEDIATION status. The gross_score_id on `risk` is immutable; residual
-- score history lives here.
-- assessed_by stores the email of the user who submitted the reassessment.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_assessment (
  id                INT          NOT NULL AUTO_INCREMENT,
  risk_id           INT          NOT NULL,
  score_id          INT          NOT NULL COMMENT 'FK to risk_score (residual score for this assessment)',
  progress          TEXT         NOT NULL,
  reassessment_date DATE         NOT NULL,
  assessed_by       VARCHAR(255) NOT NULL COMMENT 'email of the assessor',
  created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by        VARCHAR(255) NULL,
  PRIMARY KEY (id),
  KEY idx_risk_assessment_risk      (risk_id),
  KEY idx_risk_assessment_risk_time (risk_id, created_at),
  CONSTRAINT fk_risk_assessment_risk  FOREIGN KEY (risk_id)  REFERENCES risk(id)       ON DELETE CASCADE,
  CONSTRAINT fk_risk_assessment_score FOREIGN KEY (score_id) REFERENCES risk_score(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


SET FOREIGN_KEY_CHECKS = 1;
