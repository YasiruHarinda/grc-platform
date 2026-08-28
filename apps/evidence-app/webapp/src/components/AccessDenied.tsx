import { useState } from "react";
import { useAuthContext } from "@asgardeo/auth-react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import Snackbar from "@mui/material/Snackbar";
import Alert from "@mui/material/Alert";
import { useCurrentUser } from "../hooks/useCurrentUser";
import { clearFileUrlCache } from "../utils/stableFileUrl";

/**
 * Shown instead of the app shell when /api/me comes back 403: signed in to
 * Asgardeo, but holding none of this app's roles. No sidebar, no navbar, no
 * routes - this renders above the Router (see App.tsx), so it must not use
 * useNavigate, Link, useLocation or <Navigate>, none of which have a Router
 * to attach to here.
 *
 * There is deliberately no Retry or Reload button. The role is carried in
 * the access token (backend/app/auth.py), and a token already issued keeps
 * the claims it was minted with, so no amount of retrying this page can
 * ever succeed. Signing out and back in is the only cure, which is why
 * Sign out is the one action on this page.
 */
export default function AccessDenied() {
  const { state, signOut } = useAuthContext();
  const { forbiddenMessage } = useCurrentUser();
  const [signOutError, setSignOutError] = useState<string | null>(null);

  const account = state.email ?? state.username;

  const handleSignOut = () => {
    // Unlike Navbar's handleSignOut, this does NOT call queryClient.clear().
    // Clearing would drop the ["me"] query, which would refetch, show the
    // loading spinner in App.tsx, and remount this component fresh - wiping
    // out signOutError before anyone could read it. There is also nothing
    // useful to clear: the only thing cached for this person is the 403.
    clearFileUrlCache();
    signOut().catch((err) => {
      console.error("Sign-out failed:", err);
      setSignOutError(
        "Sign-out failed. You are still signed in. Close the browser before leaving this machine.",
      );
    });
  };

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        height: "100vh",
        textAlign: "center",
        gap: 2,
        px: 3,
      }}
    >
      <Typography variant="h4">You don't have access to the Evidence App</Typography>
      <Typography color="text.secondary" sx={{ maxWidth: 480 }}>
        {forbiddenMessage}
      </Typography>
      {account && (
        <Typography color="text.secondary">
          Signed in as <strong>{account}</strong>.
        </Typography>
      )}
      <Typography color="text.secondary" sx={{ maxWidth: 480 }}>
        Your role is granted the moment you sign in, so reloading this page will not help. Once an
        administrator has assigned you a role, sign out and sign in again to pick it up.
      </Typography>
      <Button variant="contained" size="large" onClick={handleSignOut} sx={{ mt: 1 }}>
        Sign Out
      </Button>

      {/* Deliberately does not auto-hide: this one has to be read and
          dismissed, not missed while walking away from the machine. */}
      <Snackbar
        open={signOutError != null}
        onClose={() => setSignOutError(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert onClose={() => setSignOutError(null)} severity="error" variant="filled" sx={{ width: "100%" }}>
          {signOutError}
        </Alert>
      </Snackbar>
    </Box>
  );
}
