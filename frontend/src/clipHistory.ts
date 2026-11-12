export interface ClipEntry {
  id: number;
  type: string;
  content: string;
  preview: string;
  thumbnail?: string;
  timestamp: number;
  pinned: boolean;
  sourceApp?: string;
  sourceTitle?: string;
  sourcePath?: string;
  note?: string;
  tags?: string;
}

export interface ClipHistoryChange {
  entry: ClipEntry | null;
  deletedIds: number[];
  total: number;
}

export function sortClips(clips: ClipEntry[]) {
  return [...clips].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    return b.timestamp - a.timestamp;
  });
}

export function mergeClips(existing: ClipEntry[], incoming: ClipEntry[]) {
  const byId = new Map<number, ClipEntry>();
  existing.forEach((clip) => byId.set(clip.id, clip));
  incoming.forEach((clip) => byId.set(clip.id, clip));
  return sortClips(Array.from(byId.values()));
}

export function applyHistoryChange(
  existing: ClipEntry[],
  change: ClipHistoryChange,
  pageSize = 50,
) {
  const deleted = new Set(change.deletedIds);
  const kept = existing.filter((clip) => !deleted.has(clip.id));
  const incoming = change.entry && !deleted.has(change.entry.id) ? [change.entry] : [];
  // Retain the loaded page capacity rather than accumulating every captured
  // item in a resident, hidden WebView. Explicit pagination can grow it.
  const capacity = Math.max(pageSize, existing.length);
  const items = mergeClips(kept, incoming).slice(0, Math.min(capacity, change.total));
  return { items, total: change.total, hasMore: items.length < change.total };
}
