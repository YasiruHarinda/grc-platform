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
import { useAsgardeo, useBrowserUrl } from "@asgardeo/react";
import { Box, Button, LinearProgress, Typography } from "@wso2/oxygen-ui";
import AppLayout from "@layouts/AppLayout";
import { authConfig } from "@config/authConfig";

const isMockAuth = window.config?.GRC_PLATFORM_MOCK_AUTH === true;

// How long RealAuthGuard waits before giving up and calling signIn() itself.
// Short in the common case (hydrating an existing session from storage);
// much longer while an OAuth callback exchange may be in flight, since that's
// a real network round trip — see the comment where each is used.
const SIGN_IN_GRACE_MS = 500;
const AUTH_EXCHANGE_FALLBACK_MS = 10000;

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

// Drives mounting off isSignedIn rather than @asgardeo/react-router's
// ProtectedRoute, which swaps its loader/children based on isLoading — a
// value confirmed (via debug logging) to flap continuously without ever
// settling in this SDK version, unmounting/remounting the whole app shell
// (and everything inside it, including data-fetching effects) on every
// flicker. isSignedIn has been reliably stable across every test; AppLayout
// already has its own hasInitialized latch to handle the loading UI, so
// ProtectedRoute's built-in loader-swap isn't needed here regardless.
function RealAuthGuard(): JSX.Element {
  const { isSignedIn, signIn } = useAsgardeo();
  const { hasAuthParams } = useBrowserUrl();
  const hasTriggeredSignInRef = useRef(false);
  const [signInError, setSignInError] = useState(false);

  const triggerSignIn = () => {
    hasTriggeredSignInRef.current = true;
    setSignInError(false);
    signIn().catch(() => {
      // Reset the guard so a manual retry (below) is allowed to call
      // signIn() again; an uncaught rejection would otherwise leave the
      // user stuck on the loader forever with no way back to the login page.
      hasTriggeredSignInRef.current = false;
      setSignInError(true);
    });
  };

  useEffect(() => {
    if (isSignedIn) return;
    // The URL still carries the OAuth callback params (code + session_state)
    // right after redirecting back from Asgardeo, for as long as the SDK is
    // exchanging them for a session — which is a real network round trip and
    // routinely takes longer than the short grace period below. Racing the
    // short timer here previously fired a second, redundant signIn()
    // mid-exchange; since the user had just authenticated, Asgardeo's SSO
    // session silently re-approved it, reloading the whole app with a fresh
    // code and restarting the same race — visible as the app "loading again
    // and again" after sign-in until one exchange happened to finish inside
    // the window. Use a much longer fallback while those params are present,
    // so a normal exchange always finishes well within it and never races —
    // but a failed one (stale code, network blip) still eventually surfaces
    // the error UI below instead of leaving the user stuck on the loader
    // forever with no error and no retry button.
    const hasCallbackParams = hasAuthParams(new URL(window.location.href), authConfig.signInRedirectURL);
    // Grace period: don't redirect to the login page the instant isSignedIn
    // is falsy — give the SDK a brief window to finish hydrating an existing
    // session from storage first (isSignedIn can start out false/undefined
    // even for an already-valid session). Any change to isSignedIn cancels
    // this timer via the effect's own cleanup and reschedules a fresh one,
    // so a flip to true (at any point) always cancels a pending redirect;
    // signIn() only actually fires if isSignedIn stays falsy for the full
    // uninterrupted window.
    const timer = setTimeout(
      () => {
        if (!hasTriggeredSignInRef.current) {
          triggerSignIn();
        }
      },
      hasCallbackParams ? AUTH_EXCHANGE_FALLBACK_MS : SIGN_IN_GRACE_MS,
    );
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- signIn/hasAuthParams deliberately omitted: neither is guaranteed reference-stable, and including them would reset this grace-period timer on every unrelated render, potentially preventing it from ever firing
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

  return <AppLayout />;
}
