import { StrictMode, createContext, useContext, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import CssBaseline from "@mui/material/CssBaseline";
import GlobalStyles from "@mui/material/GlobalStyles";
import { AuthProvider } from "@asgardeo/auth-react";
import App from "./App.tsx";

const asgardeoClientID = import.meta.env.VITE_ASGARDEO_CLIENT_ID;
const asgardeoBaseUrl = import.meta.env.VITE_ASGARDEO_BASE_URL;

if (!asgardeoClientID || !asgardeoBaseUrl) {
  const missing = [
    !asgardeoClientID && "VITE_ASGARDEO_CLIENT_ID",
    !asgardeoBaseUrl && "VITE_ASGARDEO_BASE_URL",
  ]
    .filter(Boolean)
    .join(", ");
  throw new Error(`Missing required environment variable(s): ${missing}`);
}

const authConfig = {
  clientID: asgardeoClientID,
  baseUrl: asgardeoBaseUrl,
  signInRedirectURL: window.location.origin,
  signOutRedirectURL: window.location.origin,
  scope: ["openid", "profile", "email", "groups"],
};

type ColorMode = "light" | "dark";

interface ColorModeContextType {
  mode: ColorMode;
  toggleColorMode: () => void;
}

export const ColorModeContext = createContext<ColorModeContextType>({
  mode: "light",
  toggleColorMode: () => {},
});

export const useColorMode = () => useContext(ColorModeContext);

function buildTheme(mode: ColorMode) {
  const isDark = mode === "dark";
  return createTheme({
    palette: {
      mode,
      primary: {
        main: isDark ? "#F87643" : "#fa7b3f",
        dark: "#d45a1e",
        light: "#fb9463",
        contrastText: "#ffffff",
      },
      secondary: { main: isDark ? "#3c3c3c" : "#e8e8e8" },
      background: {
        default: isDark ? "#0d0d14" : "#f5f5f5",
        paper:   isDark ? "#1a1a24" : "#ffffff",
      },
      text: {
        primary:   isDark ? "#efefef"  : "#40404B",
        secondary: isDark ? "#D0D3E2"  : "#6b6b7b",
      },
      divider: isDark ? "rgba(255,255,255,0.09)" : "rgba(0,0,0,0.07)",
    },
    typography: {
      fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'Inter', sans-serif",
      h4: { fontWeight: 700, letterSpacing: "-0.02em" },
      h5: { fontWeight: 700, letterSpacing: "-0.01em" },
      h6: { fontWeight: 600 },
      button: { textTransform: "none", fontWeight: 600 },
      subtitle2: { fontWeight: 600 },
    },
    shape: { borderRadius: 12 },
    components: {
      MuiStack: {
        defaultProps: { useFlexGap: true },
      },
      MuiPaper: {
        defaultProps: { elevation: 0 },
        styleOverrides: {
          outlined: {
            borderColor: isDark ? "rgba(255,255,255,0.09)" : "#E5E7EB",
          },
        },
      },
      MuiCard: {
        defaultProps: { elevation: 0 },
        styleOverrides: {
          root: {
            border: `1px solid ${isDark ? "rgba(255,255,255,0.09)" : "#E5E7EB"}`,
            transition: "box-shadow 200ms ease, border-color 200ms ease",
            "&:hover": {
              boxShadow: isDark
                ? "0 4px 14px rgba(0,0,0,0.4)"
                : "0 4px 14px rgba(0,0,0,0.06)",
              borderColor: isDark ? "rgba(255,255,255,0.18)" : "#D1D5DB",
            },
          },
        },
      },
      MuiButton: {
        styleOverrides: {
          root: { borderRadius: 8, paddingInline: 16 },
          containedPrimary: {
            boxShadow: "none",
            "&:hover": { boxShadow: "0 4px 12px rgba(255,115,0,0.25)" },
          },
        },
      },
      MuiTableHead: {
        styleOverrides: {
          root: {
            backgroundColor: isDark ? "rgba(255,255,255,0.04)" : "#F9FAFB",
            "& .MuiTableCell-root": {
              fontWeight: 600,
              fontSize: "0.78rem",
              textTransform: "uppercase",
              letterSpacing: "0.04em",
            },
          },
        },
      },
      MuiChip: {
        styleOverrides: {
          root: { fontWeight: 600, textTransform: "capitalize" },
        },
      },
      MuiAppBar: {
        styleOverrides: {
          root: { boxShadow: "none" },
        },
      },
    },
  });
}

const STORAGE_KEY = "ep-color-mode";

function getInitialMode(): ColorMode {
  const saved = localStorage.getItem(STORAGE_KEY) as ColorMode | null;
  if (saved === "light" || saved === "dark") return saved;
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

const queryClient = new QueryClient();

function ThemedApp() {
  const [mode, setMode] = useState<ColorMode>(getInitialMode);

  const colorMode = useMemo<ColorModeContextType>(
    () => ({
      mode,
      toggleColorMode: () => {
        setMode((prev) => {
          const next = prev === "light" ? "dark" : "light";
          localStorage.setItem(STORAGE_KEY, next);
          return next;
        });
      },
    }),
    [mode]
  );

  const theme = useMemo(() => buildTheme(mode), [mode]);

  return (
    <ColorModeContext.Provider value={colorMode}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <GlobalStyles styles={{ "html, body, #root": { height: "100%", margin: 0, padding: 0 } }} />
        <App />
      </ThemeProvider>
    </ColorModeContext.Provider>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <AuthProvider config={authConfig}>
      <QueryClientProvider client={queryClient}>
        <ThemedApp />
      </QueryClientProvider>
    </AuthProvider>
  </StrictMode>
);
