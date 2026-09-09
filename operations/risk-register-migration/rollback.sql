-- =============================================================================
-- risk-register-import — ROLLBACK
--
-- Deletes everything the importer wrote, identified by the marker
--   created_by = 'risk-sheet-migration'
-- Run by hand, against the same database the import targeted, by someone with
-- DB access (they are inside the network already). The importer itself never
-- deletes.
--
-- Order matters: children before parents (FKs are RESTRICT on the history
-- tables). `user` rows are intentionally NOT deleted — other data may reference
-- them and re-provisioning them is a no-op.
--
-- Review the counts from the first block before running the DELETEs.
-- =============================================================================

USE grc_platform;

-- ── What would be removed ────────────────────────────────────────────────────
SELECT 'risk'                    AS table_name, COUNT(*) AS rows_to_delete FROM risk                    WHERE created_by = 'risk-sheet-migration'
UNION ALL SELECT 'risk_escalation',        COUNT(*) FROM risk_escalation        WHERE created_by = 'risk-sheet-migration'
UNION ALL SELECT 'risk_change_log',        COUNT(*) FROM risk_change_log        WHERE created_by = 'risk-sheet-migration'
UNION ALL SELECT 'risk_action_step',       COUNT(*) FROM risk_action_step step
          JOIN risk_action_plan plan ON plan.id = step.plan_id
          WHERE plan.created_by = 'risk-sheet-migration'
UNION ALL SELECT 'risk_action_plan',       COUNT(*) FROM risk_action_plan       WHERE created_by = 'risk-sheet-migration'
UNION ALL SELECT 'user_role_grant',        COUNT(*) FROM user_role_grant        WHERE created_by = 'risk-sheet-migration';

-- ── DELETE (uncomment to run) ───────────────────────────────────────────────
-- START TRANSACTION;
--
-- DELETE FROM risk_escalation  WHERE created_by = 'risk-sheet-migration';
-- DELETE FROM risk_change_log  WHERE created_by = 'risk-sheet-migration';
--
-- DELETE step FROM risk_action_step step
--   JOIN risk_action_plan plan ON plan.id = step.plan_id
--   WHERE plan.created_by = 'risk-sheet-migration';
--
-- -- risk_category_reference / risk_compliance_reference carry no created_by;
-- -- they cascade when their risk row goes.
-- DELETE FROM risk_action_plan WHERE created_by = 'risk-sheet-migration';
-- DELETE FROM risk            WHERE created_by = 'risk-sheet-migration';
--
-- DELETE FROM user_role_grant WHERE created_by = 'risk-sheet-migration';
--
-- COMMIT;

-- ── Verify (expect zero) ───────────────────────────────────────────────────
-- SELECT COUNT(*) FROM risk WHERE created_by = 'risk-sheet-migration';
