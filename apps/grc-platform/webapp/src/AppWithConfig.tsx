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

import { type JSX } from "react";
import { BrowserRouter } from "react-router";
import { GlobalStyles, OxygenUIThemeProvider } from "@wso2/oxygen-ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import App from "./App";
import { AsgardeoProvider } from "@asgardeo/react";
import { loggerConfig } from "@config/loggerConfig";
import LoggerProvider from "@context/logger/LoggerProvider";
import { authConfig } from "@config/authConfig";
import { ThemePreferenceProvider } from "@context/theme/ThemePreferenceContext";
import { useThemePreference } from "@context/theme/useThemePreference";

/**
 * Custom retry function for React Query.
 * Only retries on 502 (Bad Gateway) and 503 (Service Unavailable) errors.
 *
 * @param {number} failureCount - The number of times the request has failed.
 * @param {Error} error - The error that occurred.
 * @returns {boolean} True if the request should be retried, false otherwise.
 */
function shouldRetry(failureCount: number, error: Error): boolean {
  // Max 2 retries (3 total attempts including the initial one)
  if (failureCount >= 2) {
    return false;
  }

  // Check if error has a status code property
  const errorWithStatus = error as Error & {
    response?: { status?: number };
    status?: number;
  };
  const statusCode = errorWithStatus.response?.status || errorWithStatus.status;

  // Only retry on 502 (Bad Gateway) and 503 (Service Unavailable)
  return statusCode === 502 || statusCode === 503;
}

const queryClient: QueryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: shouldRetry,
      retryDelay: (attemptIndex) => Math.min(1000 * 2 ** attemptIndex, 30000),
      // Nothing pushes status changes to other users' browsers, so a tab left
      // open goes stale as soon as someone else acts. Refetching on focus and
      // reconnect makes returning to the tab the refresh signal.
      refetchOnWindowFocus: true,
      refetchOnReconnect: true,
      refetchOnMount: true,
    },
    mutations: {
      retry: shouldRetry,
      retryDelay: (attemptIndex) => Math.min(1000 * 2 ** attemptIndex, 30000),
    },
  },
});

function ThemedApp(): JSX.Element {
  const { theme } = useThemePreference();
  return (
    <OxygenUIThemeProvider theme={theme}>
      <GlobalStyles
        styles={{
          "html, body, #root": {
            width: "100%",
            maxWidth: "100vw",
            overflowX: "clip",
          },
        }}
      />
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </OxygenUIThemeProvider>
  );
}

export default function AppWithConfig(): JSX.Element {
  return (
    <AsgardeoProvider
      baseUrl={authConfig.baseUrl}
      clientId={authConfig.clientId}
      afterSignInUrl={authConfig.signInRedirectURL}
      afterSignOutUrl={authConfig.signOutRedirectURL}
      tokenLifecycle={{ refreshToken: { autoRefresh: true } }}
      scopes={["openid", "email", "groups", "profile"]}
      preferences={{
        theme: {
          inheritFromBranding: false,
        },
        user: {
          fetchUserProfile: false,
          fetchOrganizations: false,
        },
      }}
    >
      <BrowserRouter>
        <LoggerProvider config={loggerConfig}>
          <ThemePreferenceProvider>
            <ThemedApp />
          </ThemePreferenceProvider>
        </LoggerProvider>
      </BrowserRouter>
    </AsgardeoProvider>
  );
}
