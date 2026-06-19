# GRC Platform Webapp

The GRC Platform Webapp is the React single-page application for WSO2 TrustHub — a
governance, risk, and compliance platform. It hosts two modules behind a shared
shell:

- **Audit Hub** — run SOC 2 / HIPAA / ISO 27001 audits: controls, evidence
  collection, population/sample workflows, and review.
- **Risk Hub** — risk register and assessment workflows.

The app authenticates users through **Asgardeo** (or WSO2 Identity Server) and
talks to the GRC Platform **Go backend** over REST. Access is role-gated.

## Tech Stack

| Area | Technology |
|------|------------|
| UI | [React 19](https://react.dev/), [TypeScript](https://www.typescriptlang.org/), [Vite 7](https://vite.dev/) |
| Design system | [Oxygen UI](https://github.com/wso2/oxygen-ui) (`@wso2/oxygen-ui` 0.6.0) |
| Data fetching | [TanStack Query 5](https://tanstack.com/query/latest) |
| Routing | [React Router 7](https://reactrouter.com/) |
| Authentication | [@asgardeo/react](https://github.com/asgardeo/asgardeo-auth-react-sdk) |

## Prerequisites

- [Node.js](https://nodejs.org/) 20+ (LTS recommended)
- npm 10+
- (Optional) A running GRC Platform Go backend for real data
- (Optional) An Asgardeo organisation with a registered SPA application — only
  needed once you switch off mock auth

## Getting Started

### 1. Install dependencies

```bash
npm install --legacy-peer-deps
```

> `--legacy-peer-deps` is required because `@wso2/oxygen-ui` pins an exact React
> peer version.

### 2. Configure the app

Configuration is **not** read from `.env` files. The browser loads a runtime
`public/config.js` (referenced in `index.html`) which sets `window.config`
before the React bundle starts.

A ready-to-run `public/config.js` is already present with **mock auth enabled**,
so you can start immediately without Asgardeo or a backend:

```javascript
window.config = {
  // Bypass Asgardeo for local development. Set to false to use real auth.
  GRC_PLATFORM_MOCK_AUTH: true,

  GRC_PLATFORM_AUTH_BASE_URL: "https://api.asgardeo.io/t/<your-org>",
  GRC_PLATFORM_AUTH_CLIENT_ID: "<your-client-id>",
  GRC_PLATFORM_AUTH_SIGN_IN_REDIRECT_URL: "http://localhost:3000/",
  GRC_PLATFORM_AUTH_SIGN_OUT_REDIRECT_URL: "http://localhost:3000/",

  GRC_PLATFORM_BACKEND_BASE_URL: "http://localhost:8080",

  GRC_PLATFORM_THEME: "acrylicOrange",
  GRC_PLATFORM_LOG_LEVEL: "DEBUG",
};
```

Values are read **once at page load** — restart the dev server (or hard-refresh)
after editing `config.js`.

### 3. Run the webapp

```bash
npm run dev
```

Open [http://localhost:3000](http://localhost:3000). The root path redirects to
`/audit/dashboard`.

## Available Scripts

| Command | Description |
|---------|-------------|
| `npm run dev` | Start the Vite dev server on port 3000 |
| `npm run build` | Type-check (`tsc -b`) and build to `dist/` |
| `npm run preview` | Serve the production build locally |
| `npm run lint` | Run ESLint |

## Configuration Keys

| Key | Required | Description |
|-----|----------|-------------|
| `GRC_PLATFORM_MOCK_AUTH` | No | `true` bypasses Asgardeo and signs in a local "Dev User". Set `false`/omit for real auth. |
| `GRC_PLATFORM_AUTH_BASE_URL` | When not mocking | Asgardeo/IS tenant base URL (e.g. `https://api.asgardeo.io/t/<org>`) |
| `GRC_PLATFORM_AUTH_CLIENT_ID` | When not mocking | OAuth 2.0 SPA client ID |
| `GRC_PLATFORM_AUTH_SIGN_IN_REDIRECT_URL` | When not mocking | Post-login redirect (must match the IdP callback allowlist) |
| `GRC_PLATFORM_AUTH_SIGN_OUT_REDIRECT_URL` | When not mocking | Post-logout redirect (must match the IdP logout allowlist) |
| `GRC_PLATFORM_BACKEND_BASE_URL` | For real data | GRC backend REST base URL (no trailing slash) |
| `GRC_PLATFORM_THEME` | No | Oxygen UI theme: `acrylicOrange` (default), `acrylicPurple`, `highContrast`, `classic` |
| `GRC_PLATFORM_LOG_LEVEL` | No | Console log level: `ERROR`, `WARN`, `INFO`, `DEBUG` |

### Authentication modes

- **Mock (default):** `GRC_PLATFORM_MOCK_AUTH: true` — no Asgardeo needed. The
  auth guard is bypassed and the header shows a local "Dev User".
- **Real:** set `GRC_PLATFORM_MOCK_AUTH` to `false` and fill the `GRC_PLATFORM_AUTH_*`
  keys. Register a **Single Page Application** in Asgardeo and align its
  callback/logout URLs with the redirect keys above.

## Project Structure

```text
webapp/
├── public/
│   └── config.js            # Runtime config (window.config), loaded by index.html
├── src/
│   ├── components/          # Shared UI: header, sidebar, footer, error pages, banners
│   ├── config/              # Reads window.config (auth, api, theme, logger)
│   ├── constants/           # Shared constants
│   ├── context/             # App-wide providers (loader, error/success banners, logger)
│   ├── hooks/               # Shared hooks (logger, auth API client, responsive)
│   ├── layouts/             # App shell, auth guard, error layout
│   ├── modules/             # Feature modules
│   │   ├── registry.ts      # Module definitions (Audit Hub, Risk Hub)
│   │   ├── audit/           # Audit Hub — pages, constants (roles), …
│   │   └── risk/            # Risk Hub — pages, constants (roles), …
│   ├── providers/           # Cross-cutting providers (idle-timeout session guard)
│   ├── utils/               # Shared utilities
│   ├── App.tsx              # Routes
│   ├── AppWithConfig.tsx    # Providers (Asgardeo, Query, theme, logger)
│   └── main.tsx             # Entry point
├── index.html               # Loads /config.js before the bundle
└── vite.config.ts           # Aliases, dev server port, env prefix
```

### Modules

Domain code lives under `src/modules/`. Each module owns its pages, routes, and
sidebar nav, and is registered in `src/modules/registry.ts`.

| Module | Base path | Purpose |
|--------|-----------|---------|
| `audit` | `/audit` | Audit Hub — controls, evidence, audit workflows |
| `risk` | `/risk` | Risk Hub — risk register and assessments |

Each module owns these files:

```
src/modules/audit/                    src/modules/risk/
├── routes.tsx   → auditRoutes        ├── routes.tsx   → riskRoutes
├── nav.ts       → auditNav           ├── nav.ts       → riskNav
└── pages/...                         └── pages/...
            ↓ imported & spread by ↓
App.tsx:      <Route>{auditRoutes}{riskRoutes}</Route>
SideBar.tsx:  SECTIONS = [auditNav, riskNav]   (maps over them)
```

This **registration pattern** keeps the Audit and Risk owners working in separate
files so they don't cause merge conflicts.

**To add a page** (e.g. an Audit controls list):

1. Create the page in `src/modules/audit/pages/`.
2. Register its route in `src/modules/audit/routes.tsx`.
3. Add its sidebar item in `src/modules/audit/nav.ts`.

You never edit `App.tsx` or `SideBar.tsx` for normal page work — they just import
and spread each module's `routes` / `nav`. The Risk owner does the same in their
own files.

**Ownership / conflict map:**

| File | Edited by | Conflict risk |
|------|-----------|---------------|
| `modules/audit/{routes,nav}` + `pages/**` | Audit owner only | none |
| `modules/risk/{routes,nav}` + `pages/**` | Risk owner only | none |
| `App.tsx`, `SideBar.tsx` | only when adding a whole new module | near-zero |

## Import Aliases

Defined in `vite.config.ts` and `tsconfig.app.json` — prefer these over deep
relative imports:

| Alias | Resolves to |
|-------|-------------|
| `@/` | `src/` |
| `@assets/` | `src/assets/` |
| `@components/` | `src/components/` |
| `@config/` | `src/config/` |
| `@constants/` | `src/constants/` |
| `@context/` | `src/context/` |
| `@hooks/` | `src/hooks/` |
| `@layouts/` | `src/layouts/` |
| `@modules/` | `src/modules/` |
| `@providers/` | `src/providers/` |
| `@utils/` | `src/utils/` |

## Logging

A small logger respects `GRC_PLATFORM_LOG_LEVEL`. Use the `useLogger` hook:

```typescript
import { useLogger } from "@hooks/useLogger";

const logger = useLogger();
logger.info("Loaded assigned controls");
```

Configuration is resolved in `src/config/loggerConfig.ts`.

## Troubleshooting

| Symptom | Likely cause |
|---------|--------------|
| `npm install` fails on peer deps | Use `npm install --legacy-peer-deps` (oxygen-ui pins React) |
| Blank page / console error at startup | `public/config.js` missing or malformed |
| Stuck on a login redirect | `GRC_PLATFORM_MOCK_AUTH` is `false` but `GRC_PLATFORM_AUTH_*` keys are empty or mismatched with the IdP |
| Config change not applied | Restart the dev server or hard-refresh; `config.js` is read once at load |
| API 401 / no data | Backend not running, wrong `GRC_PLATFORM_BACKEND_BASE_URL`, or token not forwarded |
| VS Code shows import errors but `npm run build` passes | Restart the TS server: ⌘⇧P → "TypeScript: Restart TS Server" |
