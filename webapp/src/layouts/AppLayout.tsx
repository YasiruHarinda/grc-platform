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

import {
  Box,
  useAppShell,
  useMediaQuery,
  useTheme,
  LinearProgress,
  Typography,
} from "@wso2/oxygen-ui";
import AppShellLayout from "@layouts/AppShellLayout";
import { type JSX, type ReactNode, useRef, useEffect, useState } from "react";
import { useAsgardeo } from "@asgardeo/react";
import { useLoader } from "@context/linear-loader/LoaderContext";
import { useLocation, Outlet } from "react-router";
import IdleTimeoutProvider from "@providers/IdleTimeoutProvider";
import Footer from "@components/footer/Footer";
import Header from "@components/header/Header";
import SideBar from "@components/side-nav-bar/SideBar";
import {
  getSidebarCollapsed,
  setSidebarCollapsed,
} from "@utils/sidebarStorage";
import { useIsMidSizeTouchViewport } from "@hooks/useResponsiveLayout";

interface AppLayoutProps {
  children?: ReactNode;
}

export default function AppLayout({ children }: AppLayoutProps): JSX.Element {
  const location = useLocation();
  const { isLoading: isAuthLoading } = useAsgardeo();

  useEffect(() => {
    document.getElementById("main-scroll-container")?.scrollTo({ top: 0 });
  }, [location.pathname]);

  const theme = useTheme();
  const isCompactViewport = useMediaQuery(theme.breakpoints.down("md"));
  const isMidSizeTouchViewport = useIsMidSizeTouchViewport();
  const { state: shellState, actions: shellActions } = useAppShell({
    initialCollapsed:
      getSidebarCollapsed() || isCompactViewport || isMidSizeTouchViewport,
  });

  const { isVisible } = useLoader();
  const isLoginCallback =
    new URLSearchParams(location.search).has("code") &&
    new URLSearchParams(location.search).has("state");

  // Latch to true once auth finishes loading the first time, and stay true
  // (so a later token-refresh that briefly flips isAuthLoading won't re-show
  // the loading screen).
  const [hasInitialized, setHasInitialized] = useState(false);
  const [loadingMessage, setLoadingMessage] = useState<string>(
    isLoginCallback ? "Authenticating…" : "Loading…",
  );

  useEffect(() => {
    if (!isAuthLoading) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- one-time init latch when auth finishes
      setHasInitialized(true);
    }
  }, [isAuthLoading]);

  useEffect(() => {
    if (!isAuthLoading || !isLoginCallback) return;
    const t1 = setTimeout(() => setLoadingMessage("Fetching user info…"), 1500);
    const t2 = setTimeout(() => setLoadingMessage("Please wait…"), 3000);
    return () => {
      clearTimeout(t1);
      clearTimeout(t2);
    };
  }, [isAuthLoading, isLoginCallback]);

  useEffect(() => {
    setSidebarCollapsed(shellState.sidebarCollapsed);
  }, [shellState.sidebarCollapsed]);

  const wasCompactViewport = useRef(isCompactViewport);
  useEffect(() => {
    if (
      isCompactViewport &&
      !wasCompactViewport.current &&
      !shellState.sidebarCollapsed
    ) {
      shellActions.toggleSidebar();
    }
    wasCompactViewport.current = isCompactViewport;
  }, [isCompactViewport, shellState.sidebarCollapsed, shellActions]);

  const isSidebarOverlay = isMidSizeTouchViewport;
  const isSidebarOpen = isSidebarOverlay && !shellState.sidebarCollapsed;

  const previousIsSidebarOverlay = useRef(isSidebarOverlay);
  useEffect(() => {
    if (
      !previousIsSidebarOverlay.current &&
      isSidebarOverlay &&
      !shellState.sidebarCollapsed
    ) {
      shellActions.toggleSidebar();
    }
    previousIsSidebarOverlay.current = isSidebarOverlay;
  }, [isSidebarOverlay, shellState.sidebarCollapsed, shellActions]);

  const previousPathname = useRef(location.pathname);
  useEffect(() => {
    if (
      isSidebarOverlay &&
      previousPathname.current !== location.pathname &&
      !shellState.sidebarCollapsed
    ) {
      shellActions.toggleSidebar();
    }
    previousPathname.current = location.pathname;
  }, [location.pathname, isSidebarOverlay, shellState.sidebarCollapsed, shellActions]);

  const handleSidebarClose = (): void => {
    if (!shellState.sidebarCollapsed) {
      shellActions.toggleSidebar();
    }
  };

  return (
    <IdleTimeoutProvider>
      <Box
        sx={{
          display: "flex",
          flexDirection: "column",
          height: "100dvh",
          overflow: "hidden",
        }}
      >
        <AppShellLayout
          header={
            <Header
              onToggleSidebar={shellActions.toggleSidebar}
              collapsed={shellState.sidebarCollapsed}
            />
          }
          sidebar={
            <SideBar
              collapsed={isSidebarOverlay ? false : shellState.sidebarCollapsed}
              expandedMenus={shellState.expandedMenus}
              onSelect={shellActions.setActiveMenuItem}
              onToggleExpand={shellActions.toggleMenu}
            />
          }
          sidebarOverlay={isSidebarOverlay}
          sidebarOpen={isSidebarOpen}
          onSidebarClose={handleSidebarClose}
          footer={<Footer />}
        >
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              flex: 1,
              minHeight: 0,
              minWidth: 0,
              width: "100%",
              maxWidth: "100%",
              position: "relative",
              boxSizing: "border-box",
            }}
          >
            {isVisible && (
              <LinearProgress
                color="warning"
                sx={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  right: 0,
                  zIndex: 1300,
                  height: 3,
                }}
              />
            )}
            <Box
              sx={{
                flex: 1,
                minHeight: 0,
                minWidth: 0,
                width: "100%",
                maxWidth: "100%",
                display: "flex",
                flexDirection: "column",
                boxSizing: "border-box",
                px: { xs: 1.5, sm: 2, md: 3 },
                py: { xs: 2, sm: 2.5, md: 3 },
              }}
            >
              {!hasInitialized ? (
                <Box
                  sx={{
                    flex: 1,
                    display: "flex",
                    flexDirection: "column",
                    alignItems: "center",
                    justifyContent: "center",
                    gap: 2,
                  }}
                >
                  <LinearProgress
                    color="warning"
                    sx={{ width: "80%", maxWidth: 400, height: 4 }}
                  />
                  <Typography variant="body2" color="text.secondary">
                    {loadingMessage}
                  </Typography>
                </Box>
              ) : (
                <Box
                  sx={{
                    width: "100%",
                    maxWidth: "100%",
                    minWidth: 0,
                    flex: 1,
                    boxSizing: "border-box",
                  }}
                >
                  {children || (
                    <Outlet
                      context={{
                        sidebarCollapsed: shellState.sidebarCollapsed,
                      }}
                    />
                  )}
                </Box>
              )}
            </Box>
          </Box>
        </AppShellLayout>
      </Box>
    </IdleTimeoutProvider>
  );
}
