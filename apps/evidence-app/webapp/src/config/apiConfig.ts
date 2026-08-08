import "./portalConfig";

// Base URL for the backend API, read from `window.config` (see
// portalConfig.ts). Empty when unset, which keeps requests relative — see
// the dual-mode `baseURL` in api/client.ts, which turns this into "/api"
// rather than an absolute address in that case.
export const BACKEND_BASE_URL: string =
  window.config?.EVIDENCE_PORTAL_BACKEND_BASE_URL ?? "";
