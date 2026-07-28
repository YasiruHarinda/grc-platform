import axios from "axios";

// Evidence file URLs come from the backend as short-lived Azure signed
// links (absolute URLs pointing directly at blob storage) and are used
// as-is in <img> src with no Authorization header, so this is an identity pass-through.
export const getFileUrl = (fileUrl: string): string => fileUrl;

export const api = axios.create({
  baseURL: "/api",
});

// ---- Auth bridge -------------------------------------------------------
// The Asgardeo SDK lives in React context; axios does not. App.tsx registers
// the SDK's token accessors here so every request attaches a *fresh* token and
// can recover from expiry — instead of caching one token string forever (which
// broke every API call ~1h after login, once the cached token expired).
type AuthHooks = {
  getAccessToken: () => Promise<string>;
  refreshAccessToken: () => Promise<unknown>;
  onAuthLost?: () => void;
};

let _auth: AuthHooks | null = null;

export function registerAuth(hooks: AuthHooks | null) {
  _auth = hooks;
}

/**
 * Current access token, or null when unauthenticated. Reads from the Asgardeo
 * SDK on every call, so a token the SDK refreshed in the background is picked up
 * automatically. Used by axios (below) and by the SSE fetch in AgentRunner,
 * which cannot go through the axios interceptors.
 */
export async function getAuthToken(): Promise<string | null> {
  if (!_auth) return null;
  try {
    return await _auth.getAccessToken();
  } catch {
    return null;
  }
}

api.interceptors.request.use(async (config) => {
  const token = await getAuthToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Concurrent 401s must share a single in-flight refresh instead of each
// triggering their own — with refresh-token rotation, only the first of a
// batch of parallel refreshes succeeds and the rest would otherwise fall
// into onAuthLost() and needlessly sign the user out.
let _refreshPromise: Promise<unknown> | null = null;

function refreshAccessTokenOnce(): Promise<unknown> {
  if (!_refreshPromise) {
    _refreshPromise = Promise.resolve(_auth!.refreshAccessToken()).finally(() => {
      _refreshPromise = null;
    });
  }
  return _refreshPromise;
}

// On a 401, force one silent refresh via the SDK and retry the request once.
// If the refresh itself fails (refresh token expired), hand back to sign-in.
api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const original = error.config as
      | (typeof error.config & { _retried?: boolean })
      | undefined;
    if (
      _auth &&
      original &&
      !original._retried &&
      error.response?.status === 401
    ) {
      original._retried = true;
      try {
        await refreshAccessTokenOnce();
        return api(original);
      } catch {
        _auth.onAuthLost?.();
      }
    }
    return Promise.reject(error);
  }
);

export const meApi = {
  whoami: () => api.get("/me").then((r) => r.data as { email: string; role: string }),
};

export const productsApi = {
  list: () => api.get("/products").then((r) => r.data),
  create: (data: { name: string; description?: string }) =>
    api.post("/products", data).then((r) => r.data),
  update: (id: number, data: { name?: string; description?: string }) =>
    api.patch(`/products/${id}`, data).then((r) => r.data),
  delete: (id: number) => api.delete(`/products/${id}`),
};

export const frameworksApi = {
  list: (productId?: number) =>
    api
      .get("/frameworks", { params: productId ? { product_id: productId } : {} })
      .then((r) => r.data),
  create: (data: { product_id: number; name: string; description?: string }) =>
    api.post("/frameworks", data).then((r) => r.data),
  update: (id: number, data: { name?: string; description?: string }) =>
    api.patch(`/frameworks/${id}`, data).then((r) => r.data),
  delete: (id: number) => api.delete(`/frameworks/${id}`),
};

export const controlsApi = {
  list: (frameworkId?: number) =>
    api
      .get("/controls", { params: frameworkId ? { framework_id: frameworkId } : {} })
      .then((r) => r.data),
  create: (data: { framework_id: number; control_ref: string; title: string; description?: string }) =>
    api.post("/controls", data).then((r) => r.data),
  update: (id: number, data: { control_ref?: string; title?: string; description?: string }) =>
    api.patch(`/controls/${id}`, data).then((r) => r.data),
  delete: (id: number) => api.delete(`/controls/${id}`),
};

export const evidenceApi = {
  list: () => api.get("/evidence").then((r) => r.data),
  get: (id: number) => api.get(`/evidence/${id}`).then((r) => r.data),
  create: (formData: FormData) =>
    api.post("/evidence", formData, {
      headers: { "Content-Type": "multipart/form-data" },
    }).then((r) => r.data),
  rename: (id: number, description: string) =>
    api.patch(`/evidence/${id}`, { description }).then((r) => r.data),
  delete: (id: number) => api.delete(`/evidence/${id}`),
  deleteFile: (fileId: number) => api.delete(`/evidence/files/${fileId}`),
};

export const submissionsApi = {
  list: () => api.get("/submissions").then((r) => r.data),
  create: (data: { evidence_id: number; submitted_by: string; notes?: string }) =>
    api.post("/submissions", data).then((r) => r.data),
  updateStatus: (id: number, status: string) =>
    api.patch(`/submissions/${id}`, { status }).then((r) => r.data),
};

export const usageApi = {
  summary: () => api.get("/usage/summary").then((r) => r.data),
  timeseries: (days = 30) =>
    api.get("/usage/timeseries", { params: { days } }).then((r) => r.data),
  byModel: () => api.get("/usage/by-model").then((r) => r.data),
  recent: (limit = 20) =>
    api.get("/usage/recent", { params: { limit } }).then((r) => r.data),
};

export const agentApi = {
  createTask: (data: { prompt: string; control_id?: number; title?: string; region_hint?: string; portal_url?: string; kind?: "run" | "login"; max_steps?: number; use_vision?: boolean; max_actions_per_step?: number }) =>
    api.post("/agent/tasks", data).then((r) => r.data),
  openLoginBrowser: (url: string) =>
    api.post("/agent/tasks", { prompt: url, kind: "login" }).then((r) => r.data),
  resetBrowser: () =>
    api.post("/agent/tasks", { prompt: "reset", kind: "reset" }).then((r) => r.data),
  listTasks: (limit = 50) =>
    api.get("/agent/tasks", { params: { limit } }).then((r) => r.data),
  getTask: (taskId: number) =>
    api.get(`/agent/tasks/${taskId}`).then((r) => r.data),
  cancelTask: (taskId: number) =>
    api.post(`/agent/tasks/${taskId}/cancel`).then((r) => r.data),
  resumeTask: (taskId: number) =>
    api.post(`/agent/tasks/${taskId}/resume`).then((r) => r.data),
  runnerStatus: () =>
    api.get("/agent/runner-status").then((r) => r.data as { online: boolean; last_seen: string | null }),
  // Runner-facing — used by the local runner package (Week 3)
  nextTask: (runnerId: string) =>
    api.get("/agent/tasks/next", { params: { runner_id: runnerId } }).then((r) => r.data),
  postProgress: (taskId: number, progress: object) =>
    api.post(`/agent/tasks/${taskId}/progress`, progress).then((r) => r.data),
  postResult: (taskId: number, result: object) =>
    api.post(`/agent/tasks/${taskId}/result`, result).then((r) => r.data),
};
