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
-- NO MIGRATIONS. Every target database is already at the current schema and
-- the current role/privilege naming. This file only asserts the reference
-- rows as they should be now — it never renames, retires, backfills, or
-- remediates existing rows. Schema and data migrations are handled outside
-- this repo. Do not re-add UPDATE/DELETE "migration" blocks here.
--
-- In particular, role.scope_basis is set ONLY here. Nothing in the Go code
-- writes it — grant_repo.go only ever reads it via COALESCE(scope_basis,'').
-- A role left with a NULL basis resolves to an empty one, and
-- grant.Set.SourceScopeIDs()/AssignmentScopeIDs() then match nothing, so a
-- team-scoped caller (e.g. a Risk Assigner on one register) sees ZERO risks.
-- Skipping this file does not degrade the platform gracefully — it silently
-- empties it for everyone except GLOBAL holders and people named individually
-- on a risk.
--
-- Prerequisites: shared.sql must already have run.
-- Run as: mysql -u <user> -p grc_platform < shared_seed_data.sql
--
-- Idempotent: safe to re-run. Every statement is INSERT ... ON DUPLICATE KEY
-- UPDATE and only asserts row state.
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
--   Everything else is granted by an admin through User Management.
--
--   role.module decides which scopes a role may be granted against:
--   RISK → GLOBAL or RISK_TEAM, AUDIT → GLOBAL or AUDIT_TEAM, SHARED → GLOBAL.
--
--   There is deliberately no all-privileges stand-in role: under scoped grants
--   it would be every module at once, hold every privilege, and inevitably be
--   granted GLOBAL — defeating the entire model.
-- =============================================================================

USE grc_platform;

-- ── role ──────────────────────────────────────────────────────────────────────
-- scope_basis is the load-bearing column here — see the file header. It says
-- which dimension of a risk a grant on this role scopes by: SOURCE_REGISTER
-- (where the risk was raised) or ASSIGNMENT_TEAM (where the work was routed).
-- A risk_team row can be both a register and an assignment team, so the scope
-- id alone cannot say which sense was meant: "Risk Owner @ Asgardeo" means
-- risks ASSIGNED to Asgardeo, while "Risk Assigner @ Asgardeo" means risks
-- RAISED there. NULL is valid only for roles that are GLOBAL-only.
--
-- There is no risk-action-owner role. Completing an action plan's steps is
-- authorised by being that plan's action_owner_id — the identity axis — not by
-- holding a role. An Action Owner may be any employee, including one with no
-- grants at all, which a role-based model could not express.
--
-- grc-platform-risk-management is RISK-only: management approval/rejection in
-- the Risk Hub. grc-platform-audit-management is its Audit Hub counterpart,
-- module='AUDIT', so it can be granted AUDIT_TEAM-scoped, not just GLOBAL.
--
-- grc-platform-audit-compliance-team is module='AUDIT' too, but it carries
-- AUDIT_REVIEW_EVIDENCE (globalOnlyPrivileges in grant_service.go), so it can
-- only ever be granted GLOBAL.
--
-- assignable_user_type declares which kind of person a role may be granted
-- to (INTERNAL/EXTERNAL identities live in separate Asgardeo organisations).
-- Every role here is INTERNAL except grc-platform-audit-external-auditor.
INSERT INTO `role` (role_name, description, module, scope_basis, assignable_user_type, status) VALUES
  ('grc-platform-risk-compliance-admin',
   'Risk Hub administrator. Full access to all risk privileges, including final compliance approval, rejection, and closure.',
   'RISK', 'SOURCE_REGISTER', 'INTERNAL', 'ACTIVE'),
  ('grc-platform-risk-assigner',
   'Creates risks, drives them through the workflow, submits for approval, and records assessments.',
   'RISK', 'SOURCE_REGISTER', 'INTERNAL', 'ACTIVE'),
  ('grc-platform-risk-owner',
   'Approves or rejects risks at the owner stage and records residual assessments.',
   'RISK', 'ASSIGNMENT_TEAM', 'INTERNAL', 'ACTIVE'),
  ('grc-platform-risk-compliance-team',
   'Read-only oversight for the Risk Hub: views dashboards, analytics, and risk registers. Grant GLOBAL for org-wide oversight, or scope to specific registers. Audit Hub has its own counterpart, grc-platform-audit-compliance-team.',
   'RISK', 'SOURCE_REGISTER', 'INTERNAL', 'ACTIVE'),
  ('grc-platform-risk-management',
   'Management approval/rejection in the Risk Hub. Grant GLOBAL for org-wide, or scope to a register. Audit Hub has its own counterpart, grc-platform-audit-management.',
   'RISK', 'SOURCE_REGISTER', 'INTERNAL', 'ACTIVE'),
  ('grc-platform-admin',
   'Platform administrator. Manages users, role grants, and the Admin Console reference-data screens (Risk Teams/Categories/Compliance References; Audit Hub equivalents pending). SHARED, so it can only be granted GLOBAL — see the bootstrap grant template at the end of this file.',
   'SHARED', NULL, 'INTERNAL', 'ACTIVE'),
  ('grc-platform-audit-compliance-admin',
   'Audit Compliance Admin - full control of the Audit Hub',
   'AUDIT', NULL, 'INTERNAL', 'ACTIVE'),
  ('grc-platform-audit-compliance-team',
   'Audit Compliance Team - submit (any) + internal review, org-wide read',
   'AUDIT', NULL, 'INTERNAL', 'ACTIVE'),
  ('grc-platform-audit-internal-team',
   'Audit Internal Team - submit evidence for own team only',
   'AUDIT', NULL, 'INTERNAL', 'ACTIVE'),
  ('grc-platform-audit-management',
   'Audit Management - org-wide or team-scoped read-only oversight. Grant GLOBAL for org-wide, or scope to one audit team for a team-scoped read-only lead.',
   'AUDIT', NULL, 'INTERNAL', 'ACTIVE'),
  ('grc-platform-audit-external-auditor',
   'Audit External Auditor - validate + select sample for assigned controls',
   'AUDIT', NULL, 'EXTERNAL', 'ACTIVE')
ON DUPLICATE KEY UPDATE
  description            = VALUES(description),
  module                 = VALUES(module),
  scope_basis            = VALUES(scope_basis),
  assignable_user_type   = VALUES(assignable_user_type),
  status                 = VALUES(status);

-- ── privilege ─────────────────────────────────────────────────────────────────
-- privilege_name must match the constants in
-- backend/internal/shared/privilege/privilege.go exactly.
INSERT INTO privilege (privilege_name, module, status) VALUES
  -- Risk Hub — all prefixed RISK_ so they group together (visually and
  -- alphabetically) apart from the Audit Hub block below.
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
  -- Seeded INACTIVE: an escalation is now answered with a comment, and
  -- additional plans are created by the Risk Assigner under
  -- RISK_MANAGE_ACTION_PLANS. Kept in the catalogue so any role_privilege row
  -- still referencing it keeps a valid FK; privilege.Store filters on
  -- status = 'ACTIVE', so it simply stops resolving.
  ('RISK_CREATE_MANAGEMENT_ACTION_PLAN', 'RISK', 'INACTIVE'),
  -- Seeded INACTIVE: completing an action plan's steps is authorised by being
  -- the plan's action_owner_id, checked directly — not by holding a privilege.
  ('RISK_COMPLETE_ACTION_STEPS',   'RISK', 'INACTIVE'),
  -- Audit Hub (12 privileges). Coarse booleans only — row scope (all / team /
  -- owned / assigned) is DERIVED from these at request time, never expressed
  -- here — see deriveScopes in internal/audit/handler/dashboard.go. In
  -- particular AUDIT_VIEW_ALL_AUDITS is the org-wide-read signal: held GLOBAL
  -- it means `all` scope; a holder of AUDIT_SUBMIT_EVIDENCE without it is
  -- `owned`; a holder of AUDIT_VALIDATE_EVIDENCE without it is `assigned`.
  ('AUDIT_VIEW_AUDITS',            'AUDIT', 'ACTIVE'),
  ('AUDIT_VIEW_ALL_AUDITS',        'AUDIT', 'ACTIVE'),
  ('AUDIT_CREATE_AUDIT',           'AUDIT', 'ACTIVE'),
  ('AUDIT_UPDATE_AUDIT',           'AUDIT', 'ACTIVE'),
  ('AUDIT_MANAGE_CONTROLS',        'AUDIT', 'ACTIVE'),
  ('AUDIT_MANAGE_FRAMEWORKS',      'AUDIT', 'ACTIVE'),
  ('AUDIT_SUBMIT_EVIDENCE',        'AUDIT', 'ACTIVE'),
  ('AUDIT_REVIEW_EVIDENCE',        'AUDIT', 'ACTIVE'),
  -- ValidateEvidence and SelectSample are auditor-only actions, layered on top
  -- of the assigned-auditor scope check (requireAssignedAuditor) so the grant
  -- is visible in the matrix and the frontend can render off can(...).
  ('AUDIT_VALIDATE_EVIDENCE',      'AUDIT', 'ACTIVE'),
  ('AUDIT_SELECT_SAMPLE',          'AUDIT', 'ACTIVE'),
  ('AUDIT_ADD_COMMENT',            'AUDIT', 'ACTIVE'),
  -- Gates internal-only control comments (hidden from external auditors).
  ('AUDIT_VIEW_INTERNAL_COMMENTS', 'AUDIT', 'ACTIVE'),
  -- Shared platform (3 privileges) — Admin Console's gates, all held only
  -- by grc-platform-admin (see role_privilege below): one consistent
  -- authority boundary for the whole console.
  --
  -- MANAGE_USERS gates User Management: provisioning users and
  -- granting/revoking roles.
  ('MANAGE_USERS',            'SHARED', 'ACTIVE'),
  -- MANAGE_RISK_HUB gates the Risk Teams / Risk Categories / Compliance
  -- References / Risk Scores screens. Module tagged RISK for categorisation
  -- (grouping alongside the other Risk Hub privileges above) — this does NOT
  -- make it grantable team-scoped in practice, because it is only ever
  -- granted to grc-platform-admin, which is module SHARED with a NULL
  -- scope_basis, forcing every grant of that role GLOBAL regardless of what
  -- module its individual privileges carry. Supersedes
  -- RISK_MANAGE_TEAMS/SCORES/COMPLIANCE_REFS for gating purposes; those three
  -- are not granted to grc-platform-risk-compliance-admin (see role_privilege
  -- below).
  ('MANAGE_RISK_HUB',         'RISK',   'ACTIVE'),
  -- MANAGE_AUDIT_HUB is the Audit Hub equivalent — seeded and granted now so
  -- the console's shape is right, even though the screens it will gate
  -- (Audit Teams/Frameworks/Products) are a stubbed, later phase. Same
  -- GLOBAL-only-in-practice reasoning as MANAGE_RISK_HUB.
  ('MANAGE_AUDIT_HUB',        'AUDIT',  'ACTIVE')
ON DUPLICATE KEY UPDATE
  module = VALUES(module),
  status = VALUES(status);

-- ── role_privilege ────────────────────────────────────────────────────────────

-- grc-platform-admin → user/role-grant management, plus the Admin Console's
-- reference-data screens (Risk Hub, Audit Hub — the latter stubbed for now).
-- Deliberately narrow beyond that: this is the role that hands out authority
-- and administers the platform's own lookup tables, not one that does risk or
-- audit work. A bootstrap admin grants themselves whatever else they need.
-- All three privileges are only ever granted GLOBAL: the role is SHARED, and
-- the grant service rejects any scoped grant of a role carrying it.
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN ('MANAGE_USERS', 'MANAGE_RISK_HUB', 'MANAGE_AUDIT_HUB') AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-admin'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-risk-compliance-admin → every Risk Hub privilege except
-- RISK_MANAGE_TEAMS/SCORES/COMPLIANCE_REFS, which are gated by the Admin
-- Console's MANAGE_RISK_HUB (held only by grc-platform-admin).
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'RISK_VIEW_RISKS', 'RISK_VIEW_ALL_RISKS', 'RISK_VIEW_DASHBOARD', 'RISK_CREATE', 'RISK_UPDATE', 'RISK_SUBMIT', 'RISK_CANCEL',
  'RISK_OWNER_APPROVE', 'RISK_MANAGEMENT_APPROVE', 'RISK_COMPLIANCE_APPROVE',
  'RISK_OWNER_REJECT', 'RISK_MANAGEMENT_REJECT', 'RISK_COMPLIANCE_REJECT',
  'RISK_COMPLETE', 'RISK_CLOSE', 'RISK_ESCALATE', 'RISK_ASSESS',
  'RISK_MANAGE_ACTION_PLANS', 'RISK_VIEW_ANALYTICS'
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
-- Approve, reject, and close live on grc-platform-risk-compliance-admin.
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

-- grc-platform-risk-management → Risk Hub: management approval/rejection only.
-- grc-platform-audit-management is the Audit Hub counterpart. Commenting on an
-- escalated high risk carries no privilege of its own: handleEscalationComment
-- authorises on being the risk's named management_approver_id.
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'RISK_VIEW_RISKS', 'RISK_VIEW_DASHBOARD', 'RISK_MANAGEMENT_APPROVE', 'RISK_MANAGEMENT_REJECT',
  'RISK_VIEW_ANALYTICS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-risk-management'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-audit-management → Audit Hub: org-wide (or, once granted
-- AUDIT_TEAM-scoped, team-wide) read-only oversight + comment. module='AUDIT'
-- (see the role INSERT above) is what makes AUDIT_TEAM scoping possible.
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'AUDIT_VIEW_AUDITS', 'AUDIT_VIEW_ALL_AUDITS', 'AUDIT_ADD_COMMENT', 'AUDIT_VIEW_INTERNAL_COMMENTS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-audit-management'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-audit-compliance-admin — everything (12)
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'AUDIT_VIEW_AUDITS', 'AUDIT_VIEW_ALL_AUDITS', 'AUDIT_CREATE_AUDIT', 'AUDIT_UPDATE_AUDIT',
  'AUDIT_MANAGE_CONTROLS', 'AUDIT_MANAGE_FRAMEWORKS', 'AUDIT_SUBMIT_EVIDENCE', 'AUDIT_REVIEW_EVIDENCE',
  'AUDIT_VALIDATE_EVIDENCE', 'AUDIT_SELECT_SAMPLE', 'AUDIT_ADD_COMMENT', 'AUDIT_VIEW_INTERNAL_COMMENTS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-audit-compliance-admin'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-audit-compliance-team — org-wide read, submit (any), internal review, comment (6)
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'AUDIT_VIEW_AUDITS', 'AUDIT_VIEW_ALL_AUDITS', 'AUDIT_SUBMIT_EVIDENCE', 'AUDIT_REVIEW_EVIDENCE',
  'AUDIT_ADD_COMMENT', 'AUDIT_VIEW_INTERNAL_COMMENTS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-audit-compliance-team'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-audit-internal-team — submit (owned controls only) + comment (4).
-- No VIEW_ALL_AUDITS -> owned scope.
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'AUDIT_VIEW_AUDITS', 'AUDIT_SUBMIT_EVIDENCE', 'AUDIT_ADD_COMMENT', 'AUDIT_VIEW_INTERNAL_COMMENTS'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-audit-internal-team'
ON DUPLICATE KEY UPDATE is_active = TRUE;

-- grc-platform-audit-external-auditor — validate + select sample (assigned) + comment (4).
-- No VIEW_INTERNAL_COMMENTS: internal comments are hidden from auditors.
INSERT INTO role_privilege (role_id, privilege_id, is_active)
SELECT r.id, p.id, TRUE
FROM   `role` r
JOIN   privilege p ON p.privilege_name IN (
  'AUDIT_VIEW_AUDITS', 'AUDIT_VALIDATE_EVIDENCE', 'AUDIT_SELECT_SAMPLE', 'AUDIT_ADD_COMMENT'
) AND p.status = 'ACTIVE'
WHERE  r.role_name = 'grc-platform-audit-external-auditor'
ON DUPLICATE KEY UPDATE is_active = TRUE;

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
-- Copy the block below, set the uuid, and run it once per environment.
--
-- The uuid is the bootstrap admin's Asgardeo `sub` claim — the `user` table's
-- only identity (it stores no email or display name; see shared.sql). Resolve
-- it against Asgardeo/SCIM ahead of time, the same way the identity directory
-- would — there is no email to match on here to derive it from.
--
--   SET @bootstrap_admin_uuid = 'CHANGE-ME-asgardeo-sub-claim';
--
--   -- The user row is created here if absent. Nothing else creates it: users
--   -- are provisioned explicitly via POST /users/resolve, not automatically on
--   -- login, so on a fresh database there are no user rows and the grant below
--   -- would silently match nothing.
--   INSERT IGNORE INTO `user` (uuid, user_type, status, created_by)
--   VALUES (@bootstrap_admin_uuid, 'INTERNAL', 'ACTIVE', 'System');
--
--   INSERT INTO user_role_grant (user_id, role_id, scope_type, scope_id, created_by)
--   SELECT u.id, r.id, 'GLOBAL', 0, 'System'
--   FROM   `user` u
--   JOIN   `role` r ON r.role_name = 'grc-platform-admin'
--   WHERE  u.uuid = @bootstrap_admin_uuid
--   ON DUPLICATE KEY UPDATE status = 'ACTIVE';
--
--   -- Sanity check: 0 rows means the uuid matched no user, and NOBODY can
--   -- grant roles in this environment. Fix before proceeding.
--   SELECT u.uuid AS bootstrap_admin, r.role_name, g.scope_type
--   FROM   user_role_grant g
--   JOIN   `user` u ON u.id = g.user_id
--   JOIN   `role` r ON r.id = g.role_id
--   WHERE  r.role_name = 'grc-platform-admin' AND g.status = 'ACTIVE';
-- =============================================================================
