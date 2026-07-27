import { useEffect, useState } from "react";
import { useAuthContext } from "@asgardeo/auth-react";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Typography from "@mui/material/Typography";
import useMediaQuery from "@mui/material/useMediaQuery";
import { useTheme } from "@mui/material/styles";
import Navbar from "./components/Navbar";
import Sidebar from "./components/Sidebar";
import Footer from "./components/Footer";
import Dashboard from "./pages/Dashboard";
import EvidenceList from "./pages/EvidenceList";
import SubmitEvidence from "./pages/SubmitEvidence";
import AgentRunner from "./pages/AgentRunner";
import Cost from "./pages/Cost";
import { registerAuth } from "./api/client";

function AppRoutes() {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);

  const handleToggle = () => {
    if (isMobile) {
      setMobileOpen((prev) => !prev);
    } else {
      setSidebarCollapsed((prev) => !prev);
    }
  };

  return (
    <BrowserRouter>
      <Box sx={{ display: "flex", flexDirection: "column", height: "100%", overflow: "hidden" }}>
        <Navbar onToggleSidebar={handleToggle} />
        <Box sx={{ display: "flex", flex: 1, minHeight: 0 }}>
          <Sidebar
            collapsed={sidebarCollapsed}
            mobileOpen={mobileOpen}
            onMobileClose={() => setMobileOpen(false)}
          />
          {/* Right column: scrollable content + footer */}
          <Box sx={{ display: "flex", flexDirection: "column", flex: 1, minWidth: 0, overflow: "hidden" }}>
            <Box
              component="main"
              sx={{ flex: 1, overflowY: "auto", overflowX: "hidden", px: { xs: 2, sm: 3 }, py: { xs: 3, sm: 4 } }}
            >
              <Routes>
                <Route path="/" element={<Dashboard />} />
                <Route path="/evidence" element={<EvidenceList />} />
                <Route path="/submit" element={<SubmitEvidence />} />
                <Route path="/history" element={<Navigate to="/evidence" replace />} />
                <Route path="/agent" element={<AgentRunner />} />
                <Route path="/cost" element={<Cost />} />
              </Routes>
            </Box>
            <Footer />
          </Box>
        </Box>
      </Box>
    </BrowserRouter>
  );
}

export default function App() {
  const { state, signIn, getAccessToken, refreshAccessToken } = useAuthContext();
  const [tokenReady, setTokenReady] = useState(false);

  // Bridge the Asgardeo SDK's token accessors into the axios/SSE layer so every
  // request gets a *fresh* token and a 401 triggers a silent refresh + retry.
  // (Previously the token was fetched once and cached, so ~1h after login it
  // expired and every call started failing until a full page reload / re-login.)
  useEffect(() => {
    if (!state.isAuthenticated) {
      registerAuth(null);
      setTokenReady(false);
      return;
    }
    registerAuth({
      getAccessToken,
      refreshAccessToken,
      onAuthLost: () => signIn(),
    });
    // Warm-up: make sure the SDK actually holds a token before rendering pages,
    // so the first API calls don't race ahead of authentication.
    let cancelled = false;
    setTokenReady(false);
    getAccessToken()
      .then(() => { if (!cancelled) setTokenReady(true); })
      .catch(() => { if (!cancelled) setTokenReady(true); });
    return () => { cancelled = true; };
  }, [state.isAuthenticated, getAccessToken, refreshAccessToken, signIn]);

  if (state.isLoading || (state.isAuthenticated && !tokenReady)) {
    return (
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", height: "100vh" }}>
        <CircularProgress />
      </Box>
    );
  }

  if (!state.isAuthenticated) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", height: "100vh", gap: 3 }}>
        <Typography variant="h4">Evidence Portal</Typography>
        <Typography color="text.secondary">Sign in with your WSO2 account to continue</Typography>
        <Button variant="contained" size="large" onClick={() => signIn()}>
          Sign In with Asgardeo
        </Button>
      </Box>
    );
  }

  return <AppRoutes />;
}
