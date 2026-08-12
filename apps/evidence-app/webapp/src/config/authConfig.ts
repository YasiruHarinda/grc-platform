import "./portalConfig";

// Asgardeo settings read from `window.config` (see portalConfig.ts). Empty
// strings when `config.js` hasn't set them — main.tsx decides what to do
// about that (log, not throw; see decision 2 in issue #90).
export interface AuthConfig {
  clientId: string;
  baseUrl: string;
}

const getAuthConfig = (): AuthConfig => ({
  clientId: window.config?.EVIDENCE_PORTAL_AUTH_CLIENT_ID || "",
  baseUrl: window.config?.EVIDENCE_PORTAL_AUTH_BASE_URL || "",
});

export const authConfig = getAuthConfig();
