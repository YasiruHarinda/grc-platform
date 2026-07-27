import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import Box from "@mui/material/Box";
import Tooltip from "@mui/material/Tooltip";
import Divider from "@mui/material/Divider";
import Avatar from "@mui/material/Avatar";
import { BarsIcon, SunIcon, CrescentBrightIcon } from "@oxygen-ui/react-icons";
import { useNavigate } from "react-router-dom";
import { useCurrentUser } from "../hooks/useCurrentUser";
import { useColorMode } from "../main";

interface NavbarProps {
  onToggleSidebar: () => void;
}

export default function Navbar({ onToggleSidebar }: NavbarProps) {
  const navigate = useNavigate();
  const { user, isLoaded } = useCurrentUser();
  const { mode, toggleColorMode } = useColorMode();

  const isDark = mode === "dark";
  const userInitial = (user?.email ?? "U").charAt(0).toUpperCase();

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
          <Tooltip title={`${user.email ?? ""} · ${user.role}`} arrow>
            <Avatar
              sx={{
                width: 34,
                height: 34,
                fontSize: "0.85rem",
                fontWeight: 700,
                bgcolor: "primary.main",
                color: "#fff",
                cursor: "default",
              }}
            >
              {userInitial}
            </Avatar>
          </Tooltip>
        )}
      </Toolbar>
    </AppBar>
  );
}
