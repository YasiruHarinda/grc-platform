import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import MuiLink from "@mui/material/Link";
import Divider from "@mui/material/Divider";

const CURRENT_YEAR = new Date().getFullYear();

export default function Footer() {
  return (
    <>
      <Divider />
      <Box
        component="footer"
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          flexWrap: "wrap",
          gap: 1,
          px: { xs: 2, sm: 3 },
          py: 1.5,
          bgcolor: "background.paper",
        }}
      >
        <Typography variant="caption" color="text.secondary">
          © {CURRENT_YEAR} WSO2 LLC. All rights reserved.
        </Typography>
        <Box sx={{ display: "flex", gap: 2 }}>
          <MuiLink
            href="https://wso2.com/terms-of-use/"
            target="_blank"
            rel="noopener noreferrer"
            variant="caption"
            color="text.secondary"
            underline="hover"
          >
            Terms &amp; Conditions
          </MuiLink>
          <MuiLink
            href="https://wso2.com/privacy-policy/"
            target="_blank"
            rel="noopener noreferrer"
            variant="caption"
            color="text.secondary"
            underline="hover"
          >
            Privacy Policy
          </MuiLink>
        </Box>
      </Box>
    </>
  );
}
