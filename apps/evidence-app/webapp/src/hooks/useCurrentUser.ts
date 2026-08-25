import { useQuery } from "@tanstack/react-query";
import { isAxiosError } from "axios";
import { meApi } from "../api/client";

export type CurrentUser = { email: string; role: string };

// Fallback for when a proxy or gateway strips the response body: the person
// must still see why they were refused, not a blank page.
const FALLBACK_FORBIDDEN_MESSAGE =
  "You do not have access to the Evidence App. Ask an administrator to assign you the compliance evidence engineer role in Asgardeo.";

/**
 * Returns the currently logged-in user (from /api/me) — the Asgardeo JWT
 * principal, resolved by the backend from the Bearer token.
 *
 * Convenience flags:
 *   isAdmin          — show admin UI (Cost page, delete buttons, etc.)
 *   isLoaded         — first /me roundtrip has finished
 *   isForbidden      — /me came back 403: signed in, but no role in this app
 *   forbiddenMessage — the backend's own explanation, for isForbidden
 */
export function useCurrentUser() {
  const query = useQuery<CurrentUser>({
    queryKey: ["me"],
    queryFn: meApi.whoami,
    staleTime: 5 * 60 * 1000, // 5 min — identity rarely changes within a session
    // A 4xx here will never turn into a 200 on retry, so retrying wastes
    // seconds a person with no role would spend staring at a spinner.
    // 408 and 429 are the exceptions: a timeout or a rate limit can succeed
    // on a second try. Everything else still gets one retry rather than the
    // library default of three, so a real transient failure still recovers.
    retry: (failureCount, error) => {
      const status = isAxiosError(error) ? error.response?.status : undefined;
      if (status !== undefined && status >= 400 && status < 500 && status !== 408 && status !== 429) {
        return false;
      }
      return failureCount < 1;
    },
  });

  const axiosError = isAxiosError(query.error) ? query.error : undefined;
  const isForbidden = axiosError?.response?.status === 403;
  const detail = (axiosError?.response?.data as { detail?: string } | undefined)?.detail;

  return {
    user: query.data,
    isLoaded: !query.isPending,
    isAdmin: query.data?.role === "admin",
    isEngineer: query.data?.role === "engineer",
    error: query.error,
    isForbidden,
    forbiddenMessage: isForbidden ? (detail ?? FALLBACK_FORBIDDEN_MESSAGE) : undefined,
  };
}
