// NOT IN USE on Choreo's React buildpack (see issue #90) — that buildpack
// builds dist/ and serves it as static files behind a CDN, so this process
// never runs. Kept, with the Dockerfile below, so the component can be
// switched back to the Dockerfile buildpack without a code change; runtime
// config then comes from `public/config.js` either way (see
// src/config/apiConfig.ts and README.md), not from BACKEND_URL below.
//
// Production web server for Choreo.
//
// Serves the built Vite app from ./dist and proxies /api to the
// backend. The Authorization: Bearer header is forwarded unchanged so the
// backend can validate the Asgardeo token itself.
import http from 'http';
import https from 'https';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PORT = process.env.PORT || 8080;
const DIST = path.join(__dirname, 'dist');

// Public backend base URL. On Choreo this is the backend component's public
// gateway URL; locally it defaults to the dev backend.
const BACKEND_URL = (process.env.BACKEND_URL || 'http://localhost:8000').replace(/\/$/, '');

const MIME_TYPES = {
  '.html': 'text/html',
  '.js': 'application/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
};

// Checklist 3.1 mandatory security headers. Applied to static-file responses
// (serveStatic below) and to the 502 / 504 this server writes itself when the
// backend never answers — never to a response the backend actually produced.
// Those forward the backend's headers through unchanged
// (`res.writeHead(proxyRes.statusCode, proxyRes.headers)` in
// proxyToBackend), including its own Content-Security-Policy and
// Strict-Transport-Security. Attaching these there too would put two CSP
// headers on the same response; browsers intersect multiple CSPs into the
// most restrictive combination, silently producing a policy nobody wrote.
// The 502 / 504 path has no such conflict: the backend either never connected
// or never sent headers, so there is nothing to collide with.
//
// The restrictive CSP directives (default-src 'self' etc.) and
// Permissions-Policy are a separate, deferred piece of work (see issue #75)
// — not added here. `preload` on HSTS is a deliberate inclusion for the web
// app only; see issue #72.
const STATIC_SECURITY_HEADERS = {
  'X-Frame-Options': 'deny',
  'X-Content-Type-Options': 'nosniff',
  'X-Permitted-Cross-Domain-Policies': 'None',
  'Content-Security-Policy': "upgrade-insecure-requests; frame-ancestors 'none'",
  'Strict-Transport-Security': 'max-age=31536000; includeSubDomains; preload',
  'Referrer-Policy': 'no-referrer',
};

function proxyToBackend(req, res) {
  const target = new URL(BACKEND_URL + req.url);
  const isHttps = target.protocol === 'https:';
  const lib = isHttps ? https : http;

  // Forward all headers except host/transfer-encoding, keeping Authorization
  // intact so the backend can validate the Asgardeo JWT.
  const forwardHeaders = {};
  for (const [key, value] of Object.entries(req.headers)) {
    const lower = key.toLowerCase();
    if (lower !== 'transfer-encoding' && lower !== 'host') {
      forwardHeaders[key] = value;
    }
  }

  const options = {
    hostname: target.hostname,
    port: target.port || (isHttps ? 443 : 80),
    path: target.pathname + target.search,
    method: req.method,
    headers: forwardHeaders,
  };

  // `responded` guards the client's response head (only one of headers / 504 /
  // 502 may write it). `finished` tracks the upstream lifecycle, and is what
  // client-disconnect cleanup keys off — kept separate so a client that drops
  // mid-stream (after headers) still tears the upstream down.
  let responded = false;
  let finished = false;

  const proxyReq = lib.request(options, (proxyRes) => {
    // Headers arrived: the deadline bounds only time-to-first-headers, never
    // the streaming body, so clear it here. This keeps SSE alive — once this
    // fires the long-lived live-run stream can run indefinitely.
    clearTimeout(headersTimer);
    responded = true;
    res.writeHead(proxyRes.statusCode, proxyRes.headers);
    proxyRes.pipe(res);
    proxyRes.on('end', () => { finished = true; });
  });

  // Bound only how long we wait for the upstream to start responding. A
  // stalled backend that never sends headers would otherwise hold this socket
  // open forever. This never bounds the streaming body (see above).
  const headersTimeoutMs = Number(process.env.PROXY_HEADERS_TIMEOUT_MS) || 30000;
  const headersTimer = setTimeout(() => {
    if (responded) return;
    responded = true;
    finished = true;
    proxyReq.destroy(new Error('Upstream response headers timed out'));
    if (!res.headersSent) {
      res.writeHead(504, STATIC_SECURITY_HEADERS);
      res.end('Gateway Timeout');
    }
  }, headersTimeoutMs);

  proxyReq.on('error', (err) => {
    clearTimeout(headersTimer);
    finished = true;
    if (responded) return;  // already streaming; the broken pipe ends the response
    responded = true;
    console.error('Proxy error:', err.code, err.message, '→', target.href);
    if (!res.headersSent) {
      res.writeHead(502, STATIC_SECURITY_HEADERS);
      res.end('Bad Gateway');
    }
  });
  proxyReq.on('close', () => { finished = true; clearTimeout(headersTimer); });

  // Free the upstream socket if the client disconnects before the upstream has
  // finished. This fires whether or not headers arrived, so a client that
  // closes mid-SSE-stream still tears the upstream down. Guarded by `finished`
  // so it never fires after normal completion.
  const onClientGone = () => {
    if (finished) return;
    finished = true;
    clearTimeout(headersTimer);
    proxyReq.destroy();
  };
  req.on('aborted', onClientGone);
  res.on('close', onClientGone);

  req.pipe(proxyReq);
}

function serveStatic(req, res) {
  const url = req.url.split('?')[0];
  let filePath = path.join(DIST, url === '/' ? 'index.html' : url);

  // Resolve DIST and the candidate path so we can detect requests that
  // escape DIST via `..` segments (e.g. `/../../../etc/passwd`) before
  // touching the filesystem.
  const resolvedDist = path.resolve(DIST);
  const resolvedPath = path.resolve(filePath);
  const isInsideDist =
    resolvedPath === resolvedDist || resolvedPath.startsWith(resolvedDist + path.sep);

  // Fall back to index.html so client-side routing works, and also for any
  // path that resolves outside DIST (path traversal attempt).
  if (!isInsideDist || !fs.existsSync(filePath) || fs.statSync(filePath).isDirectory()) {
    filePath = path.join(DIST, 'index.html');
  }

  const contentType = MIME_TYPES[path.extname(filePath).toLowerCase()] || 'application/octet-stream';
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404, STATIC_SECURITY_HEADERS);
      res.end('Not found');
      return;
    }
    res.writeHead(200, { ...STATIC_SECURITY_HEADERS, 'Content-Type': contentType });
    res.end(data);
  });
}

http
  .createServer((req, res) => {
    const url = req.url.split('?')[0];
    if (url.startsWith('/api')) {
      proxyToBackend(req, res);
    } else {
      serveStatic(req, res);
    }
  })
  .listen(PORT, () => console.log(`Frontend on :${PORT} → ${BACKEND_URL}`));
