import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useColorMode } from "../main";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import Paper from "@mui/material/Paper";
import Stack from "@mui/material/Stack";
import Chip from "@mui/material/Chip";
import CircularProgress from "@mui/material/CircularProgress";
import Alert from "@mui/material/Alert";
import Button from "@mui/material/Button";
import Tooltip from "@mui/material/Tooltip";
import ToggleButton from "@mui/material/ToggleButton";
import ToggleButtonGroup from "@mui/material/ToggleButtonGroup";
import Table from "@mui/material/Table";
import TableHead from "@mui/material/TableHead";
import TableBody from "@mui/material/TableBody";
import TableRow from "@mui/material/TableRow";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import { CircleStarIcon, ClockAsteriskIcon } from "@oxygen-ui/react-icons";
import { usageApi } from "../api/client";

type Summary = {
  total_runs: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_tokens: number;
  total_llm_calls: number;
  total_cost_usd: number;
  last_7_days_cost_usd: number;
  last_7_days_tokens: number;
  last_7_days_runs: number;
  last_30_days_cost_usd: number;
  last_30_days_tokens: number;
  last_30_days_runs: number;
  today_cost_usd: number;
  today_runs: number;
};

type DayPoint = {
  date: string;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  runs: number;
};

type ByModel = {
  model: string;
  runs: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  cost_usd: number;
};

type RecentRun = {
  id: number;
  created_at: string;
  model: string;
  subtask_count: number;
  input_tokens: number;
  output_tokens: number;
  llm_calls: number;
  cost_usd: number;
};

function formatTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + "M";
  if (n >= 1_000) return (n / 1_000).toFixed(1) + "k";
  return String(n);
}
function formatCost(usd: number): string {
  if (usd === 0) return "$0.00";
  if (usd < 0.01) return `$${usd.toFixed(5)}`;
  if (usd < 1) return `$${usd.toFixed(4)}`;
  return `$${usd.toFixed(2)}`;
}
function shortDate(iso: string) {
  const d = new Date(iso);
  return `${d.toLocaleString("en-US", { month: "short" })} ${d.getDate()}`;
}

function StatCard({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent?: string }) {
  return (
    <Paper variant="outlined" sx={{ p: 2.5, flex: 1, minWidth: 180 }}>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ textTransform: "uppercase", letterSpacing: "0.04em", fontWeight: 600 }}
      >
        {label}
      </Typography>
      <Typography variant="h4" fontWeight={700} sx={{ mt: 0.25, color: accent ?? "text.primary", lineHeight: 1.1 }}>
        {value}
      </Typography>
      {sub && (
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5 }}>
          {sub}
        </Typography>
      )}
    </Paper>
  );
}

/** SVG bar chart of daily cost. No external deps. */
function DailyCostChart({ data, mode }: { data: DayPoint[]; mode: "cost" | "tokens" }) {
  const { mode: colorMode } = useColorMode();
  const isDark = colorMode === "dark";

  const axisColor = isDark ? "rgba(255,255,255,0.45)" : "rgba(0,0,0,0.5)";
  const gridColor = isDark ? "rgba(255,255,255,0.1)" : "rgba(0,0,0,0.08)";
  const emptyBarColor = isDark ? "rgba(255,255,255,0.08)" : "rgba(0,0,0,0.06)";

  const W = 720;
  const H = 220;
  const padX = 32;
  const padY = 24;
  const innerW = W - padX * 2;
  const innerH = H - padY * 2;

  const values = data.map((d) => (mode === "cost" ? d.cost_usd : d.input_tokens + d.output_tokens));
  const max = Math.max(0.0001, ...values);

  const barW = data.length > 0 ? innerW / data.length : innerW;
  const yTicks = 4;

  return (
    <Box sx={{ overflowX: "auto" }}>
      <svg width={W} height={H} role="img" aria-label="Daily cost chart" style={{ display: "block" }}>
        {/* y grid */}
        {Array.from({ length: yTicks + 1 }, (_, i) => {
          const v = (max * (yTicks - i)) / yTicks;
          const y = padY + (innerH * i) / yTicks;
          return (
            <g key={i}>
              <line x1={padX} y1={y} x2={padX + innerW} y2={y} stroke={gridColor} strokeWidth="1" />
              <text x={padX - 6} y={y + 3} fontSize="9" textAnchor="end" fill={axisColor}>
                {mode === "cost" ? `$${v.toFixed(v < 0.01 ? 4 : 2)}` : formatTokens(Math.round(v))}
              </text>
            </g>
          );
        })}

        {/* bars */}
        {data.map((d, i) => {
          const v = mode === "cost" ? d.cost_usd : d.input_tokens + d.output_tokens;
          const h = max > 0 ? (v / max) * innerH : 0;
          const x = padX + i * barW + barW * 0.15;
          const w = barW * 0.7;
          const y = padY + innerH - h;
          const showLabel = data.length <= 30 ? i % Math.max(1, Math.ceil(data.length / 10)) === 0 : i % Math.max(1, Math.ceil(data.length / 8)) === 0;
          return (
            <g key={d.date}>
              <title>
                {d.date}: {mode === "cost" ? formatCost(d.cost_usd) : formatTokens(d.input_tokens + d.output_tokens) + " tokens"} · {d.runs} run{d.runs === 1 ? "" : "s"}
              </title>
              <rect
                x={x}
                y={y}
                width={w}
                height={Math.max(0, h)}
                rx={2}
                fill={v > 0 ? "#FF7300" : emptyBarColor}
                opacity={v > 0 ? 0.9 : 1}
              />
              {showLabel && (
                <text x={x + w / 2} y={H - 6} fontSize="9" textAnchor="middle" fill={axisColor}>
                  {shortDate(d.date)}
                </text>
              )}
            </g>
          );
        })}
      </svg>
    </Box>
  );
}

/** Horizontal bar list — one row per model, bar width proportional to total cost. */
function ModelBreakdown({ rows }: { rows: ByModel[] }) {
  const max = Math.max(0.0001, ...rows.map((r) => r.cost_usd));
  if (rows.length === 0) {
    return (
      <Stack alignItems="center" spacing={1} py={5}>
        <Box sx={{ color: "text.disabled" }}>
          <CircleStarIcon size={40} />
        </Box>
        <Typography color="text.secondary" variant="body2">
          No usage logged yet. Run the agent to start seeing data.
        </Typography>
      </Stack>
    );
  }
  return (
    <Stack spacing={1.25} sx={{ pt: 0.5 }}>
      {rows.map((r) => {
        const pct = (r.cost_usd / max) * 100;
        return (
          <Box key={r.model}>
            <Stack direction="row" justifyContent="space-between" alignItems="baseline" mb={0.5}>
              <Stack direction="row" spacing={1} alignItems="baseline">
                <Typography variant="body2" fontWeight={600} sx={{ fontFamily: "monospace" }}>
                  {r.model}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {r.runs} run{r.runs === 1 ? "" : "s"} · {formatTokens(r.total_tokens)} tokens
                </Typography>
              </Stack>
              <Typography variant="body2" fontWeight={700}>
                {formatCost(r.cost_usd)}
              </Typography>
            </Stack>
            <Box sx={{ position: "relative", height: 8, borderRadius: 1, backgroundColor: "action.hover" }}>
              <Box
                sx={{
                  position: "absolute",
                  inset: 0,
                  width: `${pct}%`,
                  borderRadius: 1,
                  backgroundColor: "primary.main",
                  transition: "width 0.4s ease",
                }}
              />
            </Box>
          </Box>
        );
      })}
    </Stack>
  );
}

export default function Cost() {
  const [chartMode, setChartMode] = useState<"cost" | "tokens">("cost");
  const [days, setDays] = useState<number>(30);

  const {
    data: summary,
    isLoading: l1,
    isError: e1,
    refetch: refetchSummary,
  } = useQuery<Summary>({
    queryKey: ["usage-summary"],
    queryFn: usageApi.summary,
    refetchInterval: 15000,
  });
  const {
    data: ts = [],
    isLoading: l2,
    isError: e2,
    refetch: refetchTimeseries,
  } = useQuery<DayPoint[]>({
    queryKey: ["usage-timeseries", days],
    queryFn: () => usageApi.timeseries(days),
    refetchInterval: 30000,
  });
  const {
    data: byModel = [],
    isLoading: l3,
    isError: e3,
    refetch: refetchByModel,
  } = useQuery<ByModel[]>({
    queryKey: ["usage-by-model"],
    queryFn: usageApi.byModel,
    refetchInterval: 30000,
  });
  const {
    data: recent = [],
    isError: e4,
    refetch: refetchRecent,
  } = useQuery<RecentRun[]>({
    queryKey: ["usage-recent"],
    queryFn: () => usageApi.recent(10),
    refetchInterval: 15000,
  });

  const isLoading = l1 || l2 || l3;
  // Any of these failing would otherwise render as a genuinely-empty
  // (zero-cost) tenant, so surface an explicit error state instead.
  const isError = e1 || e2 || e3 || e4;
  const handleRetry = () => {
    refetchSummary();
    refetchTimeseries();
    refetchByModel();
    refetchRecent();
  };

  const inOutRatio = useMemo(() => {
    if (!summary || summary.total_tokens === 0) return null;
    const i = summary.total_input_tokens;
    const o = summary.total_output_tokens;
    return { i, o, iPct: (i / summary.total_tokens) * 100, oPct: (o / summary.total_tokens) * 100 };
  }, [summary]);

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        Cost & Usage
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Real-time LLM spend across every agent run. Captured at the API call level:
        no Azure billing delay, no separate dashboard to log into.
      </Typography>

      {isError ? (
        <Alert
          severity="error"
          action={
            <Button color="inherit" size="small" onClick={handleRetry}>
              Retry
            </Button>
          }
        >
          Couldn't load usage data.
        </Alert>
      ) : isLoading ? (
        <Box display="flex" justifyContent="center" py={6}>
          <CircularProgress />
        </Box>
      ) : (
        <Stack spacing={3}>
          <Stack direction="row" spacing={2} flexWrap="wrap" rowGap={2}>
            <StatCard
              label="Total Cost"
              value={formatCost(summary?.total_cost_usd ?? 0)}
              sub={`Across ${summary?.total_runs ?? 0} run${summary?.total_runs === 1 ? "" : "s"}`}
              accent="primary.main"
            />
            <StatCard
              label="Last 30 Days"
              value={formatCost(summary?.last_30_days_cost_usd ?? 0)}
              sub={`${summary?.last_30_days_runs ?? 0} runs · ${formatTokens(summary?.last_30_days_tokens ?? 0)} tokens`}
            />
            <StatCard
              label="Last 7 Days"
              value={formatCost(summary?.last_7_days_cost_usd ?? 0)}
              sub={`${summary?.last_7_days_runs ?? 0} runs · ${formatTokens(summary?.last_7_days_tokens ?? 0)} tokens`}
            />
            <StatCard
              label="Today"
              value={formatCost(summary?.today_cost_usd ?? 0)}
              sub={`${summary?.today_runs ?? 0} run${summary?.today_runs === 1 ? "" : "s"}`}
            />
          </Stack>

          <Paper variant="outlined" sx={{ p: 3 }}>
            <Stack direction={{ xs: "column", sm: "row" }} alignItems={{ xs: "stretch", sm: "center" }} justifyContent="space-between" spacing={2} mb={2}>
              <Box>
                <Typography variant="h6" fontWeight={700}>
                  Daily {chartMode === "cost" ? "Spend" : "Tokens"}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  Last {days} days · hover bars for details
                </Typography>
              </Box>
              <Stack direction="row" spacing={1.5} flexWrap="wrap">
                <ToggleButtonGroup
                  value={chartMode}
                  exclusive
                  size="small"
                  onChange={(_, v) => v && setChartMode(v)}
                >
                  <ToggleButton value="cost">Cost</ToggleButton>
                  <ToggleButton value="tokens">Tokens</ToggleButton>
                </ToggleButtonGroup>
                <ToggleButtonGroup
                  value={days}
                  exclusive
                  size="small"
                  onChange={(_, v) => v && setDays(v)}
                >
                  <ToggleButton value={7}>7d</ToggleButton>
                  <ToggleButton value={30}>30d</ToggleButton>
                  <ToggleButton value={90}>90d</ToggleButton>
                </ToggleButtonGroup>
              </Stack>
            </Stack>
            <DailyCostChart data={ts} mode={chartMode} />
          </Paper>

          <Stack direction={{ xs: "column", md: "row" }} spacing={3}>
            <Paper variant="outlined" sx={{ p: 3, flex: 1.2 }}>
              <Typography variant="h6" fontWeight={700} sx={{ mb: 0.5 }}>
                Cost by Model
              </Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 2 }}>
                Where the money goes, broken down by LLM model.
              </Typography>
              <ModelBreakdown rows={byModel} />
            </Paper>

            <Paper variant="outlined" sx={{ p: 3, flex: 0.8 }}>
              <Typography variant="h6" fontWeight={700} sx={{ mb: 0.5 }}>
                Input vs Output
              </Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 2 }}>
                Most cost is usually output tokens. They're priced higher.
              </Typography>

              {inOutRatio ? (
                <Stack spacing={2}>
                  <Box>
                    <Stack direction="row" justifyContent="space-between" mb={0.5}>
                      <Typography variant="body2" fontWeight={600}>Input tokens</Typography>
                      <Typography variant="body2" fontWeight={700}>{formatTokens(inOutRatio.i)} ({inOutRatio.iPct.toFixed(1)}%)</Typography>
                    </Stack>
                    <Box sx={{ height: 10, borderRadius: 1, backgroundColor: "action.hover" }}>
                      <Box sx={{ width: `${inOutRatio.iPct}%`, height: "100%", borderRadius: 1, backgroundColor: "primary.main" }} />
                    </Box>
                  </Box>
                  <Box>
                    <Stack direction="row" justifyContent="space-between" mb={0.5}>
                      <Typography variant="body2" fontWeight={600}>Output tokens</Typography>
                      <Typography variant="body2" fontWeight={700}>{formatTokens(inOutRatio.o)} ({inOutRatio.oPct.toFixed(1)}%)</Typography>
                    </Stack>
                    <Box sx={{ height: 10, borderRadius: 1, backgroundColor: "action.hover" }}>
                      <Box sx={{ width: `${inOutRatio.oPct}%`, height: "100%", borderRadius: 1, backgroundColor: "#2E7DFA" }} />
                    </Box>
                  </Box>
                  <Box sx={{ pt: 1, borderTop: "1px solid", borderColor: "divider" }}>
                    <Stack direction="row" justifyContent="space-between" mt={1.25}>
                      <Typography variant="caption" color="text.secondary">Total LLM calls</Typography>
                      <Typography variant="caption" fontWeight={700}>{summary?.total_llm_calls ?? 0}</Typography>
                    </Stack>
                    <Stack direction="row" justifyContent="space-between" mt={0.5}>
                      <Typography variant="caption" color="text.secondary">Avg cost per run</Typography>
                      <Typography variant="caption" fontWeight={700}>
                        {summary && summary.total_runs > 0
                          ? formatCost(summary.total_cost_usd / summary.total_runs)
                          : "N/A"}
                      </Typography>
                    </Stack>
                  </Box>
                </Stack>
              ) : (
                <Typography variant="body2" color="text.secondary" sx={{ pt: 1 }}>
                  No data yet.
                </Typography>
              )}
            </Paper>
          </Stack>

          <Paper variant="outlined">
            <Box sx={{ p: 2.5 }}>
              <Stack direction="row" alignItems="center" spacing={1}>
                <Box sx={{ color: "text.secondary" }}>
                  <ClockAsteriskIcon size={18} />
                </Box>
                <Typography variant="h6" fontWeight={700}>Recent Runs</Typography>
              </Stack>
              <Typography variant="caption" color="text.secondary">
                Last 10 agent runs and what each one cost.
              </Typography>
            </Box>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.7rem" }}>When</TableCell>
                    <TableCell sx={{ fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.7rem" }}>Model</TableCell>
                    <TableCell align="right" sx={{ display: { xs: "none", sm: "table-cell" }, fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.7rem" }}>Tasks</TableCell>
                    <TableCell align="right" sx={{ fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.7rem" }}>In</TableCell>
                    <TableCell align="right" sx={{ fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.7rem" }}>Out</TableCell>
                    <TableCell align="right" sx={{ display: { xs: "none", sm: "table-cell" }, fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.7rem" }}>Calls</TableCell>
                    <TableCell align="right" sx={{ fontWeight: 600, color: "text.secondary", textTransform: "uppercase", fontSize: "0.7rem" }}>Cost</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {recent.map((r) => (
                    <TableRow key={r.id} hover>
                      <TableCell sx={{ color: "text.secondary", whiteSpace: "nowrap" }}>
                        <Tooltip title={new Date(r.created_at).toLocaleString()}>
                          <span>{new Date(r.created_at).toLocaleString()}</span>
                        </Tooltip>
                      </TableCell>
                      <TableCell>
                        <Chip
                          label={r.model}
                          size="small"
                          variant="outlined"
                          sx={{ fontFamily: "monospace", fontSize: "0.7rem", height: 20 }}
                        />
                      </TableCell>
                      <TableCell align="right" sx={{ display: { xs: "none", sm: "table-cell" } }}>{r.subtask_count}</TableCell>
                      <TableCell align="right">{formatTokens(r.input_tokens)}</TableCell>
                      <TableCell align="right">{formatTokens(r.output_tokens)}</TableCell>
                      <TableCell align="right" sx={{ display: { xs: "none", sm: "table-cell" } }}>{r.llm_calls}</TableCell>
                      <TableCell align="right" sx={{ fontWeight: 700, color: "primary.main" }}>
                        {formatCost(r.cost_usd)}
                      </TableCell>
                    </TableRow>
                  ))}
                  {recent.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={7} align="center" sx={{ py: 4, color: "text.disabled" }}>
                        No runs logged yet.
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          </Paper>
        </Stack>
      )}
    </Box>
  );
}
