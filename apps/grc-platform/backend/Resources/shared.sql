-- =============================================================================
-- GRC Platform — Shared Schema
-- Run this FIRST, before audit_schema.sql and risk_schema.sql.
-- =============================================================================
--
-- Tables:
--   user           — platform identity, shared by both modules
--   role           — mirrors Asgardeo JWT role claim strings exactly
--   privilege      — fine-grained privileges used for frontend view rendering
--   role_privilege — maps roles to privileges (many-to-many)
--
-- NOTE: user ↔ audit_team membership lives in the user_audit_team junction
--       table (many-to-many), created by audit_schema.sql after audit_team
--       exists — see that file for the table.
--       user.risk_team_id  FK → added by risk_schema.sql  (after risk_team  exists)
-- =============================================================================

SET FOREIGN_KEY_CHECKS = 0;

-- -----------------------------------------------------------------------------
-- user
-- Platform users provisioned via Asgardeo SSO.
-- Role assignment lives in Asgardeo JWT claims, not here.
-- Audit team membership is many-to-many (see user_audit_team in
-- audit_schema.sql), not a column here. risk_team_id remains a single-value
-- column; its FK constraint is wired in by risk_schema.sql.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `user` (
  id            INT          NOT NULL AUTO_INCREMENT,
  email         VARCHAR(255) NOT NULL,
  display_name  VARCHAR(255) NOT NULL,
  user_type     ENUM('INTERNAL','EXTERNAL') NOT NULL DEFAULT 'INTERNAL',
  risk_team_id  INT          NULL,
  status        ENUM('ACTIVE','INACTIVE','REMOVED') NOT NULL DEFAULT 'ACTIVE',
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by    VARCHAR(255) NULL,
  updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by    VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_user_email (email),
  KEY idx_user_risk_team  (risk_team_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- -----------------------------------------------------------------------------
-- role
-- Must match Asgardeo JWT role claim strings exactly.
-- Use status = 'INACTIVE' to soft-delete; hard-delete is blocked by FK.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `role` (
  id           INT          NOT NULL AUTO_INCREMENT,
  role_name    VARCHAR(150) COLLATE utf8mb4_bin NOT NULL COMMENT 'Must match the Asgardeo JWT role claim string exactly; binary collation enforces case-sensitivity consistent with Go map lookup',
  description  TEXT         NULL,
  status       ENUM('ACTIVE','INACTIVE') NOT NULL DEFAULT 'ACTIVE',
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by   VARCHAR(255) NULL,
  updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by   VARCHAR(255) NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_role_name (role_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


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

SET FOREIGN_KEY_CHECKS = 1;
