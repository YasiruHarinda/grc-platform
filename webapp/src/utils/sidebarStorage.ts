const SIDEBAR_COLLAPSED_KEY = "sidebar_collapsed";
const PENDING_SUCCESS_MESSAGE_KEY = "pending_success_message";

export function getSidebarCollapsed(): boolean {
  try {
    const stored = localStorage.getItem(SIDEBAR_COLLAPSED_KEY);
    if (stored === null) return false;
    return stored === "true";
  } catch {
    return false;
  }
}

export function setSidebarCollapsed(collapsed: boolean): void {
  try {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(collapsed));
  } catch {
    return;
  }
}

export function consumePendingSuccessMessage(): string | null {
  try {
    const msg = sessionStorage.getItem(PENDING_SUCCESS_MESSAGE_KEY);
    if (msg !== null) sessionStorage.removeItem(PENDING_SUCCESS_MESSAGE_KEY);
    return msg;
  } catch {
    return null;
  }
}
