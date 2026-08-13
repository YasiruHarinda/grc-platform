/**
 * Keeps an evidence screenshot's `<img src>` stable across refetches, so the
 * browser can reuse the picture it already downloaded instead of fetching it
 * again under a new URL.
 *
 * The backend signs a fresh Azure SAS URL every time it serializes evidence
 * (`backend/app/schemas/evidence.py`) — different signature, different `se=`
 * expiry, on every single response, even when the underlying blob has not
 * changed. That is correct and deliberate (ADR 0003: short-lived signed
 * links, no proxy route), but it means the browser has no way to tell two
 * signings of the same picture apart, so it re-downloads it every time.
 *
 * The fix has to live somewhere that outlives a single render, because the
 * whole point is to keep returning the *previous* render's URL. A
 * module-level `Map` does that: it survives refetches and remounts for as
 * long as the tab is open, which is exactly the lifetime we want.
 *
 * The map is keyed on the URL's **path**, not its file id or its full
 * string. The path is the blob's identity — it's the one part of a SAS URL
 * that a re-signing never changes. (Keying on an id instead is tempting but
 * wrong here: `EvidenceList.tsx`'s `galleryFiles` synthesizes a fallback
 * entry whose `id` is an *Evidence* id, not an *EvidenceFile* id, so two
 * different id spaces could collide in one map. A blob path can't collide.)
 *
 * A stored URL is handed back as long as its own `se=` expiry is more than
 * `MIN_REMAINING_MS` in the future. That slack exists so a URL just handed
 * to `<img>` can't expire while the request is still in flight — once a
 * link gets within two minutes of expiring, the fresh one takes over.
 */

// How much life a cached URL must still have left before it's reused.
// Comfortably longer than any realistic image request takes, comfortably
// shorter than the backend's signed-link lifetime, so the swap happens
// long before anything currently on screen could break.
const MIN_REMAINING_MS = 2 * 60 * 1000;

const cache = new Map<string, string>();

// Reads the SAS `se=` (signed expiry) query parameter off a URL and returns
// it as epoch milliseconds, or null if the URL is relative, malformed, or
// simply doesn't carry one. Callers treat null as "can't judge freshness" —
// never as a reason to break the image.
function expiryMs(url: string): number | null {
  try {
    const se = new URL(url).searchParams.get("se");
    if (!se) return null;
    const ms = Date.parse(se);
    return Number.isNaN(ms) ? null : ms;
  } catch {
    return null;
  }
}

// Same "don't throw on anything" reasoning as expiryMs: a URL we can't
// parse can't be cached, so it's returned as-is and the map is left alone.
function pathKey(url: string): string | null {
  try {
    return new URL(url).pathname;
  } catch {
    return null;
  }
}

/**
 * Returns a URL for the same blob that's safe to hand to `<img src>` right
 * now: the previously-seen URL if it still has plenty of life left, or the
 * fresh one otherwise (storing it for next time).
 *
 * Only use this for `<img>` sources. Downloads and "open in new tab" want
 * the freshest link available, not the longest-lived one — caching buys
 * them nothing, since a download isn't a repeated request.
 */
export function stableFileUrl(url: string): string {
  const key = pathKey(url);
  if (key == null) return url;

  const expiresAt = expiryMs(url);
  if (expiresAt == null) return url;

  const cached = cache.get(key);
  if (cached != null) {
    const cachedExpiry = expiryMs(cached);
    if (cachedExpiry != null && cachedExpiry - Date.now() > MIN_REMAINING_MS) {
      return cached;
    }
  }

  cache.set(key, url);
  return url;
}

/**
 * Drops one blob's cached URL. Called when an `<img>` using it fails to
 * load, so the retry that follows adopts the freshly re-signed URL instead
 * of handing back the same broken one.
 */
export function forgetFileUrl(url: string): void {
  const key = pathKey(url);
  if (key == null) return;
  cache.delete(key);
}

/**
 * Drops every cached URL. Called on sign-out, alongside the query client's
 * own `clear()`, so the next person to sign in on this machine never has a
 * stale entry handed back to them. In practice this rarely runs — Asgardeo's
 * `signOut()` redirects, which wipes all module state anyway — but sign-out
 * has a failure path where the redirect doesn't happen and the page stays
 * put, so it's here for that case.
 */
export function clearFileUrlCache(): void {
  cache.clear();
}
