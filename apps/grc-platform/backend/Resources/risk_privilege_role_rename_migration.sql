-- =============================================================================
-- GRC Platform — Risk Hub Privilege/Role Rename Migration
-- Run AFTER shared.sql (schema) and AFTER deploying this PR's backend code —
-- BEFORE anyone tries to use Risk Hub against a database that predates it.
--
-- This PR renamed every Risk Hub privilege_name to a RISK_-prefixed form
-- (e.g. VIEW_RISKS → RISK_VIEW_RISKS), merged grc-platform-risk-admin into
-- grc-platform-risk-compliance-admin, and merged the Risk Hub's own
-- compliance-team/management roles into shared Audit Hub role names. The
-- code's privilege/role checks match strings exactly (privilege.Store,
-- role_privilege joins), so an already-deployed database that still has the
-- OLD names will not resolve any Risk Hub privilege for anyone until this
-- runs.
--
-- Run as: mysql -u <user> -p grc_platform < risk_privilege_role_rename_migration.sql
--
-- Idempotent / safe on a fresh database: every statement is a no-op UPDATE
-- when there are no rows with the old name left to match.
--
-- Renamed in place (UPDATE, not drop/reinsert) throughout, so existing
-- role_privilege and user-facing references keep pointing at the same
-- privilege_id/role_id — nothing else needs to change alongside this.
-- =============================================================================

USE grc_platform;

-- ── privilege renames ────────────────────────────────────────────────────────
-- Risk Hub privileges renamed with a RISK_ prefix so they group apart from
-- Audit Hub privileges.
UPDATE privilege SET privilege_name = 'RISK_VIEW_RISKS'                     WHERE privilege_name = 'VIEW_RISKS';
UPDATE privilege SET privilege_name = 'RISK_VIEW_DASHBOARD'                 WHERE privilege_name = 'VIEW_RISK_DASHBOARD';
UPDATE privilege SET privilege_name = 'RISK_CREATE'                         WHERE privilege_name = 'CREATE_RISK';
UPDATE privilege SET privilege_name = 'RISK_UPDATE'                         WHERE privilege_name = 'UPDATE_RISK';
UPDATE privilege SET privilege_name = 'RISK_SUBMIT'                         WHERE privilege_name = 'SUBMIT_RISK';
UPDATE privilege SET privilege_name = 'RISK_CANCEL'                         WHERE privilege_name = 'CANCEL_RISK';
UPDATE privilege SET privilege_name = 'RISK_OWNER_APPROVE'                  WHERE privilege_name = 'OWNER_APPROVE_RISK';
UPDATE privilege SET privilege_name = 'RISK_MANAGEMENT_APPROVE'             WHERE privilege_name = 'MANAGEMENT_APPROVE_RISK';
UPDATE privilege SET privilege_name = 'RISK_COMPLIANCE_APPROVE'             WHERE privilege_name = 'COMPLIANCE_APPROVE_RISK';
UPDATE privilege SET privilege_name = 'RISK_OWNER_REJECT'                   WHERE privilege_name = 'OWNER_REJECT_RISK';
UPDATE privilege SET privilege_name = 'RISK_MANAGEMENT_REJECT'              WHERE privilege_name = 'MANAGEMENT_REJECT_RISK';
UPDATE privilege SET privilege_name = 'RISK_COMPLIANCE_REJECT'              WHERE privilege_name = 'COMPLIANCE_REJECT_RISK';
UPDATE privilege SET privilege_name = 'RISK_COMPLETE'                       WHERE privilege_name = 'COMPLETE_RISK';
UPDATE privilege SET privilege_name = 'RISK_CLOSE'                          WHERE privilege_name = 'CLOSE_RISK';
UPDATE privilege SET privilege_name = 'RISK_ESCALATE'                       WHERE privilege_name = 'ESCALATE_RISK';
UPDATE privilege SET privilege_name = 'RISK_ASSESS'                         WHERE privilege_name = 'ASSESS_RISK';
UPDATE privilege SET privilege_name = 'RISK_MANAGE_TEAMS'                   WHERE privilege_name = 'MANAGE_TEAMS';
UPDATE privilege SET privilege_name = 'RISK_MANAGE_SCORES'                  WHERE privilege_name = 'MANAGE_RISK_SCORES';
UPDATE privilege SET privilege_name = 'RISK_MANAGE_ACTION_PLANS'            WHERE privilege_name = 'MANAGE_ACTION_PLANS';
UPDATE privilege SET privilege_name = 'RISK_MANAGE_COMPLIANCE_REFS'         WHERE privilege_name = 'MANAGE_COMPLIANCE_REFS';
UPDATE privilege SET privilege_name = 'RISK_VIEW_ANALYTICS'                 WHERE privilege_name = 'VIEW_ANALYTICS';
UPDATE privilege SET privilege_name = 'RISK_CREATE_MANAGEMENT_ACTION_PLAN'  WHERE privilege_name = 'CREATE_MANAGEMENT_ACTION_PLAN_RISK';
UPDATE privilege SET privilege_name = 'RISK_COMPLETE_ACTION_STEPS'          WHERE privilege_name = 'COMPLETE_ACTION_STEPS_RISK';

-- ── role renames ─────────────────────────────────────────────────────────────
-- grc-platform-risk-admin absorbs Compliance's approve/reject/close authority
-- and is renamed to grc-platform-risk-compliance-admin. The Compliance Team
-- and Management roles are merged with their Audit Hub counterparts under
-- shared names.
UPDATE `role` SET role_name = 'grc-platform-risk-compliance-admin' WHERE role_name = 'grc-platform-risk-admin';
UPDATE `role` SET role_name = 'grc-platform-compliance-team'       WHERE role_name = 'grc-platform-risk-compliance-team';
UPDATE `role` SET role_name = 'grc-platform-management'            WHERE role_name = 'grc-platform-risk-management';

-- ── role_privilege scope reduction ──────────────────────────────────────────
-- grc-platform-compliance-team no longer approves/rejects/closes/escalates
-- Risk Hub risks or manages compliance references — that authority moved to
-- grc-platform-risk-compliance-admin above. Deactivate any already-seeded
-- grants so a database seeded before this PR doesn't retain them.
UPDATE role_privilege rp
JOIN   `role` r      ON r.id = rp.role_id
JOIN   privilege p   ON p.id = rp.privilege_id
SET    rp.is_active = FALSE
WHERE  r.role_name = 'grc-platform-compliance-team'
  AND  p.privilege_name IN (
    'RISK_COMPLIANCE_APPROVE', 'RISK_COMPLIANCE_REJECT', 'RISK_CLOSE',
    'RISK_ESCALATE', 'RISK_MANAGE_COMPLIANCE_REFS'
  );
