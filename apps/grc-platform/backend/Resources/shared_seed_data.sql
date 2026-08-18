-- =============================================================================
-- GRC Platform — Shared Reference Data
-- Run this AFTER shared.sql, and before serving any traffic.
-- =============================================================================
--
-- Seeds the role catalogue, the privilege catalogue, and the mapping between
-- them, for both Risk Hub and Audit Hub. This is REFERENCE DATA, not sample
-- data: it is identical in every environment, it is coupled to the schema in
-- shared.sql and to the privilege constants in
-- backend/internal/shared/privilege/privilege.go, and the platform does not
-- function without it.
--
-- In particular, role.scope_basis is set ONLY here. Nothing in the Go code
-- writes it — grant_repo.go only ever reads it via COALESCE(scope_basis,'').
-- A role left with a NULL basis resolves to an empty one, and
-- grant.Set.SourceScopeIDs()/AssignmentScopeIDs() then match nothing, so a
-- team-scoped caller (e.g. a Risk Assigner on one register) sees ZERO risks:
-- handleListRisks takes its fail-closed empty-page branch and the grant axis
-- of riskVisibleToCaller never matches. Skipping this file does not degrade
-- the platform gracefully — it silently empties it for everyone except
-- GLOBAL holders and people named individually on a risk.
--
-- Prerequisites: shared.sql must already have run.
-- Run as: mysql -u <user> -p grc_platform < shared_seed_data.sql
--
-- Idempotent: safe to re-run. ON DUPLICATE KEY UPDATE keeps rows consistent,
-- and every migration statement is a no-op on a fresh database.
--
-- NOT INCLUDED — the bootstrap admin grant. On a fresh environment
-- user_role_grant is empty, so nobody holds MANAGE_USERS and nobody can grant
-- it. Seeding one grant is the only way in, and it names a real person, so it
-- is environment-specific rather than reference data. See the template at the
-- end of this file.
--
-- Role strategy:
--   Role names are owned by this platform — they no longer have to match any
--   identity provider's group string. Asgardeo authenticates users and nothing
--   more.
--
--   User-to-role assignment lives in user_role_grant, granted per scope. This
--   file seeds only the catalogue (which roles exist, and what each can do).
--   Everything else is granted by an admin through User Management, or by the
--   one-time backfill (cmd/backfill-grants).
--
--   role.module decides which scopes a role may be granted against:
--   RISK → GLOBAL or RISK_TEAM, AUDIT → GLOBAL or AUDIT_TEAM, SHARED → GLOBAL.
--
--   There is deliberately no all-privileges stand-in role. The former
--   'wso2-everyone' is deleted below: under scoped grants it would be a role
--   that is every module at once, holds every privilege, and would inevitably
--   be granted GLOBAL — defeating the entire model, and reaching production
--   because it is the easiest thing to grant when something does not work.
-- =============================================================================

USE grc_platform;

-- ── Migrations ────────────────────────────────────────────────────────────────
-- No-ops on a fresh DB. Existing role_privilege rows for retired privileges are
-- ignored by privilege.Store because it filters WHERE p.status = 'ACTIVE'.
UPDATE privilege SET status = 'INACTIVE' WHERE privilege_name = 'APPROVE_RISK';
UPDATE privilege SET status = 'INACTIVE' WHERE privilege_name = 'REJECT_RISK';

-- Risk Hub privileges renamed with a RISK_ prefix so they group apart from
-- Audit Hub privileges. Renamed in place (not dropped/reinserted) so existing
-- role_privilege rows keep referencing the same privilege_id.
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

-- Role rename: grc-platform-risk-admin is now grc-platform-risk-compliance-admin
-- (it absorbs Compliance's approve/reject/close authority, see below). The
-- Compliance Team and Management roles are merged with their Audit Hub
-- counterparts under shared names — renamed in place (not dropped/reinserted)
-- so existing role_privilege/user assignments keep referencing the same
-- role_id. No-ops on a fresh DB.
UPDATE `role` SET role_name = 'grc-platform-risk-compliance-admin' WHERE role_name = 'grc-platform-risk-admin';
UPDATE `role` SET role_name = 'grc-platform-compliance-team'       WHERE role_name = 'grc-platform-risk-compliance-team';
UPDATE `role` SET role_name = 'grc-platform-management'            WHERE role_name = 'grc-platform-risk-management';

-- grc-platform-compliance-team is being split back apart by hub: the Audit
-- Hub will get its own grc-platform-audit-compliance-team, which this file
-- does not seed, and the Risk Hub's half reclaims the grc-platform-risk-
-- compliance-team name it had before the merge above. Renamed in place for
-- the same reason as the other renames in this block — existing
-- role_privilege/user assignments keep referencing the same role_id. No-op on
-- a fresh DB, since the INSERT below creates the row with the new name
-- directly.
UPDATE `role` SET role_name = 'grc-platform-risk-compliance-team'  WHERE role_name = 'grc-platform-compliance-team';

-- grc-platform-compliance-team no longer approves/rejects/closes/escalates
-- Risk Hub risks or manages compliance references — that authority moved to
-- grc-platform-risk-compliance-admin. Deactivate any already-seeded grants so
-- a local DB seeded before this change doesn't retain them (the role_privilege
-- INSERT below only re-activates the privileges still in its list; it never
-- deactivates ones no longer there).
UPDATE role_privilege rp
JOIN   `role` r      ON r.id = rp.role_id
JOIN   privilege p   ON p.id = rp.privilege_id
SET    rp.is_active = FALSE
WHERE  r.role_name = 'grc-platform-risk-compliance-team'
  AND  p.privilege_name IN (
    'RISK_COMPLIANCE_APPROVE', 'RISK_COMPLIANCE_REJECT', 'RISK_CLOSE',
    'RISK_ESCALATE', 'RISK_MANAGE_COMPLIANCE_REFS'
  );

-- ── role ──────────────────────────────────────────────────────────────────────
-- scope_basis is the load-bearing column here — see the file header. It says
-- which dimension of a risk a grant on this role scopes by: SOURCE_REGISTER
-- (where the risk was raised) or ASSIGNMENT_TEAM (where the work was routed).
-- A risk_team row can be both a register and an assignment team, so the scope
-- id alone cannot say which sense was meant: "Risk Owner @ Asgardeo" means
-- risks ASSIGNED to Asgardeo, while "Risk Assigner @ Asgardeo" means risks
-- RAISED there. NULL is valid only for roles that are GLOBAL-only.
--
-- Note there is no longer a risk-action-owner role. Completing an action plan's
-- steps is authorised by being that plan's action_owner_id — the identity axis —
-- not by holding a role. An Action Owner may be any employee, including one with
-- no grants at all, which a role-based model could not express.
INSERT INTO `role` (role_name, description, module, scope_basis, status) VALUES
  ('grc-platform-risk-compliance-admin',
   'Risk Hub administrator. Full access to all risk privileges, including final compliance approval, rejection, and closure.',
   'RISK', 'SOURCE_REGISTER', 'ACTIVE'),
  ('grc-platform-risk-assigner',
   'Creates risks, drives them through the workflow, submits for approval, and records assessments.',
   'RISK', 'SOURCE_REGISTER', 'ACTIVE'),
  ('grc-platform-risk-owner',
   'Approves or rejects risks at the owner stage and records residual assessments.',
   'RISK', 'ASSIGNMENT_TEAM', 'ACTIVE'),
  ('grc-platform-risk-compliance-team',
   'Read-only oversight for the Risk Hub: views dashboards, analytics, and risk registers. Grant GLOBAL for org-wide oversight, or scope to specific registers. Audit Hub has its own counterpart, grc-platform-audit-compliance-team.',
   'RISK', 'SOURCE_REGISTER', 'ACTIVE'),
  ('grc-platform-management',
   'Reviews and approves or rejects risks at the management approval stage.',
   'RISK', 'SOURCE_REGISTER', 'ACTIVE'),
  ('grc-platform-admin',
   'Platform administrator. Manages users and role grants. SHARED, so it can only be granted GLOBAL — see the bootstrap grant template at the end of this file.',
   'SHARED', NULL, 'ACTIVE')
ON DUPLICATE KEY UPDATE
  description = VALUES(description),
  module      = VALUES(module),
  scope_basis = VALUES(scope_basis),
  status      = VALUES(status);

-- ── privilege ─────────────────────────────────────────────────────────────────
-- privilege_name must match the constants in
-- backend/internal/shared/privilege/privilege.go exactly.
INSERT INTO privilege (privilege_name, module, status) VALUES
  -- Risk Hub (22 privileges) — all prefixed RISK_ so they group together
  -- (visually and alphabetically) apart from the Audit Hub block below.
  ('RISK_VIEW_RISKS',              'RISK', 'ACTIVE'),
  -- RISK_VIEW_ALL_RISKS grants org-wide read visibility (dashboards,
  -- analytics, registers) without any edit/approve/close/escalate rights —
  -- distinct from RISK_VIEW_RISKS, which Risk Assigner/Owner also hold but
  -- are legitimately team-scoped for. See seesEveryRisk in risk_registers.go.
  ('RISK_VIEW_ALL_RISKS',          'RISK', 'ACTIVE'),
  -- RISK_VIEW_DASHBOARD gates the Dashboard nav item/route specifically,
  -- distinct from RISK_VIEW_RISKS (which gates the Registers list) — added so
  -- an Action Owner can hold list access without also getting the dashboard.
  ('RISK_VIEW_DASHBOARD',          'RISK', 'ACTIVE'),
  ('RISK_CREATE',                  'RISK', 'ACTIVE'),
  ('RISK_UPDATE',                  'RISK', 'ACTIVE'),
  ('RISK_SUBMIT',                  'RISK', 'ACTIVE'),
  ('RISK_CANCEL',                  'RISK', 'ACTIVE'),
  ('RISK_OWNER_APPROVE',           'RISK', 'ACTIVE'),
  ('RISK_MANAGEMENT_APPROVE',      'RISK', 'ACTIVE'),
  ('RISK_COMPLIANCE_APPROVE',      'RISK', 'ACTIVE'),
  ('RISK_OWNER_REJECT',            'RISK', 'ACTIVE'),
  ('RISK_MANAGEMENT_REJECT',       'RISK', 'ACTIVE'),
  ('RISK_COMPLIANCE_REJECT',       'RISK', 'ACTIVE'),
  ('RISK_COMPLETE',                'RISK', 'ACTIVE'),
  ('RISK_CLOSE',                   'RISK', 'ACTIVE'),
  ('RISK_ESCALATE',                'RISK', 'ACTIVE'),
  ('RISK_ASSESS',                  'RISK', 'ACTIVE'),
  ('RISK_MANAGE_TEAMS',            'RISK', 'ACTIVE'),
  ('RISK_MANAGE_SCORES',           'RISK', 'ACTIVE'),
  ('RISK_MANAGE_ACTION_PLANS',     'RISK', 'ACTIVE'),
  ('RISK_MANAGE_COMPLIANCE_REFS',  'RISK', 'ACTIVE'),
  ('RISK_VIEW_ANALYTICS',          'RISK', 'ACTIVE'),
  -- RETIRED with the MANAGEMENT action plan itself: an escalation is now
  -- answered with a comment, and additional plans are created by the Risk
  -- Assigner under RISK_MANAGE_ACTION_PLANS. Seeded INACTIVE rather than
  -- deleted so existing role_privilege rows keep a valid FK; privilege.Store
  -- filters on status = 'ACTIVE', so they simply stop resolving.
  ('RISK_CREATE_MANAGEMENT_ACTION_PLAN', 'RISK', 'INACTIVE'),
  ('RISK_COMPLETE_ACTION_STEPS',   'RISK', 'ACTIVE'),
  -- Audit Hub (14 privileges)
  ('VIEW_AUDITS',             'AUDIT', 'ACTIVE'),
  ('CREATE_AUDIT',            'AUDIT', 'ACTIVE'),
  ('UPDATE_AUDIT',            'AUDIT', 'ACTIVE'),
  ('MOVE_AUDIT_TO_FIELDWORK', 'AUDIT', 'ACTIVE'),
  ('SUBMIT_AUDIT_FOR_REVIEW', 'AUDIT', 'ACTIVE'),
  ('COMPLETE_AUDIT',          'AUDIT', 'ACTIVE'),
  ('MANAGE_CONTROLS',         'AUDIT', 'ACTIVE'),
  ('SUBMIT_EVIDENCE',         'AUDIT', 'ACTIVE'),
  ('REVIEW_EVIDENCE',         'AUDIT', 'ACTIVE'),
  ('MANAGE_POPULATION',       'AUDIT', 'ACTIVE'),
  ('ADD_COMMENT',             'AUDIT', 'ACTIVE'),
  ('MANAGE_ASSIGNMENTS',      'AUDIT', 'ACTIVE'),
  ('VIEW_TRAIL',              'AUDIT', 'ACTIVE'),
  ('MANAGE_FRAMEWORKS',       'AUDIT', 'ACTIVE'),
  -- Shared platform (1 privilege)
  -- Gates User Management: provisioning users and granting/revoking roles.
  -- SHARED, not AUDIT — it spans both hubs. Declared in Go long before it had a
  -- handler; the grant editor is what finally uses it.
  ('MANAGE_USERS',            'SHARED', 'ACTIVE')
ON DUPLICATE KEY UPDATE
  module = VALUES(module),
  status = VALUES(status);

-- ── role_privilege ────────────────────────────────────────────────────────────

-- grc-platform-admin → user and role-grant management only.
-- Deliberately narrow: this is the role that hands out authority, not one that
-- does risk work. A bootstrap admin grants themselves whatever else they need.
-- MANAGE_USERS is only ever granted GLOBAL: the role is SHARED, and the grant
-- service rejects any scoped grant of a role carrying it, since a
-- register-scoped admin granting roles would need an escalation rule (a
-- grantor may not exceed their own privileges in that scope) that does not
-- exist yet.
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN ('MANAGE_USERS') AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-admin'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-risk-compliance-admin → all 23 Risk Hub privileges
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'RISK_VIEW_RISKS', 'RISK_VIEW_ALL_RISKS', 'RISK_VIEW_DASHBOARD', 'RISK_CREATE', 'RISK_UPDATE', 'RISK_SUBMIT', 'RISK_CANCEL',
  'RISK_OWNER_APPROVE', 'RISK_MANAGEMENT_APPROVE', 'RISK_COMPLIANCE_APPROVE',
  'RISK_OWNER_REJECT', 'RISK_MANAGEMENT_REJECT', 'RISK_COMPLIANCE_REJECT',
  'RISK_COMPLETE', 'RISK_CLOSE', 'RISK_ESCALATE', 'RISK_ASSESS',
  'RISK_MANAGE_TEAMS', 'RISK_MANAGE_SCORES', 'RISK_MANAGE_ACTION_PLANS',
  'RISK_MANAGE_COMPLIANCE_REFS', 'RISK_VIEW_ANALYTICS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-risk-compliance-admin'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-risk-assigner → creates, edits, submits, cancels, completes, assesses
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'RISK_VIEW_RISKS', 'RISK_VIEW_DASHBOARD', 'RISK_CREATE', 'RISK_UPDATE', 'RISK_SUBMIT', 'RISK_CANCEL',
  'RISK_COMPLETE', 'RISK_ASSESS', 'RISK_MANAGE_ACTION_PLANS', 'RISK_VIEW_ANALYTICS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-risk-assigner'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-risk-owner → owner approval, rejection, and assessment
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'RISK_VIEW_RISKS', 'RISK_VIEW_DASHBOARD', 'RISK_OWNER_APPROVE', 'RISK_OWNER_REJECT', 'RISK_ASSESS',
  'RISK_VIEW_ANALYTICS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-risk-owner'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-risk-compliance-team → Risk Hub: view-only, org-wide
-- (dashboards, analytics, registers) — no edit/approve/close/escalate.
-- Approve, reject, and close moved to grc-platform-risk-compliance-admin.
-- Audit Hub's counterpart (grc-platform-audit-compliance-team) is a separate
-- role with its own privilege grants, owned separately.
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'RISK_VIEW_RISKS', 'RISK_VIEW_ALL_RISKS', 'RISK_VIEW_DASHBOARD', 'RISK_VIEW_ANALYTICS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-risk-compliance-team'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-management → Risk Hub: management approval/rejection, and
-- commenting on an escalated high risk. That comment carries no privilege of
-- its own: handleEscalationComment authorises on being the risk's named
-- management_approver_id, so it is granted nothing here.
--
-- ⚠ OPEN: this role is module = 'RISK', so it can only be granted GLOBAL or
-- scoped to a risk_team. It was previously described as shared with the Audit
-- Hub under one name, which the module column no longer permits — a role belongs
-- to one module so that "which scopes may this be granted against" has a single
-- answer. Either split it by hub (the precedent already set by
-- grc-platform-risk-compliance-team / grc-platform-audit-compliance-team), or
-- make it SHARED and accept GLOBAL-only granting. Settle this with the Audit
-- Hub developer before his migration; nothing here depends on the outcome.
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'RISK_VIEW_RISKS', 'RISK_VIEW_DASHBOARD', 'RISK_MANAGEMENT_APPROVE', 'RISK_MANAGEMENT_REJECT',
  'RISK_VIEW_ANALYTICS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-management'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- ── Retire roles and privileges replaced by user_role_grant ───────────────────
-- No-ops on a fresh DB. role_privilege must go first — its FKs to role and
-- privilege are RESTRICT, so a role with live mappings cannot be deleted.

-- RISK_COMPLETE_ACTION_STEPS was only ever a proxy for "is this plan's
-- action_owner_id", which is now checked directly. Marked INACTIVE rather than
-- deleted so any row still referencing it is recognisable.
DELETE rp FROM role_privilege rp
JOIN   privilege p ON p.id = rp.privilege_id
WHERE  p.privilege_name = 'RISK_COMPLETE_ACTION_STEPS';
UPDATE privilege SET status = 'INACTIVE' WHERE privilege_name = 'RISK_COMPLETE_ACTION_STEPS';

-- grc-platform-risk-action-owner: an Action Owner may be any employee, with no
-- grants at all. Their access comes from being named on the plan, so the role
-- has nothing left to confer.
DELETE rp FROM role_privilege rp
JOIN   `role` r ON r.id = rp.role_id
WHERE  r.role_name = 'grc-platform-risk-action-owner';
DELETE FROM `role` WHERE role_name = 'grc-platform-risk-action-owner';

-- wso2-everyone: an all-privileges stand-in cannot coexist with scoped grants.
-- See the header note.
DELETE rp FROM role_privilege rp
JOIN   `role` r ON r.id = rp.role_id
WHERE  r.role_name = 'wso2-everyone';
DELETE FROM `role` WHERE role_name = 'wso2-everyone';

-- ── Verify ────────────────────────────────────────────────────────────────────
SELECT 'role'            AS `table`, COUNT(*) AS `count` FROM `role`
UNION ALL
SELECT 'privilege',      COUNT(*) FROM privilege WHERE status = 'ACTIVE'
UNION ALL
SELECT 'role_privilege', COUNT(*) FROM role_privilege WHERE is_active = TRUE;

-- Every RISK role must have a scope_basis. A row here means grants on that role
-- resolve to an empty scope, and its holders will see no risks at all — see the
-- file header. Expected: 0 rows.
SELECT role_name, module, 'MISSING scope_basis — holders will see nothing' AS problem
FROM   `role`
WHERE  module = 'RISK' AND status = 'ACTIVE' AND scope_basis IS NULL;

-- =============================================================================
-- Bootstrap admin grant — ENVIRONMENT-SPECIFIC, run separately
-- =============================================================================
-- Deliberately NOT executed by this file: it names a real person, so it differs
-- per environment and does not belong in tracked reference data.
--
-- On a fresh environment user_role_grant is empty, so nobody holds MANAGE_USERS
-- and nobody can grant it — the platform would be permanently locked. One seeded
-- grant is the way in, and it is the ONLY authorization path that does not
-- originate from user_role_grant itself. There is no env-var break-glass and no
-- identity-provider fallback: either would mean "SELECT * FROM user_role_grant"
-- no longer answers "who are my admins".
--
-- Copy the block below, set the email, and run it once per environment.
--
--   -- The explicit COLLATE is required, not decorative: a user-defined variable
--   -- takes the connection's collation (utf8mb4_0900_ai_ci by default on MySQL
--   -- 8/9), while user.email is utf8mb4_unicode_ci. Both have the same
--   -- coercibility, so comparing them without this raises "Illegal mix of
--   -- collations".
--   SET @bootstrap_admin_email = _utf8mb4'CHANGE-ME@example.com' COLLATE utf8mb4_unicode_ci;
--   SET @bootstrap_admin_name  = 'Bootstrap Admin';
--
--   -- The user row is created here if absent. Nothing else creates it: users
--   -- are provisioned explicitly via POST /users/resolve, not automatically on
--   -- login, so on a fresh database there are no user rows and the grant below
--   -- would silently match nothing. display_name is a placeholder; it is
--   -- overwritten from the HR entity on first use.
--   INSERT IGNORE INTO `user` (email, display_name, user_type, status, created_by)
--   VALUES (@bootstrap_admin_email, @bootstrap_admin_name, 'INTERNAL', 'ACTIVE', 'System');
--
--   INSERT INTO user_role_grant (user_id, role_id, scope_type, scope_id, created_by)
--   SELECT u.id, r.id, 'GLOBAL', 0, 'System'
--   FROM   `user` u
--   JOIN   `role` r ON r.role_name = 'grc-platform-admin'
--   WHERE  u.email = @bootstrap_admin_email
--   ON DUPLICATE KEY UPDATE status = 'ACTIVE';
--
--   -- Sanity check: 0 rows means the email matched no user, and NOBODY can
--   -- grant roles in this environment. Fix before proceeding.
--   SELECT u.email AS bootstrap_admin, r.role_name, g.scope_type
--   FROM   user_role_grant g
--   JOIN   `user` u ON u.id = g.user_id
--   JOIN   `role` r ON r.id = g.role_id
--   WHERE  r.role_name = 'grc-platform-admin' AND g.status = 'ACTIVE';
-- =============================================================================
