# Compliance Evidence Portal — Web App

React + TypeScript single-page app (Vite, MUI v5). Deployed on Choreo as a
web-application component on the **React buildpack**: Choreo builds `dist/`
and serves it as static files behind a CDN, with no server process of ours in
front of it. `index.js` and the `Dockerfile` are retained as a fallback for
switching back to the Dockerfile buildpack — see Build & deploy below.

## Stack

- **React 19 + TypeScript**, built with **Vite**
- **MUI v5** for UI, **@oxygen-ui/react-icons** for icons
- **@tanstack/react-query** for data fetching, **axios** for the API client
- **@asgardeo/auth-react** for OAuth2 sign-in

## Local development

```bash
npm install --legacy-peer-deps
cp public/config.js.example public/config.js   # fill in your values
npm run dev                                     # http://localhost:5173
```

Configuration is **not** read from `.env` files. The browser loads a runtime
`public/config.js` (referenced in `index.html`, gitignored) which sets
`window.config` before the React bundle starts. Values are read once at page
load — restart the dev server (or hard-refresh) after editing `config.js`.

With `EVIDENCE_PORTAL_BACKEND_BASE_URL` left empty, the Vite dev server
proxies `/api` to `http://localhost:8000` (see `vite.config.ts`), so no CORS
setup is needed locally. Sign-in always goes through Asgardeo, locally and in
production.

To call a backend directly instead (skipping the proxy), set
`EVIDENCE_PORTAL_BACKEND_BASE_URL` to e.g. `http://localhost:8000` in
`config.js` — and make sure the backend's `CORS_ORIGINS` includes
`http://localhost:5173`.

## Build & deploy

```bash
npm run build     # tsc + vite build → dist/
```

The web app is deployed on Choreo as a **React buildpack** component: Choreo
builds `dist/` and serves it as static files behind a CDN. No server process
of ours runs afterwards, so `config.js` must be placed alongside the built
app by whatever supplies runtime config on Choreo — the app reads it the
same way it does locally.

`index.js` and the `Dockerfile` are kept but **not used** on this buildpack;
see the header comment in each. They exist so the component can be switched
back to a Dockerfile buildpack without a code change, in which case
`config.js` is still the source of truth — `index.js` no longer generates it
from environment variables.

## Configuration keys

Set these in `public/config.js` — see
[`public/config.js.example`](public/config.js.example) for the template:

| Key | Required | Description |
| --- | --- | --- |
| `EVIDENCE_PORTAL_AUTH_CLIENT_ID` | Yes | Asgardeo OAuth application client ID. |
| `EVIDENCE_PORTAL_AUTH_BASE_URL` | Yes | Asgardeo tenant base URL, e.g. `https://api.asgardeo.io/t/<org>`. |
| `EVIDENCE_PORTAL_BACKEND_BASE_URL` | No | Backend base URL, no trailing slash and no `/api` suffix. Empty keeps API calls relative to `/api` (the Vite dev proxy or `index.js`, if either is in front); set it to call the backend directly, e.g. on the React buildpack. |

Missing required keys are logged to the browser console, not thrown — the
app still starts and reports what's missing instead of showing a blank
white page.
