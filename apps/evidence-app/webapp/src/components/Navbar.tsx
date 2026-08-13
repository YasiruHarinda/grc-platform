import { useState, type MouseEvent } from "react";
import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import Box from "@mui/material/Box";
import Tooltip from "@mui/material/Tooltip";
import Divider from "@mui/material/Divider";
import Avatar from "@mui/material/Avatar";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import Snackbar from "@mui/material/Snackbar";
import Alert from "@mui/material/Alert";
import { BarsIcon, SunIcon, CrescentBrightIcon, ArrowRightFromBracketIcon } from "@oxygen-ui/react-icons";
import { useNavigate } from "react-router-dom";
import { useAuthContext } from "@asgardeo/auth-react";
import { useQueryClient } from "@tanstack/react-query";
import { useCurrentUser } from "../hooks/useCurrentUser";
import { useColorMode } from "../main";
import { clearFileUrlCache } from "../utils/stableFileUrl";

interface NavbarProps {
  onToggleSidebar: () => void;
}

export default function Navbar({ onToggleSidebar }: NavbarProps) {
  const navigate = useNavigate();
  const { user, isLoaded } = useCurrentUser();
  const { mode, toggleColorMode } = useColorMode();
  const { signOut } = useAuthContext();
  const queryClient = useQueryClient();
  const [accountMenuAnchor, setAccountMenuAnchor] = useState<HTMLElement | null>(null);
  const [signOutError, setSignOutError] = useState<string | null>(null);

  const isDark = mode === "dark";
  const userInitial = (user?.email ?? "U").charAt(0).toUpperCase();
  const accountMenuOpen = Boolean(accountMenuAnchor);

  const handleAccountMenuOpen = (event: MouseEvent<HTMLElement>) => {
    setAccountMenuAnchor(event.currentTarget);
  };

  const handleAccountMenuClose = () => {
    setAccountMenuAnchor(null);
  };

  const handleSignOut = () => {
    handleAccountMenuClose();
    // Drop every cached query (evidence, submissions, dashboard, cost, ...)
    // so the next person to sign in on this machine never sees a flash of
    // the previous user's data before the first refetch replaces it.
    queryClient.clear();
    clearFileUrlCache();
    // If signing out fails there is nothing to see: the redirect never
    // happens, the cleared queries refetch against the still-live session,
    // and the app looks exactly as it did. Someone on a shared machine would
    // walk away from an open session believing they had left it. Say so
    // instead, so they know to close the browser.
    signOut().catch((err) => {
      console.error("Sign-out failed:", err);
      setSignOutError(
        "Sign-out failed — you are still signed in. Close the browser before leaving this machine.",
      );
    });
  };

  return (
    <AppBar
      position="static"
      color="default"
      elevation={0}
      sx={{
        backgroundColor: "background.paper",
        borderBottom: "1px solid",
        borderColor: "divider",
      }}
    >
      <Toolbar sx={{ minHeight: { xs: 56, sm: 64 }, px: { xs: 1, sm: 1.5 } }}>

        {/* Sidebar toggle */}
        <Tooltip title="Toggle sidebar">
          <IconButton
            onClick={onToggleSidebar}
            size="small"
            sx={{ mr: 1, color: "text.secondary" }}
            aria-label="Toggle sidebar"
          >
            <BarsIcon size={20} />
          </IconButton>
        </Tooltip>

        {/* Brand */}
        <Box
          sx={{ display: "flex", alignItems: "center", gap: 1.25, cursor: "pointer", flexShrink: 0 }}
          onClick={() => navigate("/")}
        >
          <Box
            component="img"
            src={isDark ? "/logo-white.svg" : "/logo-dark.svg"}
            alt="WSO2"
            sx={{ height: 20, width: "auto", display: "block" }}
          />
          <Divider orientation="vertical" flexItem sx={{ mx: 0.25, my: 1.5 }} />
          <Typography
            variant="subtitle1"
            fontWeight={600}
            sx={{ color: "text.primary", letterSpacing: "-0.01em", whiteSpace: "nowrap" }}
          >
            Evidence Portal
          </Typography>
        </Box>

        <Box sx={{ flex: 1 }} />

        {/* Dark / light mode toggle */}
        <Tooltip title={isDark ? "Switch to light mode" : "Switch to dark mode"}>
          <IconButton
            onClick={toggleColorMode}
            size="small"
            sx={{ mr: 1, color: "text.secondary" }}
            aria-label="Toggle color mode"
          >
            {isDark ? <SunIcon size={20} /> : <CrescentBrightIcon size={20} />}
          </IconButton>
        </Tooltip>

        {/* User avatar */}
        {isLoaded && user && (
          <>
            <Tooltip title={`${user.email ?? ""} · ${user.role}`} arrow>
              <IconButton
                onClick={handleAccountMenuOpen}
                size="small"
                sx={{ p: 0 }}
                aria-label="Account menu"
                aria-controls={accountMenuOpen ? "account-menu" : undefined}
                aria-haspopup="true"
                aria-expanded={accountMenuOpen ? "true" : undefined}
              >
                <Avatar
                  sx={{
                    width: 34,
                    height: 34,
                    fontSize: "0.85rem",
                    fontWeight: 700,
                    bgcolor: "primary.main",
                    color: "#fff",
                  }}
                >
                  {userInitial}
                </Avatar>
              </IconButton>
            </Tooltip>
            <Menu
              id="account-menu"
              anchorEl={accountMenuAnchor}
              open={accountMenuOpen}
              onClose={handleAccountMenuClose}
              anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
              transformOrigin={{ vertical: "top", horizontal: "right" }}
            >
              <MenuItem disabled sx={{ opacity: "1 !important" }}>
                <ListItemText
                  primary={user.email}
                  secondary={user.role}
                  primaryTypographyProps={{ noWrap: true, fontWeight: 600 }}
                />
              </MenuItem>
              <Divider />
              <MenuItem onClick={handleSignOut} aria-label="Sign out">
                <ListItemIcon>
                  <ArrowRightFromBracketIcon size={18} />
                </ListItemIcon>
                <ListItemText>Sign out</ListItemText>
              </MenuItem>
            </Menu>
          </>
        )}
      </Toolbar>

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
    </AppBar>
  );
}
