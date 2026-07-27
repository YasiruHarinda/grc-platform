import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link as RouterLink } from "react-router-dom";
import Box from "@mui/material/Box";
import Grid from "@mui/material/Grid";
import Card from "@mui/material/Card";
import CardContent from "@mui/material/CardContent";
import Typography from "@mui/material/Typography";
import Paper from "@mui/material/Paper";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Chip from "@mui/material/Chip";
import Stack from "@mui/material/Stack";
import FormControl from "@mui/material/FormControl";
import InputLabel from "@mui/material/InputLabel";
import Select from "@mui/material/Select";
import MenuItem from "@mui/material/MenuItem";
import Tooltip from "@mui/material/Tooltip";
import Link from "@mui/material/Link";
import Alert from "@mui/material/Alert";
import Button from "@mui/material/Button";
import { DocumentIcon, EnvelopeIcon, HierarchyIcon, ClockAsteriskIcon, BoltIcon, CircleUserIcon } from "@oxygen-ui/react-icons";
import { evidenceApi, submissionsApi, frameworksApi, productsApi, controlsApi } from "../api/client";

type Product = { id: number; name: string };
type Framework = { id: number; name: string; product_id: number };
type Control = { id: number; framework_id: number };
type Evidence = { id: number; control_id: number; title?: string; description?: string };
type Submission = { id: number; evidence_id: number; submitted_by: string; status: string; submitted_at: string };

const statusColor = (status: string): "warning" | "success" | "error" | "default" => {
  if (status === "pending") return "warning";
  if (status === "approved") return "success";
  if (status === "rejected") return "error";
  return "default";
};

function relativeTime(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime();
  if (ms < 0) return "Just now";
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return "Just now";
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min} min ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr} hr${hr === 1 ? "" : "s"} ago`;
  const day = Math.floor(hr / 24);
  if (day < 7) return `${day} day${day === 1 ? "" : "s"} ago`;
  return new Date(iso).toLocaleDateString();
}

export default function Dashboard() {
  const [productId, setProductId] = useState<number | "">("");

  const {
    data: products = [],
    isError: productsError,
    refetch: refetchProducts,
  } = useQuery<Product[]>({ queryKey: ["products"], queryFn: productsApi.list });
  const {
    data: allFrameworks = [],
    isError: frameworksError,
    refetch: refetchFrameworks,
  } = useQuery<Framework[]>({ queryKey: ["frameworks"], queryFn: () => frameworksApi.list() });
  const {
    data: allControls = [],
    isError: controlsError,
    refetch: refetchControls,
  } = useQuery<Control[]>({ queryKey: ["controls"], queryFn: () => controlsApi.list() });
  const {
    data: allEvidence = [],
    isError: evidenceError,
    refetch: refetchEvidence,
  } = useQuery<Evidence[]>({ queryKey: ["evidence"], queryFn: evidenceApi.list });
  const {
    data: allSubmissions = [],
    isError: submissionsError,
    refetch: refetchSubmissions,
  } = useQuery<Submission[]>({ queryKey: ["submissions"], queryFn: submissionsApi.list });

  // Any of these failing means the stats/table below would render misleading
  // zero-valued data, so surface an explicit error state instead.
  const isError = productsError || frameworksError || controlsError || evidenceError || submissionsError;
  const handleRetry = () => {
    refetchProducts();
    refetchFrameworks();
    refetchControls();
    refetchEvidence();
    refetchSubmissions();
  };

  const { frameworks, evidence, submissions } = useMemo(() => {
    if (productId === "") {
      return { frameworks: allFrameworks, evidence: allEvidence, submissions: allSubmissions };
    }
    const pid = Number(productId);
    const fws = allFrameworks.filter((f) => f.product_id === pid);
    const fwIds = new Set(fws.map((f) => f.id));
    const ctrlIds = new Set(allControls.filter((c) => fwIds.has(c.framework_id)).map((c) => c.id));
    const evs = allEvidence.filter((e) => ctrlIds.has(e.control_id));
    const evIds = new Set(evs.map((e) => e.id));
    const subs = allSubmissions.filter((s) => evIds.has(s.evidence_id));
    return { frameworks: fws, evidence: evs, submissions: subs };
  }, [productId, allFrameworks, allControls, allEvidence, allSubmissions]);

  const evidenceById = useMemo(() => {
    const m = new Map<number, Evidence>();
    for (const e of allEvidence) m.set(e.id, e);
    return m;
  }, [allEvidence]);

  const recentSubmissions = useMemo(
    () =>
      [...submissions]
        .sort((a, b) => new Date(b.submitted_at).getTime() - new Date(a.submitted_at).getTime())
        .slice(0, 6),
    [submissions]
  );

  const stats = [
    {
      label: "Total Evidence",
      value: evidence.length,
      icon: <DocumentIcon size={26} />,
      bg: "rgba(255,115,0,0.08)",
      iconColor: "#FF7300",
    },
    {
      label: "Total Submissions",
      value: submissions.length,
      icon: <EnvelopeIcon size={26} />,
      bg: "rgba(46,125,250,0.08)",
      iconColor: "#2E7DFA",
    },
    {
      label: "Frameworks",
      value: frameworks.length,
      icon: <HierarchyIcon size={26} />,
      bg: "rgba(34,197,94,0.08)",
      iconColor: "#22C55E",
    },
    {
      label: "Pending Reviews",
      value: submissions.filter((s) => s.status === "pending").length,
      icon: <ClockAsteriskIcon size={26} />,
      bg: "rgba(234,179,8,0.10)",
      iconColor: "#EAB308",
    },
  ];

  return (
    <Box>
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1, flexWrap: "wrap", gap: 2 }}>
        <Box>
          <Typography variant="h4" gutterBottom>
            Dashboard
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Overview of evidence, submissions, and pending reviews
            {productId !== "" ? ` for "${products.find((p) => p.id === Number(productId))?.name ?? "selected product"}"` : " across all products"}.
          </Typography>
        </Box>
        <FormControl size="small" sx={{ minWidth: 220 }}>
          <InputLabel>Product</InputLabel>
          <Select
            label="Product"
            value={productId}
            onChange={(e) => setProductId(e.target.value as number | "")}
          >
            <MenuItem value="">All Products</MenuItem>
            {products.map((p) => (
              <MenuItem key={p.id} value={p.id}>{p.name}</MenuItem>
            ))}
          </Select>
        </FormControl>
      </Stack>

      {isError ? (
        <Alert
          severity="error"
          sx={{ mt: 2, mb: 5 }}
          action={
            <Button color="inherit" size="small" onClick={handleRetry}>
              Retry
            </Button>
          }
        >
          Couldn't load dashboard data.
        </Alert>
      ) : (
        <>
        <Grid container spacing={2.5} sx={{ mt: 2, mb: 5 }}>
          {stats.map(({ label, value, icon, bg, iconColor }) => (
            <Grid item xs={12} sm={6} md={3} key={label}>
              <Card>
                <CardContent sx={{ py: 2.5 }}>
                  <Stack direction="row" alignItems="center" spacing={2}>
                    <Box
                      sx={{
                        width: 48,
                        height: 48,
                        borderRadius: 2,
                        backgroundColor: bg,
                        color: iconColor,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                      }}
                    >
                      {icon}
                    </Box>
                    <Box>
                      <Typography variant="h4" fontWeight={700} sx={{ lineHeight: 1.1 }}>
                        {value}
                      </Typography>
                      <Typography variant="body2" color="text.secondary" mt={0.25}>
                        {label}
                      </Typography>
                    </Box>
                  </Stack>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
  
        <Stack direction="row" alignItems="baseline" justifyContent="space-between" sx={{ mb: 1.5 }}>
          <Typography variant="h6">Recent Submissions</Typography>
          <Link component={RouterLink} to="/evidence" underline="hover" sx={{ fontSize: "0.875rem", fontWeight: 500 }}>
            View all →
          </Link>
        </Stack>
  
        {/* Desktop table — hidden on mobile */}
        <Box sx={{ display: { xs: "none", md: "block" } }}>
          <TableContainer component={Paper} variant="outlined">
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell sx={{ fontWeight: 600 }}>Evidence</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Submitted By</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Status</TableCell>
                  <TableCell sx={{ fontWeight: 600 }} align="right">When</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {recentSubmissions.map((s) => {
                  const ev = evidenceById.get(s.evidence_id);
                  const evidenceLabel =
                    ev?.title?.trim() ||
                    ev?.description?.trim() ||
                    `Evidence #${s.evidence_id}`;
                  return (
                    <TableRow key={s.id} hover>
                      <TableCell sx={{ maxWidth: 360 }}>
                        <Typography
                          variant="body2"
                          sx={{
                            display: "-webkit-box",
                            WebkitLineClamp: 2,
                            WebkitBoxOrient: "vertical",
                            overflow: "hidden",
                            lineHeight: 1.4,
                          }}
                        >
                          {evidenceLabel.replace(/^AI Agent:\s*/, "")}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Chip
                          icon={s.submitted_by === "ai-agent" ? <BoltIcon size={12} /> : <CircleUserIcon size={12} />}
                          label={s.submitted_by === "ai-agent" ? "AI Agent" : "Manual"}
                          size="small"
                          color={s.submitted_by === "ai-agent" ? "primary" : "default"}
                          variant={s.submitted_by === "ai-agent" ? "filled" : "outlined"}
                          sx={{ fontWeight: 600, fontSize: "0.72rem" }}
                        />
                      </TableCell>
                      <TableCell>
                        <Chip label={s.status} color={statusColor(s.status)} size="small" />
                      </TableCell>
                      <TableCell align="right" sx={{ color: "text.secondary", whiteSpace: "nowrap" }}>
                        <Tooltip title={new Date(s.submitted_at).toLocaleString()} arrow>
                          <span>{relativeTime(s.submitted_at)}</span>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  );
                })}
                {recentSubmissions.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={4} align="center" sx={{ color: "text.disabled", py: 5 }}>
                      No submissions yet{productId !== "" ? " for this product" : ""}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </Box>
  
        {/* Mobile cards — hidden on desktop */}
        <Stack spacing={1.5} sx={{ display: { xs: "flex", md: "none" } }}>
          {recentSubmissions.length === 0 && (
            <Paper variant="outlined" sx={{ p: 3, textAlign: "center" }}>
              <Typography color="text.disabled">No submissions yet{productId !== "" ? " for this product" : ""}</Typography>
            </Paper>
          )}
          {recentSubmissions.map((s) => {
            const ev = evidenceById.get(s.evidence_id);
            const evidenceLabel = (ev?.title?.trim() || ev?.description?.trim() || `Evidence #${s.evidence_id}`).replace(/^AI Agent:\s*/, "");
            return (
              <Paper key={s.id} variant="outlined" sx={{ p: 2 }}>
                <Stack spacing={0.75}>
                  <Stack direction="row" justifyContent="space-between" alignItems="flex-start">
                    <Typography variant="body2" fontWeight={500} sx={{ flex: 1, pr: 1, lineHeight: 1.4 }}>
                      {evidenceLabel}
                    </Typography>
                    <Chip label={s.status} color={statusColor(s.status)} size="small" sx={{ flexShrink: 0 }} />
                  </Stack>
                  <Stack direction="row" justifyContent="space-between" alignItems="center">
                    <Chip
                      icon={s.submitted_by === "ai-agent" ? <BoltIcon size={11} /> : <CircleUserIcon size={11} />}
                      label={s.submitted_by === "ai-agent" ? "AI Agent" : "Manual"}
                      size="small"
                      color={s.submitted_by === "ai-agent" ? "primary" : "default"}
                      variant={s.submitted_by === "ai-agent" ? "filled" : "outlined"}
                      sx={{ fontWeight: 600, fontSize: "0.68rem", height: 20 }}
                    />
                    <Tooltip title={new Date(s.submitted_at).toLocaleString()} arrow>
                      <Typography variant="caption" color="text.secondary">{relativeTime(s.submitted_at)}</Typography>
                    </Tooltip>
                  </Stack>
                </Stack>
              </Paper>
            );
          })}
        </Stack>
        </>
      )}
    </Box>
  );
}
