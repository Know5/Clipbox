export function formatTime(ts: number) {
  const d = new Date(ts);
  const now = new Date();
  const diff = now.getTime() - d.getTime();
  if (diff < 60_000) return "刚刚";
  if (diff < 3600_000) return Math.floor(diff / 60_000) + " 分钟前";
  if (diff < 86400_000) return Math.floor(diff / 3600_000) + " 小时前";
  if (d.toDateString() === now.toDateString()) {
    return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  }
  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  if (d.toDateString() === yesterday.toDateString()) return "昨天";
  return d.toLocaleDateString([], { month: "short", day: "numeric" });
}
