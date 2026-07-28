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
import { useAsgardeo } from "@asgardeo/react";
import { Box, Button, LinearProgress, Typography } from "@wso2/oxygen-ui";
import AppLayout from "@layouts/AppLayout";

const isMockAuth = window.config?.GRC_PLATFORM_MOCK_AUTH === true;

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
    // Grace period: don't redirect to the login page the instant isSignedIn
    // is falsy — give the SDK a brief window to finish hydrating an existing
    // session from storage first (isSignedIn can start out false/undefined
    // even for an already-valid session). Any change to isSignedIn cancels
    // this timer via the effect's own cleanup and reschedules a fresh one,
    // so a flip to true (at any point) always cancels a pending redirect;
    // signIn() only actually fires if isSignedIn stays falsy for the full
    // uninterrupted window.
    const timer = setTimeout(() => {
      if (!hasTriggeredSignInRef.current) {
        triggerSignIn();
      }
    }, 500);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- signIn deliberately omitted: not guaranteed reference-stable, and including it would reset this grace-period timer on every unrelated render, potentially preventing it from ever firing
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
