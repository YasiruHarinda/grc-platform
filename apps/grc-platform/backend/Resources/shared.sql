-- =============================================================================
-- GRC Platform — Shared Schema
-- Run this FIRST, before audit_schema.sql and risk_schema.sql.
--
-- Full order:  shared.sql → risk_schema.sql / audit_schema.sql
--              → shared_seed_data.sql   (required, not optional — see below)
-- =============================================================================
--
-- Tables:
--   user            — platform identity, shared by both modules
--   role            — platform-owned role names (no longer mirrors an IdP)
--   privilege       — fine-grained privileges used for frontend view rendering
--   role_privilege  — maps roles to privileges (many-to-many)
--   user_role_grant — WHO holds WHAT role, WHERE. The single record of a user's
--                     standing, replacing both per-module team-membership tables
--                     and the Asgardeo group claim. See the table comment below.
--
-- NOTE: role assignment is owned by this platform, not by the IdP. Asgardeo
--       authenticates users and nothing more — no group or role claim is read
--       from its tokens. Team membership is likewise not a column on `user`:
--       it is carried by user_role_grant.scope_id, so a user's role and the
--       scope it applies in are recorded together rather than in two tables
--       that have to agree with each other.
--   user            — platform identity, shared by both modules
--   role            — platform-owned role names (no longer mirrors an IdP)
--   privilege       — fine-grained privileges used for frontend view rendering
--   role_privilege  — maps roles to privileges (many-to-many)
--   user_role_grant — WHO holds WHAT role, WHERE. The single record of a user's
--                     standing, replacing both per-module team-membership tables
--                     and the Asgardeo group claim. See the table comment below.
--
-- NOTE: role assignment is owned by this platform, not by the IdP. Asgardeo
--       authenticates users and nothing more — no group or role claim is read
--       from its tokens. Team membership is likewise not a column on `user`:
--       it is carried by user_role_grant.scope_id, so a user's role and the
--       scope it applies in are recorded together rather than in two tables
--       that have to agree with each other.
--
-- Assumes the `grc_platform` database already exists (e.g. provisioned ahead
-- of time on a hosted/managed MySQL instance) and that the connecting user
-- only needs privileges scoped to it — no CREATE DATABASE here. Every table
-- below sets its own CHARSET/COLLATE explicitly, so the database's own
-- default charset doesn't matter.
--
-- Run order (each file is standalone — it selects the database itself, so no
-- -D/database argument is required):
--   mysql -u <user> -p < shared.sql
--   mysql -u <user> -p < audit_schema.sql
--   mysql -u <user> -p < risk_schema.sql
--
-- Seed data lives outside this directory and is applied afterwards.
-- =============================================================================

USE grc_platform;

SET FOREIGN_KEY_CHECKS = 0;

-- -----------------------------------------------------------------------------
-- user
-- Platform users, authenticated via Asgardeo SSO.
-- Neither roles nor team membership are columns here — both are rows in
-- user_role_grant below.
--
-- A user row may exist with NO grants at all. That is a legitimate, expected
-- state, not an incomplete one: an Action Owner may be any employee, resolved
-- and upserted on the fly when named on an action plan. Such a user reaches
-- exactly the risks they are personally named on, and nothing else.
-- Platform users, authenticated via Asgardeo SSO.
-- Neither roles nor team membership are columns here — both are rows in
-- user_role_grant below.
--
-- A user row may exist with NO grants at all. That is a legitimate, expected
-- state, not an incomplete one: an Action Owner may be any employee, resolved
-- and upserted on the fly when named on an action plan. Such a user reaches
-- exactly the risks they are personally named on, and nothing else.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `user` (
  id            INT          NOT NULL AUTO_INCREMENT,
  uuid          CHAR(36)     NOT NULL,
  user_type     ENUM('INTERNAL','EXTERNAL') NOT NULL DEFAULT 'INTERNAL',
  status        ENUM('ACTIVE','INACTIVE','REMOVED') NOT NULL DEFAULT 'ACTIVE',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by    VARCHAR(255) NULL,
  updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by    VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_user_uuid (uuid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- uuid is the Asgardeo `sub` claim, and this table's only identity: a
-- security review required that the platform stop storing user emails and
-- display names, and the Audit Hub module (the last consumer still reading
-- email/display_name directly) has since been converted to resolve both
-- through the identity directory instead. No real user rows existed at the
-- time of that conversion, so no backfill was needed and the column drop
-- below lands in the same push, not staged.
--
-- Guarded the same way as role.module/scope_basis below, so re-running this
-- file against an existing database is a no-op past the first run.
SET @user_has_uuid = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user' AND COLUMN_NAME = 'uuid'
);
SET @add_uuid_sql = IF(@user_has_uuid = 0,
  'ALTER TABLE `user` ADD COLUMN uuid CHAR(36) NULL AFTER id, ADD UNIQUE KEY uq_user_uuid (uuid)',
  'SELECT 1');
PREPARE add_uuid_stmt FROM @add_uuid_sql;
EXECUTE add_uuid_stmt;
DEALLOCATE PREPARE add_uuid_stmt;

-- email becomes NULLable here first: the Admin Console's "Add User" (see
-- ADMIN_CONSOLE_DESIGN.md) provisions a platform user by uuid alone — nothing
-- else about them is known or wanted, per the same security review noted
-- above — and uq_user_email cannot take a second "" once the first uuid-only
-- row has one. display_name stays NOT NULL: it already receives "" rather
-- than a real value from every caller migrating off it (see
-- internal/user/handler/resolve.go's comment), which satisfies NOT NULL with
-- no schema change, so there is nothing to relax there.
--
-- Guarded on email existing AND still NOT NULL — a fresh database has no
-- email column at all, and running MODIFY COLUMN against a column that
-- doesn't exist errors out rather than no-opping.
SET @user_email_not_nullable = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user' AND COLUMN_NAME = 'email' AND IS_NULLABLE = 'NO'
);
SET @make_email_nullable_sql = IF(@user_email_not_nullable > 0,
  'ALTER TABLE `user` MODIFY COLUMN email VARCHAR(255) NULL',
  'SELECT 1');
PREPARE make_email_nullable_stmt FROM @make_email_nullable_sql;
EXECUTE make_email_nullable_stmt;
DEALLOCATE PREPARE make_email_nullable_stmt;

-- Then drops email/display_name and tightens uuid to NOT NULL, taking a
-- database in either the pre-nullable shape above or the just-staged
-- nullable-email shape the rest of the way to the final one this file's
-- CREATE TABLE now produces on a fresh database. Guarded on email's
-- existence, so it's a no-op once already applied. `uq_user_email` is
-- dropped first: MySQL refuses to drop a column a unique key still
-- references.
SET @user_has_email = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user' AND COLUMN_NAME = 'email'
);
SET @drop_email_sql = IF(@user_has_email > 0,
  'ALTER TABLE `user` DROP INDEX uq_user_email, DROP COLUMN email, DROP COLUMN display_name, MODIFY COLUMN uuid CHAR(36) NOT NULL',
  'SELECT 1');
PREPARE drop_email_stmt FROM @drop_email_sql;
EXECUTE drop_email_stmt;
DEALLOCATE PREPARE drop_email_stmt;


-- -----------------------------------------------------------------------------
-- role
-- Platform-owned role names. These are ours: they no longer have to match any
-- external identity provider's group string. The grc-platform-* prefix is
-- historical and kept deliberately — renaming during the migration off
-- Asgardeo would have changed two things at once.
--
-- module determines which scopes a role may be granted against, enforced by the
-- application on write to user_role_grant:
--   RISK   → GLOBAL or RISK_TEAM
--   AUDIT  → GLOBAL or AUDIT_TEAM
--   SHARED → GLOBAL only
-- The role decides the module; the scope decides the breadth. Keeping module on
-- the role (rather than deriving it from the union of its privileges) is what
-- makes that rule checkable — a role's privileges can span both hubs, so they
-- cannot answer "which module is this role for".
--
-- scope_basis says WHICH DIMENSION of a risk a grant on this role scopes by.
-- A risk carries two team references, and different roles are about different
-- ones:
--   SOURCE_REGISTER → matches risk.source_register_id. The register a risk was
--                     raised in. Risk Assigner, Compliance, Management.
--   ASSIGNMENT_TEAM → matches risk.assignment_team_id. The team the work was
--                     routed to. Risk Owner.
--   NULL            → the role is GLOBAL-only and scopes nothing (SHARED roles,
--                     and every audit role — audit_control has exactly one
--                     team column, so this ambiguity does not exist there).
--
-- This has to be recorded rather than inferred, because a risk_team row can be
-- BOTH a register and an assignment team — 7 of the 9 seeded teams are — so the
-- grant alone cannot say which sense was meant. "Risk Owner @ Asgardeo" means
-- risks ASSIGNED to Asgardeo; "Risk Assigner @ Asgardeo" means risks RAISED
-- there. Same row shape, different dimension.
--
-- Deriving this from the role's privileges instead would recreate the
-- hand-maintained allowlists this design exists to delete, and checking the
-- role NAME would break the module's rule that code never does that.
--
-- Use status = 'INACTIVE' to soft-delete; hard-delete is blocked by FK.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `role` (
  id           INT          NOT NULL AUTO_INCREMENT,
  role_name    VARCHAR(150) COLLATE utf8mb4_bin NOT NULL COMMENT 'Binary collation keeps role-name matching case-sensitive and consistent with Go map lookup',
  description  TEXT         NULL,
  module       ENUM('RISK','AUDIT','SHARED') NOT NULL,
  scope_basis  ENUM('SOURCE_REGISTER','ASSIGNMENT_TEAM') NULL COMMENT 'Which risk column a grant on this role scopes by; NULL for GLOBAL-only roles. See table comment',
  assignable_user_type ENUM('INTERNAL','EXTERNAL') NOT NULL DEFAULT 'INTERNAL' COMMENT 'Which kind of person this role may be granted to. INTERNAL/EXTERNAL identities live in separate Asgardeo organisations, so a role never spans both — no EITHER value',
  status       ENUM('ACTIVE','INACTIVE') NOT NULL DEFAULT 'ACTIVE',
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by   VARCHAR(255) NULL,
  updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by   VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_role_name (role_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Evolution guard for `role` — module and scope_basis are new columns on a
-- table that pre-dates them. On a FRESH database the CREATE TABLE above already
-- creates both, so this is a no-op there. On an EXISTING database (staging,
-- production), CREATE TABLE IF NOT EXISTS silently does nothing when `role`
-- already exists, and without this block the two columns would never appear —
-- not as NULL, but genuinely MISSING, which fails every query the moment the
-- new backend queries them (GetRoleByID, ListRoles, grantSelect).
--
-- Deliberately not `ADD COLUMN IF NOT EXISTS`: that syntax needs MySQL
-- 8.0.29+, and nothing in this repo pins a MySQL patch version. The
-- information_schema guard below is portable to any MySQL 8.
--
-- module gets a DEFAULT so existing rows backfill immediately rather than
-- failing the NOT NULL constraint; shared_seed_data.sql's role INSERT (ON
-- DUPLICATE KEY UPDATE module = VALUES(module)) corrects it to the real value
-- for every seeded role right after this runs. scope_basis is nullable by
-- design (NULL means GLOBAL-only) and needs no default for the same reason.
SET @role_has_module = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'role' AND COLUMN_NAME = 'module'
);
SET @add_module_sql = IF(@role_has_module = 0,
  'ALTER TABLE `role` ADD COLUMN module ENUM(''RISK'',''AUDIT'',''SHARED'') NOT NULL DEFAULT ''RISK'' AFTER description',
  'SELECT 1');
PREPARE add_module_stmt FROM @add_module_sql;
EXECUTE add_module_stmt;
DEALLOCATE PREPARE add_module_stmt;

SET @role_has_scope_basis = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'role' AND COLUMN_NAME = 'scope_basis'
);
SET @add_scope_basis_sql = IF(@role_has_scope_basis = 0,
  'ALTER TABLE `role` ADD COLUMN scope_basis ENUM(''SOURCE_REGISTER'',''ASSIGNMENT_TEAM'') NULL '
  'COMMENT ''Which risk column a grant on this role scopes by; NULL for GLOBAL-only roles. See table comment'' '
  'AFTER module',
  'SELECT 1');
PREPARE add_scope_basis_stmt FROM @add_scope_basis_sql;
EXECUTE add_scope_basis_stmt;
DEALLOCATE PREPARE add_scope_basis_stmt;

-- assignable_user_type gets a DEFAULT ('INTERNAL') so existing rows backfill
-- immediately — every pre-existing role is in fact INTERNAL-only today, so
-- the default is also the correct final value for all of them except
-- grc-platform-audit-external-auditor, which shared_seed_data.sql's role
-- INSERT (ON DUPLICATE KEY UPDATE assignable_user_type = VALUES(...))
-- corrects to EXTERNAL right after this runs.
SET @role_has_assignable_user_type = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'role' AND COLUMN_NAME = 'assignable_user_type'
);
SET @add_assignable_user_type_sql = IF(@role_has_assignable_user_type = 0,
  'ALTER TABLE `role` ADD COLUMN assignable_user_type ENUM(''INTERNAL'',''EXTERNAL'') NOT NULL DEFAULT ''INTERNAL'' '
  'COMMENT ''Which kind of person this role may be granted to. See table comment'' '
  'AFTER scope_basis',
  'SELECT 1');
PREPARE add_assignable_user_type_stmt FROM @add_assignable_user_type_sql;
EXECUTE add_assignable_user_type_stmt;
DEALLOCATE PREPARE add_assignable_user_type_stmt;


-- -----------------------------------------------------------------------------
-- privilege
-- Fine-grained privileges used for frontend view rendering.
-- module scopes each privilege to RISK, AUDIT, or SHARED.
-- privilege_name is the key the frontend checks to conditionally render UI.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `privilege` (
  id              INT          NOT NULL AUTO_INCREMENT,
  privilege_name  VARCHAR(150) NOT NULL,
  description     TEXT         NULL,
  module          ENUM('RISK','AUDIT','SHARED') NOT NULL,
  status          ENUM('ACTIVE','INACTIVE') NOT NULL DEFAULT 'ACTIVE',
  created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by      VARCHAR(255) NULL,
  updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by      VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_privilege_name (privilege_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- role_privilege
-- Many-to-many junction between role and privilege.
-- Composite PK (role_id, privilege_id) enforces uniqueness.
-- is_active allows toggling a mapping without deleting it.
-- FKs are RESTRICT — use status to soft-delete roles/privileges.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS role_privilege (
  role_id      INT          NOT NULL,
  privilege_id INT          NOT NULL,
  is_active    BOOLEAN      NOT NULL DEFAULT TRUE,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by   VARCHAR(255) NULL,
  updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by   VARCHAR(255) NULL,
  PRIMARY KEY (role_id, privilege_id),
  CONSTRAINT fk_rp_role      FOREIGN KEY (role_id)      REFERENCES `role`(id)      ON DELETE RESTRICT,
  CONSTRAINT fk_rp_privilege FOREIGN KEY (privilege_id) REFERENCES `privilege`(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- user_role_grant
-- The single record of who holds what role, where — a triple of
-- (user, role, scope). Replaces per-module team-membership tables, which
-- recorded membership with no role and so could not express "Risk Owner in one
-- register, Risk Assigner in another".
--
-- SCOPE — IT TAKES BOTH COLUMNS TOGETHER
-- scope_type and scope_id are meaningless apart. scope_type says WHICH TABLE
-- scope_id points into; scope_id says which row of it. Reading scope_id alone
-- tells you nothing, because there is no foreign key to disambiguate it:
--
--   scope_type = 'GLOBAL'     → scope_id is 0 and unused. Every scope the
--                               role's module defines, present and future.
--   scope_type = 'RISK_TEAM'  → scope_id is a risk_team.id  (a register or
--                               assignment team)
--   scope_type = 'AUDIT_TEAM' → scope_id is an audit_team.id
--
-- So the same scope_id = 1 means "Asgardeo" under RISK_TEAM and "audit team 1"
-- under AUDIT_TEAM. The pair is the answer; neither column is.
--
-- Granting a role on one register therefore looks like this — and resolve the
-- register by CODE rather than hardcoding an id, which differs per environment:
--
--   INSERT INTO user_role_grant (user_id, role_id, scope_type, scope_id, created_by)
--   SELECT u.id, r.id, 'RISK_TEAM', t.id, '<actor-uuid>'
--   FROM   `user` u, `role` r, risk_team t
--   WHERE  u.uuid = '<someone's-asgardeo-uuid>'
--     AND  r.role_name = 'grc-platform-risk-assigner'
--     AND  t.code = 'ASG';          -- ← t.id lands in scope_id: THIS is the register
--
-- For a GLOBAL grant, pass 'GLOBAL' and 0 and name no team at all.
--
-- GLOBAL is a WILDCARD, never an expansion into one row per team. A team
-- created months from now is covered by an existing GLOBAL grant immediately,
-- with no backfill and nothing to keep in sync. Expanding instead would leave
-- new teams silently uncovered — do not "helpfully" materialise these rows.
--
-- WHY scope_id IS NOT NULL WITH A 0 SENTINEL
-- MySQL treats NULLs as distinct in a unique index, so a nullable scope_id
-- would allow two identical GLOBAL grants for the same (user, role) — and
-- revoking one would silently leave the other in force. The sentinel makes
-- uq_grant actually hold; chk_grant_scope stops the two columns disagreeing.
--
-- WHY scope_id HAS NO FOREIGN KEY
-- It references risk_team or audit_team depending on scope_type, so no single
-- FK can express it. Integrity rests on two things: application-level
-- validation on write, and the rule that TEAM TABLES ARE NEVER HARD-DELETED
-- (both soft-delete via status). A grant pointing at a missing team grants
-- nothing, so the failure mode is closed, not open.
--
-- ACCESS RULE
-- Read is broad, authority is narrow: a grant on a risk's ASSIGNMENT team
-- confers visibility and picker-eligibility only, never write or approval
-- rights. Those follow the SOURCE register. Otherwise assigning a risk to a
-- team — an ordinary business field any assigner can set — would hand that
-- team's role-holders authority over it. On the audit side the analogous rule
-- is enforced in code, not schema.
--
-- Grants are NOT the only source of access. Being personally named on a risk
-- (owner_id, assigner_id, management_approver_id, an action plan's
-- action_owner_id) or an audit control (owner_id, auditor_id) confers a narrow
-- per-record capability with no grant at all.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_role_grant (
  id         INT      NOT NULL AUTO_INCREMENT,
  user_id    INT      NOT NULL,
  role_id    INT      NOT NULL,
  scope_type ENUM('GLOBAL','RISK_TEAM','AUDIT_TEAM') NOT NULL COMMENT 'Which table scope_id points into. Read it together with scope_id — neither means anything alone',
  scope_id   INT      NOT NULL DEFAULT 0 COMMENT 'The scope itself: risk_team.id when scope_type=RISK_TEAM (this is the register), audit_team.id when AUDIT_TEAM, 0/unused when GLOBAL. No FK — see table comment',
  status     ENUM('ACTIVE','INACTIVE') NOT NULL DEFAULT 'ACTIVE',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by VARCHAR(255) NULL COMMENT 'Who granted this — the audit trail for a privilege-granting action',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_grant (user_id, role_id, scope_type, scope_id),
  KEY idx_grant_user (user_id),
  KEY idx_grant_scope (scope_type, scope_id),
  CONSTRAINT fk_grant_user FOREIGN KEY (user_id) REFERENCES `user`(id) ON DELETE CASCADE,
  CONSTRAINT fk_grant_role FOREIGN KEY (role_id) REFERENCES `role`(id) ON DELETE RESTRICT,
  CONSTRAINT chk_grant_scope CHECK (
    (scope_type =  'GLOBAL' AND scope_id =  0) OR
    (scope_type <> 'GLOBAL' AND scope_id >  0)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS = 1;
