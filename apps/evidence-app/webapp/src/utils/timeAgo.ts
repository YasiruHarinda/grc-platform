/**
 * Renders an ISO timestamp as "3 minutes ago" / "2 hours ago" / "5 days ago".
 *
 * Lives here rather than in a component because all three pickers show it in
 * their delete dialog, and the elapsed time is the load-bearing part of that
 * warning: an agent run's `status` alone can't be trusted (a crashed Runner
 * leaves a row marked "running" for ever, and there's no heartbeat column),
 * but a person reading "3 minutes ago" against "6 days ago" can tell a live
 * run from a stale one instantly.
 *
 * `Intl.RelativeTimeFormat` is built in, so this needs no date library —
 * package.json has none, and one warning string doesn't justify adding one.
 * All it costs is picking the unit by hand.
 */
export function timeAgo(isoDate: string | null): string {
  if (!isoDate) return "recently";

  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  const diffMinutes = Math.round((new Date(isoDate).getTime() - Date.now()) / 60000);
  if (Math.abs(diffMinutes) < 60) return rtf.format(diffMinutes, "minute");

  const diffHours = Math.round(diffMinutes / 60);
  if (Math.abs(diffHours) < 24) return rtf.format(diffHours, "hour");

  const diffDays = Math.round(diffHours / 24);
  return rtf.format(diffDays, "day");
}
