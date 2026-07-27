# Compliance Evidence Portal — Web App

React + TypeScript single-page app (Vite, MUI v5). Deployed on Choreo as a
web-application component that builds `dist/` and serves it via `index.js`.

## Stack

- **React 19 + TypeScript**, built with **Vite**
- **MUI v5** for UI, **@oxygen-ui/react-icons** for icons
- **@tanstack/react-query** for data fetching, **axios** for the API client
- **@asgardeo/auth-react** for OAuth2 sign-in

## Local development

```bash
npm install --legacy-peer-deps
npm run dev                    # http://localhost:5173
```

The Vite dev server proxies `/api` to `http://localhost:8000`
(see `vite.config.ts`), so no CORS setup is needed locally. Sign-in always goes
through Asgardeo, locally and in production.

## Build & serve (production / Choreo)

```bash
npm run build     # tsc + vite build → dist/
npm start         # node index.js — serves dist/ and proxies to the backend
```

`index.js` reads **`BACKEND_URL`** from the environment and proxies `/api`
to it (falling back to `http://localhost:8000` for local use). On
Choreo, set `BACKEND_URL` to the backend component's public gateway URL.

## Environment variables

See [`.env.example`](.env.example). All are optional:

| Variable | Purpose |
| --- | --- |
| `VITE_ASGARDEO_CLIENT_ID` | Override the Asgardeo app client ID (default baked into `src/main.tsx`). |
| `VITE_ASGARDEO_BASE_URL` | Override the Asgardeo tenant base URL. |
| `BACKEND_URL` | Runtime (Node server): backend base URL to proxy to. |
