-- =============================================================================
-- GRC Platform — Audit Hub RBAC Seed
-- Run AFTER shared.sql (needs the role, privilege, role_privilege,
-- user_role_grant tables — see shared.sql's role.module/scope_basis columns).
-- =============================================================================
--
-- Seeds the five Audit Hub roles, the twelve AUDIT_* privileges, and the
-- role -> privilege grants that back the authorization matrix.
--
-- Idempotent: safe to re-run. Roles/privileges upsert on their unique name;
-- grants upsert on (role_id, privilege_id) and are matched by name, so the
-- auto-increment ids never need to be known in advance. Re-running re-activates
-- anything that was soft-deactivated, so this file is the source of truth.
--
-- Role names: no longer required to match an Asgardeo group string. Role
-- membership is granted in our own database (user_role_grant), resolved fresh
-- on every request — see docs/new/Audit-Role-Grant-Migration-Design.md. The
-- "-stg" suffix and its per-environment duplication are gone with it: these
-- are internal database identifiers now, the same in every environment, and
-- nothing in Go or TSX references them by string.
--
-- module='AUDIT' on every role here (role.module, shared.sql) constrains which
-- scopes it may be granted against to GLOBAL or AUDIT_TEAM — see shared.sql's
-- table comment. scope_basis is left NULL (its default): audit_control has
-- exactly one team column, so the risk module's SOURCE_REGISTER /
-- ASSIGNMENT_TEAM ambiguity does not exist here.
--
-- Scope note: privileges are coarse booleans only. Row scope (all / team /
-- owned / assigned) is DERIVED from the caller's grants at request time, not
-- expressed here — see deriveScopes in internal/audit/handler/dashboard.go. In
-- particular AUDIT_VIEW_ALL_AUDITS is the org-wide-read signal: held GLOBAL it
-- means `all` scope; held on an AUDIT_TEAM grant it means `team` scope for that
-- team (management only — see grc-platform-audit-management below). A holder
-- of AUDIT_SUBMIT_EVIDENCE without it is `owned`; a holder of
-- AUDIT_VALIDATE_EVIDENCE without it is `assigned`.
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Roles (all five — including Management)
--
-- grc-platform-audit-management is a NEW role, distinct from
-- shared_seed_data.sql's grc-platform-management (module='RISK'), which stays
-- as-is on the risk side. Splitting rather than sharing one SHARED role is
-- what makes AUDIT_TEAM-scoped team-lead grants possible: a SHARED role is
-- GLOBAL-only by construction (no single team table its scope could point
-- at), which would make a team lead impossible to express.
-- -----------------------------------------------------------------------------
INSERT INTO `role` (role_name, description, module, status, created_by, updated_by) VALUES
  ('grc-platform-audit-compliance-admin',  'Audit Compliance Admin - full control of the Audit Hub',                  'AUDIT', 'ACTIVE', 'system', 'system'),
  ('grc-platform-audit-compliance-team',   'Audit Compliance Team - submit (any) + internal review, org-wide read',   'AUDIT', 'ACTIVE', 'system', 'system'),
  ('grc-platform-audit-internal-team',     'Audit Internal Team - submit evidence for own team only',                 'AUDIT', 'ACTIVE', 'system', 'system'),
  ('grc-platform-audit-external-auditor',  'Audit External Auditor - validate + select sample for assigned controls', 'AUDIT', 'ACTIVE', 'system', 'system'),
  ('grc-platform-management',        'Audit Management - org-wide read at GLOBAL, or team lead when granted at AUDIT_TEAM scope', 'AUDIT', 'ACTIVE', 'system', 'system')
ON DUPLICATE KEY UPDATE
  description = VALUES(description),
  module      = VALUES(module),
  status      = 'ACTIVE',
  updated_by  = VALUES(updated_by);

-- Retire the pre-migration "-stg" role rows. They matched an Asgardeo group
-- string that nothing reads anymore (see middleware/auth.go's grant
-- resolution); leaving them ACTIVE would let them keep showing up as
-- grantable roles in GET /roles / a future grant admin UI. INACTIVE, not
-- deleted: role.id is FK'd from any historical user_role_grant row, and
-- role_privilege's FK is ON DELETE RESTRICT.
UPDATE `role` SET status = 'INACTIVE', updated_by = 'system'
WHERE role_name COLLATE utf8mb4_bin IN (
  'grc-platform-audit-compliance-admin-stg',
  'grc-platform-audit-compliance-team-stg',
  'grc-platform-audit-internal-team-stg',
  'grc-platform-audit-external-auditor-stg',
  'grc-platform-management-stg'
);

-- -----------------------------------------------------------------------------
-- Privileges (module = AUDIT) — twelve
-- -----------------------------------------------------------------------------
INSERT INTO `privilege` (privilege_name, description, module, status, created_by, updated_by) VALUES
  ('AUDIT_VIEW_AUDITS',            'View audits, controls and dashboards (baseline)',            'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_VIEW_ALL_AUDITS',        'Org-wide read visibility (all scope, all tabs, framework tab)', 'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_CREATE_AUDIT',           'Create audits',                                              'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_UPDATE_AUDIT',           'Edit audits',                                                'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_MANAGE_CONTROLS',        'Create/edit controls and assign auditors',                   'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_MANAGE_FRAMEWORKS',      'Create/edit frameworks and framework controls',              'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_SUBMIT_EVIDENCE',        'Submit evidence and population',                             'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_REVIEW_EVIDENCE',        'Internal review (approve/reject) of evidence and population', 'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_VALIDATE_EVIDENCE',      'Auditor validation (sign-off) of evidence and population',   'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_SELECT_SAMPLE',          'Select the OE sample for a control',                         'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_ADD_COMMENT',            'Add control comments',                                       'AUDIT', 'ACTIVE', 'system', 'system'),
  ('AUDIT_VIEW_INTERNAL_COMMENTS', 'See internal-only control comments (hidden from auditors)',  'AUDIT', 'ACTIVE', 'system', 'system')
ON DUPLICATE KEY UPDATE
  description = VALUES(description),
  module      = VALUES(module),
  status      = 'ACTIVE',
  updated_by  = VALUES(updated_by);

-- -----------------------------------------------------------------------------
-- Grants (role -> privilege), matched by name so ids stay implicit.
-- -----------------------------------------------------------------------------
INSERT INTO role_privilege (role_id, privilege_id, is_active, created_by, updated_by)
SELECT r.id, p.id, TRUE, 'system', 'system'
FROM (
  -- Audit Compliance Admin — everything (12)
  SELECT 'grc-platform-audit-compliance-admin' AS role_name, 'AUDIT_VIEW_AUDITS'       AS privilege_name
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_VIEW_ALL_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_CREATE_AUDIT'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_UPDATE_AUDIT'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_MANAGE_CONTROLS'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_MANAGE_FRAMEWORKS'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_SUBMIT_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_REVIEW_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_VALIDATE_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_SELECT_SAMPLE'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_ADD_COMMENT'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin', 'AUDIT_VIEW_INTERNAL_COMMENTS'

  -- Audit Compliance Team — org-wide read, submit (any), internal review, comment (6)
  UNION ALL SELECT 'grc-platform-audit-compliance-team', 'AUDIT_VIEW_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-compliance-team', 'AUDIT_VIEW_ALL_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-compliance-team', 'AUDIT_SUBMIT_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-team', 'AUDIT_REVIEW_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-team', 'AUDIT_ADD_COMMENT'
  UNION ALL SELECT 'grc-platform-audit-compliance-team', 'AUDIT_VIEW_INTERNAL_COMMENTS'

  -- Audit Internal Team — submit (owned controls only) + comment (4). No VIEW_ALL -> owned scope.
  UNION ALL SELECT 'grc-platform-audit-internal-team', 'AUDIT_VIEW_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-internal-team', 'AUDIT_SUBMIT_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-internal-team', 'AUDIT_ADD_COMMENT'
  UNION ALL SELECT 'grc-platform-audit-internal-team', 'AUDIT_VIEW_INTERNAL_COMMENTS'

  -- Audit External Auditor — validate + select sample (assigned) + comment (4).
  -- No VIEW_INTERNAL_COMMENTS: internal comments are hidden from auditors.
  UNION ALL SELECT 'grc-platform-audit-external-auditor', 'AUDIT_VIEW_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-external-auditor', 'AUDIT_VALIDATE_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-external-auditor', 'AUDIT_SELECT_SAMPLE'
  UNION ALL SELECT 'grc-platform-audit-external-auditor', 'AUDIT_ADD_COMMENT'

  -- Audit Management — org-wide read-only observer (+ comment) (4). Granted
  -- GLOBAL it is the read-only observer; granted AUDIT_TEAM it is a team lead
  -- (see managedTeamIDs in internal/audit/handler/dashboard.go) — the same
  -- four privileges either way, only the grant's scope changes what they reach.
  UNION ALL SELECT 'grc-platform-audit-management', 'AUDIT_VIEW_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-management', 'AUDIT_VIEW_ALL_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-management', 'AUDIT_ADD_COMMENT'
  UNION ALL SELECT 'grc-platform-audit-management', 'AUDIT_VIEW_INTERNAL_COMMENTS'
) g
JOIN `role`      r ON r.role_name      = g.role_name COLLATE utf8mb4_bin
JOIN `privilege` p ON p.privilege_name = g.privilege_name COLLATE utf8mb4_unicode_ci
ON DUPLICATE KEY UPDATE
  is_active  = TRUE,
  updated_by = 'system';
