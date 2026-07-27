import { useQuery } from "@tanstack/react-query";
import { meApi } from "../api/client";

export type CurrentUser = { email: string; role: string };

/**
 * Returns the currently logged-in user (from /api/me) — the Asgardeo JWT
 * principal, resolved by the backend from the Bearer token.
 *
 * Convenience flags:
 *   isAdmin   — show admin UI (Cost page, delete buttons, etc.)
 *   isLoaded  — first /me roundtrip has finished
 */
export function useCurrentUser() {
  const query = useQuery<CurrentUser>({
    queryKey: ["me"],
    queryFn: meApi.whoami,
    staleTime: 5 * 60 * 1000, // 5 min — identity rarely changes within a session
  });

  return {
    user: query.data,
    isLoaded: !query.isPending,
    isAdmin: query.data?.role === "admin",
    isEngineer: query.data?.role === "engineer",
    error: query.error,
  };
}
