# Compliance Evidence Submission Portal

A WSO2-internal tool for collecting and submitting compliance evidence
(screenshots from cloud consoles) against security-framework controls. Engineers
use a browser-automation AI agent to navigate cloud portals, capture
screenshots, and store them as evidence linked to compliance controls.

## Components

| Folder | What it is | Runs on |
| --- | --- | --- |
| [`backend/`](backend) | FastAPI REST API + PostgreSQL (Neon) + Azure Blob storage | Choreo (service) |
| [`webapp/`](webapp) | React + TypeScript single-page app (Vite, MUI) | Choreo (web app) |
| [`runner/`](runner) | `wso2-runner` local browser-automation agent | Engineer's machine |

```text
webapp (React/Vite)  ──▶  backend (FastAPI)  ──▶  PostgreSQL (Neon)
                                 ▲                 Azure Blob (evidence)
                                 │
                     runner (wso2-runner, local machine)
```

The **runner** runs locally — never in Docker or Choreo — because it needs a
headful Chromium for SSO/MFA login and OS-level screen capture.

## Authentication

Both the web app and the runner authenticate to the backend with **Asgardeo**
OAuth2 tokens. The backend validates every request against Asgardeo's UserInfo
endpoint — the same way locally and in production.

## Getting started

Each component has its own README with setup instructions:

- [Backend](backend/README.md)
- [Web app](webapp/README.md)
- [Runner](runner/README.md)

## Deployment

The backend and web app deploy to **Choreo** (see each component's
`.choreo/component.yaml`). PostgreSQL is hosted on Neon and evidence files in
Azure Blob Storage.
