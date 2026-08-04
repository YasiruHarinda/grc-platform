-- =============================================================================
-- GRC Platform — Risk Escalation Leads + Change Log Details + History FK
-- Policy Migration
-- Run AFTER shared.sql and risk_schema.sql — BEFORE deploying this PR's
-- backend code against a database that already has risk_escalation,
-- risk_change_log, and risk_assessment (i.e. anything not freshly created
-- from the current risk_schema.sql).
--
-- All three tables' CREATE TABLE statements in risk_schema.sql are IF NOT
-- EXISTS, so on an existing database that file is a no-op for them — it will
-- not add the columns below, and it will not change an already-existing FK's
-- ON DELETE rule. Without this migration:
--   - the Go backend and the entity select/write assigner_lead_email,
--     action_owner_lead_email, and details against columns that don't exist,
--     failing with "Unknown column";
--   - risk_change_log/risk_assessment rows stay silently CASCADE-deleted
--     alongside their risk, instead of RESTRICTing the delete the way an
--     append-only history table should.
--
-- Run as: mysql -u <user> -p grc_platform < risk_escalation_and_change_log_migration.sql
--
-- Idempotent / safe on a fresh database: the ADD COLUMN blocks are guarded
-- via information_schema (MySQL has no ADD COLUMN IF NOT EXISTS), the ENUM
-- widening is a MODIFY COLUMN restating the same superset each time (a no-op
-- once applied, and no existing row's value is ever removed from the set),
-- and the FK rebuilds are guarded on the constraint's current DELETE_RULE so
-- a database already on RESTRICT is left untouched.
-- =============================================================================

USE grc_platform;

-- ── risk_escalation: assigner_lead_email / action_owner_lead_email ─────────────
-- Line managers of the risk assigner and the action plan owner, resolved from
-- the HR entity once at escalation time and frozen here — see risk_schema.sql
-- for the full column comment.
SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'risk_escalation'
     AND column_name = 'assigner_lead_email') = 0,
  'ALTER TABLE risk_escalation
     ADD COLUMN assigner_lead_email VARCHAR(255) NULL AFTER decision,
     ADD COLUMN action_owner_lead_email VARCHAR(255) NULL AFTER assigner_lead_email',
  'DO 0');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- ── risk_change_log: workflow events alongside the existing field-diff actions ──
-- The action ENUM gains ESCALATE/COMMENT/etc. alongside CREATE/UPDATE/DELETE,
-- and a details JSON column carries an event row's payload (a field-diff row
-- leaves it NULL and uses field_changed/old_value/new_value instead). Mirrors
-- audit_trail's shape in audit_schema.sql.
ALTER TABLE risk_change_log MODIFY COLUMN action ENUM(
  'CREATE','UPDATE','DELETE',
  'SUBMIT','APPROVE','REJECT','ESCALATE','COMMENT',
  'ASSESS','COMPLETE','CLOSE','CANCEL'
) NOT NULL;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.columns
   WHERE table_schema = DATABASE() AND table_name = 'risk_change_log'
     AND column_name = 'details') = 0,
  'ALTER TABLE risk_change_log ADD COLUMN details JSON NULL AFTER new_value',
  'DO 0');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- ── risk_change_log / risk_assessment: risk_id FK CASCADE → RESTRICT ────────────
-- Both are append-only history tables (see risk_schema.sql's FK ON DELETE
-- policy section for the full reasoning). CASCADE means either one silently
-- loses its rows the moment its risk is deleted — defeating the point of
-- keeping history. MySQL has neither "ALTER FOREIGN KEY" nor "DROP FOREIGN
-- KEY IF EXISTS", and rejects dropping and re-adding a constraint of the
-- same name within one ALTER TABLE ("Duplicate foreign key constraint
-- name") — so each rebuild is two separate guarded statements, both gated on
-- the same @needs_fix captured once up front, not rechecked after the DROP
-- (by then the constraint is gone, so a fresh check would never see CASCADE
-- and the ADD would wrongly no-op).
SET @needs_fix := (
  (SELECT DELETE_RULE FROM information_schema.REFERENTIAL_CONSTRAINTS
   WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'risk_change_log'
     AND CONSTRAINT_NAME = 'fk_change_log_risk') = 'CASCADE');

SET @sql := IF(@needs_fix, 'ALTER TABLE risk_change_log DROP FOREIGN KEY fk_change_log_risk', 'DO 0');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF(@needs_fix,
  'ALTER TABLE risk_change_log ADD CONSTRAINT fk_change_log_risk FOREIGN KEY (risk_id) REFERENCES risk(id) ON DELETE RESTRICT',
  'DO 0');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @needs_fix := (
  (SELECT DELETE_RULE FROM information_schema.REFERENTIAL_CONSTRAINTS
   WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'risk_assessment'
     AND CONSTRAINT_NAME = 'fk_risk_assessment_risk') = 'CASCADE');

SET @sql := IF(@needs_fix, 'ALTER TABLE risk_assessment DROP FOREIGN KEY fk_risk_assessment_risk', 'DO 0');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF(@needs_fix,
  'ALTER TABLE risk_assessment ADD CONSTRAINT fk_risk_assessment_risk FOREIGN KEY (risk_id) REFERENCES risk(id) ON DELETE RESTRICT',
  'DO 0');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
