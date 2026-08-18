// Shape of the runtime configuration the browser loads from `/config.js`
// (see `public/config.js.example`) before the React bundle starts. That
// script sets `window.config`; everything under `src/config/` only reads
// it. Keys are deliberately not `VITE_`-prefixed — that prefix means
// "substituted at build time" to Vite, and these are read at page load.
export interface EvidencePortalWindowConfig {
  EVIDENCE_PORTAL_AUTH_BASE_URL?: string;
  EVIDENCE_PORTAL_AUTH_CLIENT_ID?: string;
  EVIDENCE_PORTAL_BACKEND_BASE_URL?: string;
}

declare global {
  interface Window {
    config?: EvidencePortalWindowConfig;
  }
}

export {};
