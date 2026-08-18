import "./portalConfig";

// Base URL for the backend API, read from `window.config` (see
// portalConfig.ts). Empty when unset, which keeps requests relative — see
// the dual-mode `baseURL` in api/client.ts, which turns this into "/api"
// rather than an absolute address in that case.
//
// A trailing slash is stripped here rather than at the call sites, because
// every caller appends a path beginning with "/" and two of them do so
// independently (api/client.ts and the SSE fetch in pages/AgentRunner.tsx).
// Left in, "https://host/" would produce "https://host//api/...", which is a
// different path and 404s. The value is hand-written into config.js by
// whoever deploys, and a URL copied out of a console commonly carries the
// slash — config.js.example says not to include one, but that is a note and
// not a guard. index.js normalises the same way for the same reason.
export const BACKEND_BASE_URL: string = (
  window.config?.EVIDENCE_PORTAL_BACKEND_BASE_URL ?? ""
).replace(/\/$/, "");
