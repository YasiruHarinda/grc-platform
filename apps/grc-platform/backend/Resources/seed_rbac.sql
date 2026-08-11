-- =============================================================================
-- GRC Platform — Audit Hub RBAC Seed
-- Run AFTER shared.sql (needs the role, privilege, role_privilege tables).
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
-- Role names: the primary IdP uses identity mapping, so role_name must equal the
-- Asgardeo group string EXACTLY, including the environment suffix. These are the
-- STAGING names ("-stg"). A different environment (prod/local) needs its own seed
-- with the matching suffix (or none). The group string lives ONLY here (as
-- role.role_name) — application code never references it (see ADR-0002).
--
-- Scope note: privileges are coarse booleans only. Row scope (all / owned /
-- assigned) is DERIVED from these privileges at request time, not expressed here
-- — see docs/adr/0001-audit-rbac-scope-model.md and docs/adr/0002-privilege-
-- derived-scope.md. In particular AUDIT_VIEW_ALL_AUDITS is the org-wide-read
-- signal: holders get `all` scope; a holder of AUDIT_SUBMIT_EVIDENCE without it
-- is `owned`; a holder of AUDIT_VALIDATE_EVIDENCE without it is `assigned`.
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Roles (all five — including Management)
-- -----------------------------------------------------------------------------
INSERT INTO `role` (role_name, description, status, created_by, updated_by) VALUES
  ('grc-platform-audit-compliance-admin-stg', 'Audit Compliance Admin - full control of the Audit Hub', 'ACTIVE', 'system', 'system'),
  ('grc-platform-audit-compliance-team-stg',  'Audit Compliance Team - submit (any) + internal review, org-wide read', 'ACTIVE', 'system', 'system'),
  ('grc-platform-audit-internal-team-stg',    'Audit Internal Team - submit evidence for own team only', 'ACTIVE', 'system', 'system'),
  ('grc-platform-audit-external-auditor-stg', 'Audit External Auditor - validate + select sample for assigned controls', 'ACTIVE', 'system', 'system'),
  ('grc-platform-management-stg',             'Management - org-wide read-only observer', 'ACTIVE', 'system', 'system')
ON DUPLICATE KEY UPDATE
  description = VALUES(description),
  status      = 'ACTIVE',
  updated_by  = VALUES(updated_by);

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
  SELECT 'grc-platform-audit-compliance-admin-stg' AS role_name, 'AUDIT_VIEW_AUDITS'       AS privilege_name
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_VIEW_ALL_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_CREATE_AUDIT'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_UPDATE_AUDIT'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_MANAGE_CONTROLS'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_MANAGE_FRAMEWORKS'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_SUBMIT_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_REVIEW_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_VALIDATE_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_SELECT_SAMPLE'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_ADD_COMMENT'
  UNION ALL SELECT 'grc-platform-audit-compliance-admin-stg', 'AUDIT_VIEW_INTERNAL_COMMENTS'

  -- Audit Compliance Team — org-wide read, submit (any), internal review, comment (6)
  UNION ALL SELECT 'grc-platform-audit-compliance-team-stg', 'AUDIT_VIEW_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-compliance-team-stg', 'AUDIT_VIEW_ALL_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-compliance-team-stg', 'AUDIT_SUBMIT_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-team-stg', 'AUDIT_REVIEW_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-compliance-team-stg', 'AUDIT_ADD_COMMENT'
  UNION ALL SELECT 'grc-platform-audit-compliance-team-stg', 'AUDIT_VIEW_INTERNAL_COMMENTS'

  -- Audit Internal Team — submit (owned controls only) + comment (4). No VIEW_ALL -> owned scope.
  UNION ALL SELECT 'grc-platform-audit-internal-team-stg', 'AUDIT_VIEW_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-internal-team-stg', 'AUDIT_SUBMIT_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-internal-team-stg', 'AUDIT_ADD_COMMENT'
  UNION ALL SELECT 'grc-platform-audit-internal-team-stg', 'AUDIT_VIEW_INTERNAL_COMMENTS'

  -- Audit External Auditor — validate + select sample (assigned) + comment (4).
  -- No VIEW_INTERNAL_COMMENTS: internal comments are hidden from auditors.
  UNION ALL SELECT 'grc-platform-audit-external-auditor-stg', 'AUDIT_VIEW_AUDITS'
  UNION ALL SELECT 'grc-platform-audit-external-auditor-stg', 'AUDIT_VALIDATE_EVIDENCE'
  UNION ALL SELECT 'grc-platform-audit-external-auditor-stg', 'AUDIT_SELECT_SAMPLE'
  UNION ALL SELECT 'grc-platform-audit-external-auditor-stg', 'AUDIT_ADD_COMMENT'

  -- Management — org-wide read-only observer (+ comment) (4)
  UNION ALL SELECT 'grc-platform-management-stg', 'AUDIT_VIEW_AUDITS'
  UNION ALL SELECT 'grc-platform-management-stg', 'AUDIT_VIEW_ALL_AUDITS'
  UNION ALL SELECT 'grc-platform-management-stg', 'AUDIT_ADD_COMMENT'
  UNION ALL SELECT 'grc-platform-management-stg', 'AUDIT_VIEW_INTERNAL_COMMENTS'
) g
JOIN `role`      r ON r.role_name      = g.role_name COLLATE utf8mb4_bin
JOIN `privilege` p ON p.privilege_name = g.privilege_name COLLATE utf8mb4_unicode_ci
ON DUPLICATE KEY UPDATE
  is_active  = TRUE,
  updated_by = 'system';
