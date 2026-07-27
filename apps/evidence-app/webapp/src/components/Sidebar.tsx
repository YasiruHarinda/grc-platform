import Box from "@mui/material/Box";
import Drawer from "@mui/material/Drawer";
import List from "@mui/material/List";
import ListItemButton from "@mui/material/ListItemButton";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import Tooltip from "@mui/material/Tooltip";
import Divider from "@mui/material/Divider";
import {
  HomeIcon,
  DocumentIcon,
  ArrowUpIcon,
  BoltIcon,
  ReceiptIcon,
} from "@oxygen-ui/react-icons";
import { useNavigate, useLocation } from "react-router-dom";
import { useCurrentUser } from "../hooks/useCurrentUser";

export const SIDEBAR_WIDTH = 220;
export const SIDEBAR_COLLAPSED_WIDTH = 60;

const allNavItems = [
  { label: "Dashboard", to: "/",        icon: HomeIcon,     adminOnly: false },
  { label: "Evidence",  to: "/evidence", icon: DocumentIcon, adminOnly: false },
  { label: "Submit",    to: "/submit",   icon: ArrowUpIcon,  adminOnly: false },
  { label: "Agent",     to: "/agent",    icon: BoltIcon,     adminOnly: false },
  { label: "Cost",      to: "/cost",     icon: ReceiptIcon,  adminOnly: true  },
];

interface SidebarContentProps {
  collapsed: boolean;
}

function SidebarContent({ collapsed }: SidebarContentProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const { isAdmin } = useCurrentUser();

  const navItems = allNavItems.filter((item) => !item.adminOnly || isAdmin);

  return (
    <Box
      sx={{
        width: collapsed ? SIDEBAR_COLLAPSED_WIDTH : SIDEBAR_WIDTH,
        transition: "width 200ms ease",
        overflow: "hidden",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        borderRight: "1px solid",
        borderColor: "divider",
        bgcolor: "background.paper",
      }}
    >
      <List disablePadding sx={{ pt: 1, flex: 1 }}>
        {navItems.map(({ label, to, icon: Icon }) => {
          const active = to === "/" ? location.pathname === "/" : location.pathname.startsWith(to);
          const button = (
            <ListItemButton
              key={to}
              onClick={() => navigate(to)}
              selected={active}
              sx={{
                mx: 1,
                mb: 0.5,
                borderRadius: 2,
                minHeight: 44,
                px: collapsed ? 1.5 : 1.75,
                justifyContent: collapsed ? "center" : "flex-start",
                color: active ? "primary.main" : "text.secondary",
                bgcolor: active ? "rgba(250,123,63,0.08) !important" : "transparent",
                "&:hover": { bgcolor: "action.hover", color: "text.primary" },
                "& .MuiListItemIcon-root": {
                  color: active ? "primary.main" : "text.secondary",
                  minWidth: collapsed ? 0 : 36,
                  justifyContent: "center",
                },
              }}
            >
              <ListItemIcon>
                <Icon size={20} />
              </ListItemIcon>
              {!collapsed && (
                <ListItemText
                  primary={label}
                  primaryTypographyProps={{
                    fontSize: "0.875rem",
                    fontWeight: active ? 600 : 400,
                    noWrap: true,
                  }}
                />
              )}
            </ListItemButton>
          );

          return collapsed ? (
            <Tooltip key={to} title={label} placement="right" arrow>
              <span>{button}</span>
            </Tooltip>
          ) : (
            <span key={to}>{button}</span>
          );
        })}
      </List>
      <Divider />
    </Box>
  );
}

interface SidebarProps {
  collapsed: boolean;
  mobileOpen: boolean;
  onMobileClose: () => void;
}

export default function Sidebar({ collapsed, mobileOpen, onMobileClose }: SidebarProps) {
  return (
    <>
      {/* Desktop permanent sidebar */}
      <Box
        component="nav"
        sx={{
          display: { xs: "none", md: "flex" },
          flexShrink: 0,
          width: collapsed ? SIDEBAR_COLLAPSED_WIDTH : SIDEBAR_WIDTH,
          transition: "width 200ms ease",
        }}
      >
        <SidebarContent collapsed={collapsed} />
      </Box>

      {/* Mobile temporary drawer */}
      <Drawer
        variant="temporary"
        anchor="left"
        open={mobileOpen}
        onClose={onMobileClose}
        ModalProps={{ keepMounted: true }}
        PaperProps={{ sx: { width: SIDEBAR_WIDTH, border: "none" } }}
        sx={{ display: { xs: "block", md: "none" } }}
      >
        <SidebarContent collapsed={false} />
      </Drawer>
    </>
  );
}
