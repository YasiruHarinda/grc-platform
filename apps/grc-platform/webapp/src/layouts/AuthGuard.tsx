// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import { type JSX, useEffect, useRef, useState } from "react";
import { Navigate, useLocation } from "react-router";
import { useAsgardeo, useBrowserUrl } from "@asgardeo/react";
import { Box, Button, LinearProgress, Typography } from "@wso2/oxygen-ui";
import AppLayout from "@layouts/AppLayout";
import { authConfig } from "@config/authConfig";

const isMockAuth = window.config?.GRC_PLATFORM_MOCK_AUTH === true;

// How long RealAuthGuard waits before calling signIn() itself: short while
// hydrating from storage, much longer while an OAuth exchange is in flight.
const SIGN_IN_GRACE_MS = 500;
const AUTH_EXCHANGE_FALLBACK_MS = 10000;

// Parks the deep link across the OAuth round trip, which always returns to the
// fixed afterSignInUrl and would otherwise lose it.
const RETURN_TO_KEY = "grc:auth:returnTo";

// Pure read — runs as a useState initializer, which StrictMode double-invokes.
// Clearing here would hand the second call a null. Storage can throw when
// blocked; deep links just don't survive sign-in then.
function readReturnTo(): string | null {
  try {
    return sessionStorage.getItem(RETURN_TO_KEY);
  } catch {
    return null;
  }
}

function clearReturnTo(): void {
  try {
    sessionStorage.removeItem(RETURN_TO_KEY);
  } catch {
    // Ignore: see readReturnTo.
  }
}

// Skips "/" (where we'd land anyway) and spent OAuth callback params.
function saveReturnTo(): void {
  const { pathname, search } = window.location;
  if (pathname === "/" || search.includes("code=")) return;
  try {
    sessionStorage.setItem(RETURN_TO_KEY, pathname + search);
  } catch {
    // Ignore: see readReturnTo.
  }
}

const authLoader = (
  <Box
    sx={{
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      height: "100dvh",
    }}
  >
    <LinearProgress
      color="warning"
      sx={{ width: "80%", maxWidth: 400, height: 4 }}
    />
  </Box>
);

export default function AuthGuard(): JSX.Element {
  if (isMockAuth) {
    return <AppLayout />;
  }

  return <RealAuthGuard />;
}

// Drives mounting off isSignedIn, not @asgardeo/react-router's ProtectedRoute:
// its isLoading flaps forever in this SDK version, remounting the whole app
// shell (and its data fetches) on every flicker. AppLayout handles its own
// loading UI, so ProtectedRoute buys nothing here.
function RealAuthGuard(): JSX.Element {
  const { isSignedIn, signIn } = useAsgardeo();
  const { hasAuthParams } = useBrowserUrl();
  const { pathname } = useLocation();
  const hasTriggeredSignInRef = useRef(false);
  const [signInError, setSignInError] = useState(false);
  // Returning from Asgardeo is a fresh page load, so the value saved before
  // signIn() is already in storage at mount. Drop it once it's in state, or it
  // would linger on the paths that never redirect.
  const [returnTo] = useState(readReturnTo);
  useEffect(clearReturnTo, []);

  const triggerSignIn = () => {
    hasTriggeredSignInRef.current = true;
    setSignInError(false);
    saveReturnTo();
    signIn().catch(() => {
      // Reset the guard so the retry button can call signIn() again; without
      // this the user is stuck on the loader with no way back.
      hasTriggeredSignInRef.current = false;
      setSignInError(true);
    });
  };

  useEffect(() => {
    if (isSignedIn) return;
    // Callback params mean an exchange is mid-flight, so wait much longer:
    // racing it fires a redundant signIn(), which SSO silently re-approves,
    // restarting the race — the app "loading again and again". Still bounded,
    // so a failed exchange surfaces the error UI instead of hanging.
    const hasCallbackParams = hasAuthParams(new URL(window.location.href), authConfig.signInRedirectURL);
    // Grace period: isSignedIn can start out falsy even for a valid session,
    // so give hydration a window first. Any flip to true cancels this timer
    // via the effect cleanup, so signIn() fires only if it stays falsy.
    const timer = setTimeout(
      () => {
        if (!hasTriggeredSignInRef.current) {
          triggerSignIn();
        }
      },
      hasCallbackParams ? AUTH_EXCHANGE_FALLBACK_MS : SIGN_IN_GRACE_MS,
    );
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- signIn/hasAuthParams omitted: neither is reference-stable, so including them would reset the timer on every render and it may never fire
  }, [isSignedIn]);

  if (signInError) {
    return (
      <Box
        sx={{
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          gap: 2,
          height: "100dvh",
        }}
      >
        <Typography>Sign-in failed. Please check your connection and try again.</Typography>
        <Button variant="contained" onClick={triggerSignIn}>
          Try again
        </Button>
      </Box>
    );
  }

  if (!isSignedIn) {
    return authLoader;
  }

  // Restore the deep link. During render, not an effect, so the "/" redirect to
  // the dashboard never mounts (child effects run before parent ones).
  // Gated on "/" — the only place a restore is wanted. Comparing against
  // returnTo instead loops forever once a page rewrites its own query string
  // (AuditDetailPage strips ?control=), blanking the app.
  if (returnTo && pathname === "/") {
    return <Navigate to={returnTo} replace />;
  }

  return <AppLayout />;
}
