import { useState, useEffect, useCallback, useRef, memo } from "react";
import { GetClipPageByTypeAndTag, SearchClipPageByTypeAndTag, GetClipTags, CopyToClipboard, CopyAndPaste, TogglePin, DeleteClip, ClearAll, GetHotkeySettings, GetAppSettings, HideWindow, ToggleWindowPin, GetWindowPinned, GetImageDataURL, GetClipDetails, GetClipImageMeta, UpdateClipMetadata, ExportClip, ReadClipboardText, GetSourceIcon } from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime/runtime";
import Settings from "./Settings";
import { refreshAccent, syncWindowBackground } from "./theme";
import "./App.css";

interface ClipEntry {
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

interface ClipPage {
  items: ClipEntry[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
}

interface ClipTag {
  name: string;
  count: number;
}

interface ClearResult {
  deletedClips?: number;
  deletedImages?: number;
  pinnedKept?: number;
  stats?: {
    pinnedClips?: number;
  };
}

interface ClipExportResult {
  path?: string;
  type?: string;
  bytes?: number;
  cancelled?: boolean;
}

interface CopyFeedback {
  clipId: number | null;
  message: string;
  error: boolean;
}

const PAGE_SIZE = 50;

// 右键菜单：搜索框一套，列表项一套。
type CtxMenu =
  | { x: number; y: number; kind: "search" }
  | { x: number; y: number; kind: "item"; clipId: number };
type ClipFilter = "all" | "text" | "image";

function describeClearResult(result?: ClearResult) {
  const deleted = result?.deletedClips ?? 0;
  const pinned = result?.pinnedKept ?? result?.stats?.pinnedClips ?? 0;
  if (deleted > 0) return `已清空 ${deleted} 条，保留 ${pinned} 条置顶`;
  if (pinned > 0) return `没有可清空记录，保留 ${pinned} 条置顶`;
  return "没有可清空记录";
}

function clipSourceTitle(clip: ClipEntry) {
  return [clip.sourceApp, clip.sourceTitle].map((item) => (item || "").trim()).filter(Boolean).join(" · ");
}

function formatDetailTime(ts: number) {
  if (!ts) return "-";
  return new Date(ts).toLocaleString([], {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function ClipDetailPanel({
  clip,
  imageUrl,
  imageLoading,
  loading,
  error,
  onClose,
  onCopy,
  onSaveMetadata,
  onExport,
}: {
  clip: ClipEntry | null;
  imageUrl: string;
  imageLoading: boolean;
  loading: boolean;
  error: string;
  onClose: () => void;
  onCopy: (id: number) => void;
  onSaveMetadata: (id: number, note: string, tags: string) => Promise<ClipEntry>;
  onExport: (id: number) => Promise<ClipExportResult>;
}) {
  const [note, setNote] = useState("");
  const [tags, setTags] = useState("");
  const [saving, setSaving] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [saveMessage, setSaveMessage] = useState("");

  useEffect(() => {
    setNote(clip?.note || "");
    setTags(clip?.tags || "");
    setSaveMessage("");
  }, [clip?.id, clip?.note, clip?.tags]);

  const metadataChanged = Boolean(clip) && (note !== (clip?.note || "") || tags !== (clip?.tags || ""));

  const handleSave = async () => {
    if (!clip || saving || !metadataChanged) return;
    setSaving(true);
    setSaveMessage("");
    try {
      const updated = await onSaveMetadata(clip.id, note, tags);
      setNote(updated.note || "");
      setTags(updated.tags || "");
      setSaveMessage("已保存");
    } catch (e) {
      console.error(e);
      setSaveMessage("保存失败");
    } finally {
      setSaving(false);
    }
  };

  const handleExport = async () => {
    if (!clip || exporting) return;
    setExporting(true);
    setSaveMessage("");
    try {
      const result = await onExport(clip.id);
      setSaveMessage(result?.cancelled ? "已取消导出" : "已导出");
    } catch (e) {
      console.error(e);
      setSaveMessage("导出失败");
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="detail-layer" onClick={onClose}>
      <div className="detail-panel" onClick={(e) => e.stopPropagation()}>
        <div className="detail-head">
          <div className="detail-title">
            <span className={`dot ${clip?.type || "text"}`} />
            <span>{clip?.type === "image" ? "图片详情" : "文本详情"}</span>
          </div>
          <button className="detail-close" onClick={onClose} title="关闭">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        <div className="detail-body">
          {loading && <div className="detail-empty">加载中...</div>}
          {!loading && error && <div className="detail-error">{error}</div>}
          {!loading && clip && (
            <>
              <div className="detail-meta-grid">
                <div><span>时间</span><strong>{formatDetailTime(clip.timestamp)}</strong></div>
                {clip.sourceApp && <div><span>应用</span><strong title={clip.sourcePath || clip.sourceApp}>{clip.sourceApp}</strong></div>}
                {clip.sourceTitle && <div><span>窗口</span><strong title={clip.sourceTitle}>{clip.sourceTitle}</strong></div>}
              </div>
              <div className="detail-metadata-editor">
                <label>
                  <span>备注</span>
                  <textarea
                    value={note}
                    maxLength={2000}
                    onChange={(e) => setNote(e.target.value)}
                    placeholder="给这条记录留个备注"
                  />
                </label>
                <label>
                  <span>标签</span>
                  <input
                    value={tags}
                    maxLength={240}
                    onChange={(e) => setTags(e.target.value)}
                    placeholder="工作, 资料, 待处理"
                  />
                </label>
              </div>
              {clip.type === "text" ? (
                <pre className="detail-text">{clip.content}</pre>
              ) : (
                <div className="detail-image-wrap">
                  {imageLoading && <div className="detail-empty">加载中...</div>}
                  {!imageLoading && imageUrl && <img src={imageUrl} className="detail-image" alt="clip-detail" />}
                  {!imageLoading && !imageUrl && <div className="detail-empty">图片不可用</div>}
                </div>
              )}
            </>
          )}
        </div>

        <div className="detail-actions">
          {saveMessage && <span className={`detail-save-message ${saveMessage.endsWith("失败") ? "error" : ""}`}>{saveMessage}</span>}
          <button className="btn-lite" onClick={onClose}>关闭</button>
          {clip && <button className="btn-lite" onClick={handleSave} disabled={saving || !metadataChanged}>{saving ? "保存中..." : "保存"}</button>}
          {clip && <button className="btn-lite" onClick={handleExport} disabled={exporting}>{exporting ? "导出中..." : "导出"}</button>}
          {clip && <button className="btn-lite primary" onClick={() => onCopy(clip.id)}>复制</button>}
        </div>
      </div>
    </div>
  );
}

function sortClips(clips: ClipEntry[]) {
  return [...clips].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    return b.timestamp - a.timestamp;
  });
}

function formatTime(ts: number) {
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

function mergeClips(existing: ClipEntry[], incoming: ClipEntry[]) {
  const byId = new Map<number, ClipEntry>();
  existing.forEach((clip) => byId.set(clip.id, clip));
  incoming.forEach((clip) => byId.set(clip.id, clip));
  return sortClips(Array.from(byId.values()));
}

interface ClipImageMeta {
  id: number;
  width: number;
  height: number;
  bytes: number;
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/** Bold the search terms in a preview line (spec 4.4). */
function Highlight({ text, query }: { text: string; query: string }) {
  const terms = query.trim().split(/\s+/).filter(Boolean).map(escapeRegExp);
  if (terms.length === 0) return <>{text}</>;
  const parts = text.split(new RegExp(`(${terms.join("|")})`, "gi"));
  return (
    <>
      {parts.map((part, index) => (index % 2 === 1 ? <strong className="hl" key={index}>{part}</strong> : part))}
    </>
  );
}

/** 来源应用图标：按 exe 路径缓存，取不到就退回按类型画的图标。 */
const sourceIconCache = new Map<string, string>();
const sourceIconPending = new Map<string, Promise<string>>();

function loadSourceIcon(path: string): Promise<string> {
  const cached = sourceIconCache.get(path);
  if (cached !== undefined) return Promise.resolve(cached);
  const pending = sourceIconPending.get(path);
  if (pending) return pending;
  const request = GetSourceIcon(path)
    .then((url) => {
      const value = url || "";
      sourceIconCache.set(path, value);
      return value;
    })
    .catch(() => {
      sourceIconCache.set(path, "");
      return "";
    })
    .finally(() => sourceIconPending.delete(path));
  sourceIconPending.set(path, request);
  return request;
}

function TypeIcon({ type }: { type: string }) {
  if (type === "image") {
    return (
      <svg className="clip-type" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.2">
        <rect x="2" y="2.5" width="12" height="11" rx="1.5" />
        <circle cx="5.6" cy="6" r="1" />
        <path d="M3 12l3.2-3.4 2.4 2.2L11 8l2.2 4" />
      </svg>
    );
  }
  return (
    <svg className="clip-type" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.2">
      <path d="M3 2.5h6.5L13 6v7.5H3z" />
      <path d="M9.2 2.7V6h3.6" />
      <path d="M5 8.5h6M5 11h4.5" />
    </svg>
  );
}

function SourceIcon({ path, type }: { path?: string; type: string }) {
  const [src, setSrc] = useState(() => (path ? sourceIconCache.get(path) || "" : ""));
  useEffect(() => {
    if (!path) {
      setSrc("");
      return;
    }
    const cached = sourceIconCache.get(path);
    if (cached !== undefined) {
      setSrc(cached);
      return;
    }
    let active = true;
    loadSourceIcon(path).then((url) => {
      if (active) setSrc(url);
    });
    return () => {
      active = false;
    };
  }, [path]);
  if (!src) return <TypeIcon type={type} />;
  return <img className="clip-source-icon" src={src} alt="" />;
}

function PinIcon({ className }: { className?: string }) {
  return (
    <svg className={className} width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
      <path d="M15.6 3H8.4v2.1l1.1.9v4.5L7 13.1V15h4.6v6h.8v-6H17v-1.9l-2.5-2.6V6l1.1-.9z" />
    </svg>
  );
}

function formatBytes(bytes: number) {
  if (!bytes || bytes < 0) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatImageMeta(meta?: ClipImageMeta) {
  if (!meta) return "";
  const size = meta.width && meta.height ? `${meta.width}×${meta.height}` : "";
  return [size, formatBytes(meta.bytes)].filter(Boolean).join(" · ");
}

interface ClipItemProps {
  clip: ClipEntry;
  index: number;
  query: string;
  timeLabel: string;
  imageLabel?: string;
  selected: boolean;
  copyStatus: CopyFeedback | null;
  deleteConfirming: boolean;
  deleting: boolean;
  onHover: (id: number) => void;
  onDouble: (id: number) => void;
  onPin: (id: number) => void;
  onDelete: (id: number) => void;
  onDetail: (id: number) => void;
}

/** 单行列表项。memo 化后，悬停/选中/反馈只重渲染受影响的行，而不是整个列表。 */
const ClipItem = memo(function ClipItem({
  clip,
  index,
  query,
  timeLabel,
  imageLabel,
  selected,
  copyStatus,
  deleteConfirming,
  deleting,
  onHover,
  onDouble,
  onPin,
  onDelete,
  onDetail,
}: ClipItemProps) {
  // 单击 = 复制并粘贴回原输入框（与回车一致）。拖选文字时不触发。
  const handleClick = () => {
    const selection = window.getSelection();
    if (selection && selection.toString().length > 0) return;
    onDouble(clip.id);
  };

  const status = copyStatus ? copyStatus.message : deleteConfirming || deleting ? (deleting ? "删除中…" : "再点删除") : "";
  const previewText = clip.type === "image" ? imageLabel || "图片" : clip.preview;

  return (
    <div
      data-clip-id={clip.id}
      role="option"
      aria-selected={selected}
      className={`clip-item ${clip.type === "image" ? "tall" : ""} ${clip.pinned ? "pinned" : ""} ${selected ? "selected" : ""} ${copyStatus ? (copyStatus.error ? "copy-error" : "copied") : ""} ${deleteConfirming ? "delete-confirming" : ""}`}
      title={clipSourceTitle(clip)}
      onMouseEnter={() => onHover(clip.id)}
      onClick={handleClick}
    >
      <span className="clip-index">{index < 9 ? index + 1 : ""}</span>
      {clip.type === "image" && clip.thumbnail ? (
        <img className="clip-thumb" src={clip.thumbnail} alt="" />
      ) : (
        <SourceIcon path={clip.sourcePath} type={clip.type} />
      )}
      <span className="clip-preview">
        <Highlight text={previewText} query={query} />
      </span>
      {clip.pinned && <PinIcon className="clip-pin" />}
      <span className={`clip-meta ${copyStatus?.error || deleteConfirming ? "error" : ""}`}>{status || timeLabel}</span>
      <div className="clip-actions">
        <button
          className="act"
          onClick={(e) => { e.stopPropagation(); onDetail(clip.id); }}
          title="查看详情 (Space)"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="3" />
            <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
          </svg>
        </button>
        <button
          className={`act ${clip.pinned ? "on" : ""}`}
          onClick={(e) => { e.stopPropagation(); onPin(clip.id); }}
          title={clip.pinned ? "取消置顶 (P)" : "置顶 (P)"}
        >
          <PinIcon />
        </button>
        <button
          className={`act danger ${deleteConfirming ? "confirming" : ""}`}
          onClick={(e) => { e.stopPropagation(); onDelete(clip.id); }}
          disabled={deleting}
          title={deleteConfirming ? "再次点击删除" : "删除 (Del)"}
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="3 6 5 6 21 6" /><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
          </svg>
        </button>
      </div>
    </div>
  );
});

function App() {
  const [clips, setClips] = useState<ClipEntry[]>([]);
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<ClipFilter>("all");
  const [selectedTag, setSelectedTag] = useState("");
  const [tagOptions, setTagOptions] = useState<ClipTag[]>([]);
  const [showSettings, setShowSettings] = useState(false);
  const [openPanel, setOpenPanel] = useState<null | "tags" | "help" | "more">(null);
  const [imageMeta, setImageMeta] = useState<Record<number, ClipImageMeta>>({});
  const requestedImageMeta = useRef<Set<number>>(new Set());
  const [hotkeyDisplay, setHotkeyDisplay] = useState("Ctrl + Alt + V");
  const [windowPinned, setWindowPinned] = useState(false);
  const [totalClips, setTotalClips] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [listLoading, setListLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [listError, setListError] = useState("");
  const [selectedClipId, setSelectedClipId] = useState<number | null>(null);
  const [clearConfirming, setClearConfirming] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [clearMessage, setClearMessage] = useState("");
  const [ctxMenu, setCtxMenu] = useState<CtxMenu | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const [showBackToTop, setShowBackToTop] = useState(false);
  // 方向键走到已加载末尾时，加载下一页后自动选中的第一条
  const pendingSelectNext = useRef(false);
  const [deleteConfirmId, setDeleteConfirmId] = useState<number | null>(null);
  const [deleteDeletingId, setDeleteDeletingId] = useState<number | null>(null);
  const [copyFeedback, setCopyFeedback] = useState<CopyFeedback>({ clipId: null, message: "", error: false });
  const [detailClip, setDetailClip] = useState<ClipEntry | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [detailImageUrl, setDetailImageUrl] = useState("");
  const [detailImageLoading, setDetailImageLoading] = useState(false);
  // 每分钟自增一次，触发相对时间标签（"x 分钟前"）重新计算
  const [, setTimeTick] = useState(0);
  const searchRef = useRef<HTMLInputElement>(null);
  const searchValueRef = useRef("");
  const filterValueRef = useRef<ClipFilter>("all");
  const selectedTagRef = useRef("");
  const clipsRef = useRef<ClipEntry[]>([]);
  const requestSeq = useRef(0);
  const clearConfirmTimer = useRef<number | null>(null);
  const clearMessageTimer = useRef<number | null>(null);
  const deleteConfirmTimer = useRef<number | null>(null);
  const copyFeedbackTimer = useRef<number | null>(null);

  const cancelClearConfirmTimer = useCallback(() => {
    if (clearConfirmTimer.current !== null) {
      window.clearTimeout(clearConfirmTimer.current);
      clearConfirmTimer.current = null;
    }
  }, []);

  const cancelDeleteConfirmTimer = useCallback(() => {
    if (deleteConfirmTimer.current !== null) {
      window.clearTimeout(deleteConfirmTimer.current);
      deleteConfirmTimer.current = null;
    }
  }, []);

  const showClearFeedback = useCallback((message: string) => {
    if (clearMessageTimer.current !== null) {
      window.clearTimeout(clearMessageTimer.current);
    }
    setClearMessage(message);
    clearMessageTimer.current = window.setTimeout(() => {
      setClearMessage("");
      clearMessageTimer.current = null;
    }, 4000);
  }, []);

  const showCopyFeedback = useCallback((clipId: number, message: string, error = false) => {
    if (copyFeedbackTimer.current !== null) {
      window.clearTimeout(copyFeedbackTimer.current);
    }
    setCopyFeedback({ clipId, message, error });
    copyFeedbackTimer.current = window.setTimeout(() => {
      setCopyFeedback({ clipId: null, message: "", error: false });
      copyFeedbackTimer.current = null;
    }, error ? 3600 : 1800);
  }, []);

  useEffect(() => {
    return () => {
      if (clearConfirmTimer.current !== null) {
        window.clearTimeout(clearConfirmTimer.current);
      }
      if (clearMessageTimer.current !== null) {
        window.clearTimeout(clearMessageTimer.current);
      }
      if (deleteConfirmTimer.current !== null) {
        window.clearTimeout(deleteConfirmTimer.current);
      }
      if (copyFeedbackTimer.current !== null) {
        window.clearTimeout(copyFeedbackTimer.current);
      }
    };
  }, []);

  const loadClipPage = useCallback(async (
    offset = 0,
    append = false,
    query = searchValueRef.current.trim(),
    clipFilter = filterValueRef.current,
    tag = selectedTagRef.current
  ) => {
    const seq = requestSeq.current + 1;
    requestSeq.current = seq;
    if (append) {
      setLoadingMore(true);
    } else {
      setListLoading(true);
    }
    setListError("");
    try {
      const [page, tags] = await Promise.all([
        query
          ? SearchClipPageByTypeAndTag(query, clipFilter, tag, PAGE_SIZE, offset)
          : GetClipPageByTypeAndTag(clipFilter, tag, PAGE_SIZE, offset),
        GetClipTags(query, clipFilter),
      ]) as [ClipPage, ClipTag[]];
      if (seq !== requestSeq.current) return;
      const items = page?.items || [];
      setClips((prev) => append ? mergeClips(prev, items) : items);
      if (!append) {
        pendingSelectNext.current = false;
      } else if (pendingSelectNext.current && items.length > 0) {
        // 方向键翻页：选到下一页的第一条
        pendingSelectNext.current = false;
        setSelectedClipId(items[0].id);
      }
      setTagOptions(tags || []);
      setTotalClips(page?.total || 0);
      setHasMore(Boolean(page?.hasMore));
    } catch (e) {
      console.error(e);
      if (seq === requestSeq.current) {
        setListError("加载历史失败");
      }
    } finally {
      if (seq === requestSeq.current) {
        setListLoading(false);
        setLoadingMore(false);
      }
    }
  }, []);

  useEffect(() => {
    searchValueRef.current = search;
  }, [search]);

  useEffect(() => {
    filterValueRef.current = filter;
  }, [filter]);

  useEffect(() => {
    selectedTagRef.current = selectedTag;
  }, [selectedTag]);

  useEffect(() => {
    clipsRef.current = clips;
  }, [clips]);

  useEffect(() => {
    loadClipPage(0, false);
    const unsub1 = EventsOn("clip:new", (entry?: ClipEntry) => {
      const activeSearch = searchValueRef.current.trim();
      const activeFilter = filterValueRef.current;
      const activeTag = selectedTagRef.current;
      // 无过滤条件时增量合并新条目，保留已加载的分页和滚动位置；
      // 处于搜索/标签/类型过滤视图时仍整页刷新，让后端决定匹配结果。
      if (!entry || !entry.id || activeSearch || activeTag || (activeFilter !== "all" && entry.type !== activeFilter)) {
        loadClipPage(0, false, activeSearch, activeFilter, activeTag);
        return;
      }
      const isNew = !clipsRef.current.some((clip) => clip.id === entry.id);
      setClips((prev) => mergeClips(prev, [entry]));
      if (isNew) setTotalClips((total) => total + 1);
    });
    const unsub2 = EventsOn("window:shown", () => {
      setTimeout(() => searchRef.current?.focus(), 100);
      refreshAccent().catch(console.error);
      // 每次重新打开都回到最新一页，避免滚到底后加载的分页"常驻"成 100/500、200/500。
      if (clipsRef.current.length > PAGE_SIZE) {
        setClips((prev) => prev.slice(0, PAGE_SIZE));
        setHasMore(true);
        setSelectedClipId(null);
      }
      if (listRef.current) listRef.current.scrollTop = 0;
      setShowBackToTop(false);
    });
    const unsub3 = EventsOn("clips:changed", () => {
      const activeSearch = searchValueRef.current.trim();
      loadClipPage(0, false, activeSearch, filterValueRef.current, selectedTagRef.current);
    });
    const unsub4 = EventsOn("clip:skipped", (payload?: { message?: string }) => {
      const message = typeof payload?.message === "string" ? payload.message : "";
      if (message) showClearFeedback(message);
    });
    const unsub5 = EventsOn("settings:open", () => {
      setShowSettings(true);
    });
    return () => { unsub1(); unsub2(); unsub3(); unsub4(); unsub5(); };
  }, [loadClipPage, showClearFeedback]);

  useEffect(() => {
    const timer = setTimeout(() => {
      loadClipPage(0, false, search.trim(), filter, selectedTag);
    }, 120);
    return () => clearTimeout(timer);
  }, [filter, search, selectedTag, loadClipPage]);

  useEffect(() => {
    GetHotkeySettings().then(config => {
      setHotkeyDisplay(config.display);
    }).catch(console.error);
  }, [showSettings]);

  useEffect(() => {
    GetWindowPinned().then(setWindowPinned).catch(console.error);
  }, []);

  const togglePanel = useCallback((panel: "tags" | "help" | "more") => {
    setOpenPanel((current) => (current === panel ? null : panel));
  }, []);

  // 点面板外、窗口失焦、按 Esc 都关掉面板（面板自身的 mousedown 已 stopPropagation）。
  useEffect(() => {
    if (!openPanel) return;
    const close = () => setOpenPanel(null);
    window.addEventListener("mousedown", close);
    window.addEventListener("blur", close);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("blur", close);
    };
  }, [openPanel]);

  // 图片行的尺寸和文件大小：按需批量取，取到后记住；失败的 id 不再重试，避免刷请求。
  useEffect(() => {
    const missing = clips
      .filter((clip) => clip.type === "image" && !imageMeta[clip.id] && !requestedImageMeta.current.has(clip.id))
      .map((clip) => clip.id);
    if (missing.length === 0) return;
    missing.forEach((id) => requestedImageMeta.current.add(id));
    let active = true;
    GetClipImageMeta(missing)
      .then((items) => {
        if (!active || !items || items.length === 0) return;
        setImageMeta((prev) => {
          const next = { ...prev };
          for (const item of items) next[item.id] = item;
          return next;
        });
      })
      .catch(console.error);
    return () => {
      active = false;
    };
  }, [clips, imageMeta]);

  useEffect(() => {
    const timer = window.setInterval(() => setTimeTick((tick) => tick + 1), 60_000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    // 主题：dark / light / system（跟随 Windows），应用到 <html data-theme>
    const media = window.matchMedia("(prefers-color-scheme: light)");
    let mode = "dark";
    const apply = () => {
      const resolved = mode === "light" || (mode === "system" && media.matches) ? "light" : "dark";
      document.documentElement.setAttribute("data-theme", resolved);
      syncWindowBackground();
      refreshAccent().catch(console.error);
    };
    GetAppSettings()
      .then((settings) => {
        mode = (settings as { theme?: string })?.theme || "dark";
        apply();
      })
      .catch(console.error);
    const unsub = EventsOn("settings:updated", (settings?: { theme?: string }) => {
      mode = settings?.theme || "dark";
      apply();
    });
    const onMediaChange = () => {
      if (mode === "system") apply();
    };
    media.addEventListener("change", onMediaChange);
    return () => {
      unsub();
      media.removeEventListener("change", onMediaChange);
    };
  }, []);

  const handleToggleWindowPin = async () => {
    try {
      const pinned = await ToggleWindowPin();
      setWindowPinned(pinned);
    } catch (e) {
      console.error(e);
    }
  };

  const handleCopy = useCallback(async (id: number) => {
    try {
      await CopyToClipboard(id);
      setListError("");
      showCopyFeedback(id, "已复制");
    } catch (e) {
      console.error(e);
      showCopyFeedback(id, "复制失败", true);
    }
  }, [showCopyFeedback]);


  const handleDoubleClick = useCallback(async (id: number) => {
    try {
      await CopyAndPaste(id);
      setListError("");
      showCopyFeedback(id, "已粘贴");
    } catch (e) {
      console.error(e);
      showCopyFeedback(id, "粘贴失败", true);
    }
  }, [showCopyFeedback]);
  const handleHover = useCallback((id: number) => {
    setSelectedClipId(id);
  }, []);

  const handlePin = useCallback(async (id: number) => {
    try {
      await TogglePin(id);
      // 本地翻转置顶状态并重排，保留已加载的分页
      setClips((prev) => sortClips(prev.map((clip) => (clip.id === id ? { ...clip, pinned: !clip.pinned } : clip))));
    } catch (e) {
      console.error(e);
    }
  }, []);

  // ── 右键菜单（搜索框项 / 列表项）──
  const openSearchMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    setCtxMenu({ x: e.clientX, y: e.clientY, kind: "search" });
  }, []);

  const openItemMenu = useCallback((e: React.MouseEvent) => {
    const target = (e.target as HTMLElement).closest("[data-clip-id]");
    if (!target) {
      e.preventDefault();
      setCtxMenu(null);
      return;
    }
    const id = Number(target.getAttribute("data-clip-id"));
    if (!Number.isFinite(id)) return;
    e.preventDefault();
    setSelectedClipId(id);
    setCtxMenu({ x: e.clientX, y: e.clientY, kind: "item", clipId: id });
  }, []);

  const handlePasteSearch = useCallback(async () => {
    setCtxMenu(null);
    try {
      const text = await ReadClipboardText();
      // 搜索框是单行：只取第一行，避免把整段文本塞进去
      const line = (text || "").split(/\r?\n/)[0].trim();
      if (line) setSearch(line);
      searchRef.current?.focus();
    } catch (e) {
      console.error(e);
    }
  }, []);

  // 菜单关闭：点任意处、窗口失焦、尺寸变化
  useEffect(() => {
    if (!ctxMenu) return;
    const close = () => setCtxMenu(null);
    window.addEventListener("mousedown", close);
    window.addEventListener("blur", close);
    window.addEventListener("resize", close);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("blur", close);
      window.removeEventListener("resize", close);
    };
  }, [ctxMenu]);

  const handleDelete = useCallback(async (id: number) => {
    cancelDeleteConfirmTimer();
    setDeleteConfirmId(null);
    setDeleteDeletingId(id);
    setListError("");
    try {
      await DeleteClip(id);
      setDetailClip((clip) => clip?.id === id ? null : clip);
      // 本地移除，保留已加载的分页
      setClips((prev) => prev.filter((clip) => clip.id !== id));
      setTotalClips((total) => Math.max(0, total - 1));
    } catch (e) {
      console.error(e);
      setListError("删除失败");
    } finally {
      setDeleteDeletingId((current) => current === id ? null : current);
    }
  }, [cancelDeleteConfirmTimer]);

  const requestDelete = useCallback((id: number) => {
    if (deleteDeletingId === id) return;
    setListError("");

    if (deleteConfirmId === id) {
      handleDelete(id).catch(console.error);
      return;
    }

    cancelDeleteConfirmTimer();
    setDeleteConfirmId(id);
    deleteConfirmTimer.current = window.setTimeout(() => {
      setDeleteConfirmId((current) => current === id ? null : current);
      deleteConfirmTimer.current = null;
    }, 3000);
  }, [cancelDeleteConfirmTimer, deleteConfirmId, deleteDeletingId, handleDelete]);

  const closeDetail = useCallback(() => {
    setDetailClip(null);
    setDetailLoading(false);
    setDetailError("");
    setDetailImageUrl("");
    setDetailImageLoading(false);
  }, []);

  const handleOpenDetail = useCallback(async (id: number) => {
    setDetailClip(null);
    setDetailError("");
    setDetailImageUrl("");
    setDetailLoading(true);
    try {
      const detail = (await GetClipDetails(id)) as ClipEntry;
      setDetailClip(detail);
      if (detail.type === "image") {
        setDetailImageLoading(true);
        try {
          const url = await GetImageDataURL(id);
          setDetailImageUrl(url || "");
        } catch (e) {
          console.error(e);
          setDetailError("图片加载失败");
        } finally {
          setDetailImageLoading(false);
        }
      }
    } catch (e) {
      console.error(e);
      setDetailError("加载详情失败");
    } finally {
      setDetailLoading(false);
    }
  }, []);

  const handleSaveMetadata = useCallback(async (id: number, note: string, tags: string) => {
    const updated = (await UpdateClipMetadata(id, note, tags)) as ClipEntry;
    setDetailClip(updated);
    // 本地更新对应条目并刷新标签选项，不整页重载
    setClips((prev) => prev.map((clip) => clip.id === id ? { ...clip, note: updated.note, tags: updated.tags } : clip));
    GetClipTags(searchValueRef.current.trim(), filterValueRef.current)
      .then((tagList) => setTagOptions(tagList || []))
      .catch(console.error);
    return updated;
  }, []);

  const handleExportClip = useCallback(async (id: number) => {
    return (await ExportClip(id)) as ClipExportResult;
  }, []);

  const handleClear = async () => {
    if (clearing) return;
    setListError("");
    setClearMessage("");
    if (!clearConfirming) {
      cancelClearConfirmTimer();
      setClearConfirming(true);
      clearConfirmTimer.current = window.setTimeout(() => {
        setClearConfirming(false);
        clearConfirmTimer.current = null;
      }, 3000);
      return;
    }

    cancelClearConfirmTimer();
    setClearConfirming(false);
    setClearing(true);
    try {
      const result = (await ClearAll()) as ClearResult;
      await loadClipPage(0, false, searchValueRef.current.trim(), filterValueRef.current, selectedTagRef.current);
      showClearFeedback(describeClearResult(result));
    } catch (e) {
      console.error(e);
      setListError("清空失败");
    } finally {
      setClearing(false);
    }
  };

  const handleLoadMore = () => {
    if (hasMore && !loadingMore && !listLoading) {
      loadClipPage(clips.length, true, searchValueRef.current.trim(), filterValueRef.current, selectedTagRef.current);
    }
  };

  // 滚动到底自动加载下一页（剩 240px 时提前触发），不用再点"加载更多"。
  const handleListScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const el = e.currentTarget;
    setShowBackToTop(el.scrollTop > 120);
    if (!hasMore || loadingMore || listLoading) return;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 240) {
      loadClipPage(clipsRef.current.length, true, searchValueRef.current.trim(), filterValueRef.current, selectedTagRef.current);
    }
  };

  const backToTop = () => {
    if (listRef.current) listRef.current.scrollTop = 0;
    setShowBackToTop(false);
  };

  const filtered = clips;
  const selectedIndex = selectedClipId == null ? -1 : filtered.findIndex((clip) => clip.id === selectedClipId);
  const selectedClip = selectedIndex >= 0 ? filtered[selectedIndex] : null;
  const detailOpen = detailLoading || Boolean(detailClip) || Boolean(detailError);

  useEffect(() => {
    if (filtered.length === 0) {
      if (selectedClipId !== null) setSelectedClipId(null);
      return;
    }
    if (selectedIndex < 0) {
      setSelectedClipId(filtered[0].id);
    }
  }, [filtered, selectedClipId, selectedIndex]);

  useEffect(() => {
    if (selectedClipId == null) return;
    const element = document.querySelector(`[data-clip-id="${selectedClipId}"]`);
    element?.scrollIntoView({ block: "nearest" });
  }, [selectedClipId]);

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.isComposing) return;
      if (e.key === "Escape") {
        if (openPanel) {
          setOpenPanel(null);
          return;
        }
        if (detailOpen) {
          closeDetail();
          return;
        }
        if (showSettings) {
          // 正在编辑设置输入框时，Esc 先失焦（触发失焦保存），再次 Esc 才返回主界面
          const active = document.activeElement as HTMLElement | null;
          if (active && (active.tagName === "INPUT" || active.tagName === "TEXTAREA")) {
            active.blur();
            return;
          }
          setShowSettings(false);
          return;
        }
        if (search.trim()) {
          // 有搜索词时先清空搜索，再次 Esc 才隐藏窗口
          setSearch("");
          return;
        }
        HideWindow();
        return;
      }
      if (showSettings) return;

      const target = e.target as HTMLElement | null;
      const isInput = target?.tagName === "INPUT" || target?.tagName === "TEXTAREA" || Boolean(target?.isContentEditable);
      // 搜索框是键盘导航的起点：放行 ↑↓/Enter 直接操作列表，其余按键正常输入文字。
      const isSearchInput = target === searchRef.current;
      if (isInput && !isSearchInput) return;
      // Home = 一下回到最新（列表顶部），不用滚回去（2026-10-29 使用者要求）
      if (e.key === "Home" && !isInput) {
        if (filtered.length === 0) return;
        e.preventDefault();
        setSelectedClipId(filtered[0].id);
        if (listRef.current) listRef.current.scrollTop = 0;
        return;
      }
      const selectAt = (index: number) => {
        if (filtered[index]) {
          setSelectedClipId(filtered[index].id);
        }
      };

      if (e.key === "ArrowDown") {
        if (filtered.length === 0) return;
        e.preventDefault();
        if (selectedIndex >= filtered.length - 1) {
          // 已选到最后一条：自动加载下一页并选中新页第一条，
          // 否则键盘最多只能走到当前页第 50 条。
          if (hasMore && !loadingMore && !listLoading) {
            pendingSelectNext.current = true;
            loadClipPage(clipsRef.current.length, true, searchValueRef.current.trim(), filterValueRef.current, selectedTagRef.current);
          }
          return;
        }
        selectAt(selectedIndex < 0 ? 0 : selectedIndex + 1);
        return;
      }
      if (e.key === "ArrowUp") {
        if (filtered.length === 0) return;
        e.preventDefault();
        selectAt(selectedIndex < 0 ? 0 : Math.max(selectedIndex - 1, 0));
        return;
      }
      // P = 置顶 / 取消置顶（规格 4.2）
      if (e.key.toLowerCase() === "p" && !isInput && !e.ctrlKey && !e.altKey) {
        const targetClip = selectedClip || filtered[0];
        if (!targetClip) return;
        e.preventDefault();
        handlePin(targetClip.id);
        return;
      }
      // 对齐 Maccy 的 ⌘+数字:Ctrl+1~9 选中第 n 条并复制;Alt+1~9 复制并粘贴。
      // 只在带修饰键时生效,所以搜索框里的普通数字仍可正常输入。
      if (/^[1-9]$/.test(e.key) && (e.ctrlKey || e.altKey) && !e.metaKey) {
        const index = Number(e.key) - 1;
        const targetClip = filtered[index];
        if (!targetClip) return;
        e.preventDefault();
        setSelectedClipId(targetClip.id);
        if (e.altKey) {
          handleDoubleClick(targetClip.id);
        } else {
          handleCopy(targetClip.id);
        }
        return;
      }
      if (e.key === "Enter") {
        const targetClip = selectedClip || filtered[0];
        if (!targetClip) return;
        e.preventDefault();
        // 2026-10-29 使用者决定:回车与单击语义统一 = 复制并粘贴回原窗口。
        // 以前回车只复制(跟随 auto_paste 设置),看起来像"按了没反应"。
        // 只想复制时按 Ctrl+回车(与 Ctrl+1~9 的"只复制"一致)。
        if (e.ctrlKey) {
          handleCopy(targetClip.id);
        } else {
          handleDoubleClick(targetClip.id);
        }
        return;
      }
      if (e.key === "Delete" && !isInput) {
        const targetClip = selectedClip || filtered[0];
        if (!targetClip) return;
        e.preventDefault();
        requestDelete(targetClip.id);
        return;
      }
      if ((e.key === " " || e.key.toLowerCase() === "i") && !isInput) {
        const targetClip = selectedClip || filtered[0];
        if (!targetClip) return;
        e.preventDefault();
        handleOpenDetail(targetClip.id).catch(console.error);
      }
    };
    window.addEventListener("keydown", down);
    return () => {
      window.removeEventListener("keydown", down);
    };
  }, [closeDetail, detailOpen, filtered, handleCopy, handleDoubleClick, handleOpenDetail, handlePin, hasMore, listLoading, loadClipPage, loadingMore, openPanel, requestDelete, search, selectedClip, selectedIndex, showSettings]);

  const emptyTitle = search.trim() ? "没有匹配记录" : "暂无剪贴板记录";
  const emptyHint = search.trim()
    ? "换个关键词试试"
    : filter === "all"
      ? "复制任意内容即可记录"
      : `还没有${filter === "text" ? "文本" : "图片"}记录`;
  const visibleTags = selectedTag && !tagOptions.some((tag) => tag.name === selectedTag)
    ? [{ name: selectedTag, count: 0 }, ...tagOptions]
    : tagOptions;

  return (
    <div className="app">
      <header className="app-header">
        {showSettings ? (
          <span className="header-drag" />
        ) : (
        <div className="search-field">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="11" cy="11" r="7.5" /><line x1="21" y1="21" x2="16.5" y2="16.5" />
          </svg>
          <input
            ref={searchRef}
            type="text"
            className="search"
            placeholder="搜索剪贴板历史…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onContextMenu={openSearchMenu}
          />
        </div>
        )}
        {!showSettings && (
          <>
        <button
          className={`header-btn ${windowPinned ? "on" : ""}`}
          onClick={handleToggleWindowPin}
          title={windowPinned ? "已固定窗口 — 点击取消" : "固定窗口（复制后不隐藏）"}
          aria-label="固定窗口"
        >
          <PinIcon />
        </button>
        <button
          className={`header-btn ${openPanel === "help" ? "on" : ""}`}
          onClick={() => togglePanel("help")}
          title="快捷键与用法"
          aria-label="快捷键"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="9" />
            <path d="M9.6 9.2a2.5 2.5 0 1 1 3.5 2.3c-.7.4-1.1.9-1.1 1.8" />
            <circle cx="12" cy="17" r="0.7" fill="currentColor" />
          </svg>
        </button>
        <button className="header-btn" onClick={() => setShowSettings(true)} title="设置" aria-label="设置">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
            <circle cx="12" cy="12" r="3" />
          </svg>
        </button>
          </>
        )}
        <button className="header-btn danger" onClick={() => HideWindow()} title="关闭 (Esc)" aria-label="关闭">
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round">
            <line x1="3" y1="3" x2="9" y2="9" /><line x1="9" y1="3" x2="3" y2="9" />
          </svg>
        </button>
      </header>

      {showSettings ? (
        <Settings onBack={() => setShowSettings(false)} />
      ) : (
        <>
          <div className="toolbar-area" onMouseDown={(e) => e.stopPropagation()}>
            <div className="toolbar">
              <div className="segmented" role="tablist" aria-label="类型筛选">
                {([["all", "全部"], ["text", "文本"], ["image", "图片"]] as [ClipFilter, string][]).map(([value, label]) => (
                  <button
                    key={value}
                    role="tab"
                    aria-selected={filter === value}
                    className={`seg ${filter === value ? "active" : ""}`}
                    onClick={() => setFilter(value)}
                  >
                    {label}
                  </button>
                ))}
              </div>
              <button
                className={`toolbar-btn ${selectedTag ? "on" : ""}`}
                onClick={() => togglePanel("tags")}
                title="按标签筛选"
              >
                {selectedTag || "标签"}
                <svg width="10" height="10" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
                  <path d="M3 4.5l3 3 3-3" />
                </svg>
              </button>
              <span className="toolbar-spacer" />
              {(copyFeedback.message || clearMessage) && (
                <span className={`toolbar-feedback ${copyFeedback.error ? "error" : ""}`} title={copyFeedback.message || clearMessage}>
                  {copyFeedback.message || clearMessage}
                </span>
              )}
              <button
                className={`toolbar-btn ${openPanel === "more" ? "on" : ""}`}
                onClick={() => togglePanel("more")}
                title="更多"
                aria-label="更多"
              >
                <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
                  <circle cx="3" cy="8" r="1.4" /><circle cx="8" cy="8" r="1.4" /><circle cx="13" cy="8" r="1.4" />
                </svg>
              </button>
            </div>

            {openPanel === "tags" && (
              <div className="popover" role="dialog" aria-label="标签筛选">
                <div className="popover-title">
                  <span>按标签筛选</span>
                  {selectedTag && (
                    <button className="toolbar-btn" onClick={() => { setSelectedTag(""); setOpenPanel(null); }}>清除</button>
                  )}
                </div>
                {visibleTags.length === 0 ? (
                  <div className="empty-hint">还没有标签，详情里可以给记录加标签</div>
                ) : (
                  <div className="tag-chips">
                    <button
                      className={`tag-chip ${selectedTag === "" ? "active" : ""}`}
                      onClick={() => { setSelectedTag(""); setOpenPanel(null); }}
                    >
                      全部标签
                    </button>
                    {visibleTags.map((tag) => (
                      <button
                        key={tag.name}
                        className={`tag-chip ${selectedTag === tag.name ? "active" : ""}`}
                        onClick={() => { setSelectedTag((current) => current === tag.name ? "" : tag.name); setOpenPanel(null); }}
                        title={tag.count ? `${tag.name} (${tag.count})` : tag.name}
                      >
                        <span>{tag.name}</span>
                        {tag.count > 0 && <strong>{tag.count}</strong>}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}

            {openPanel === "help" && (
              <div className="popover" role="dialog" aria-label="快捷键">
                <div className="popover-title">
                  <span>快捷键</span>
                  <button className="toolbar-btn" onClick={() => setOpenPanel(null)}>关闭</button>
                </div>
                <div className="help-grid">
                  <span><kbd>{hotkeyDisplay}</kbd></span><span>唤起 / 隐藏 ClipBox</span>
                  <span><kbd>↑</kbd><kbd>↓</kbd></span><span>选择条目，<kbd>Home</kbd> 回到最新</span>
                  <span><kbd>Enter</kbd></span><span>复制并粘贴回原窗口</span>
                  <span><kbd>Ctrl</kbd><kbd>Enter</kbd></span><span>只复制</span>
                  <span><kbd>Ctrl</kbd><kbd>1</kbd>–<kbd>9</kbd></span><span>复制第 n 条</span>
                  <span><kbd>Alt</kbd><kbd>1</kbd>–<kbd>9</kbd></span><span>复制并粘贴第 n 条</span>
                  <span><kbd>Space</kbd></span><span>详情、备注与标签</span>
                  <span><kbd>P</kbd></span><span>置顶 / 取消置顶</span>
                  <span><kbd>Del</kbd></span><span>删除（再按一次确认）</span>
                  <span><kbd>Esc</kbd></span><span>关闭</span>
                </div>
              </div>
            )}

            {openPanel === "more" && (
              <div className="popover" role="dialog" aria-label="更多操作">
                <div className="menu-list">
                  <button
                    className={`menu-item danger ${clearConfirming ? "confirming" : ""}`}
                    onClick={handleClear}
                    disabled={clearing}
                  >
                    {clearing ? "清空中…" : clearConfirming ? "再次点击确认清空" : "清空未置顶记录"}
                  </button>
                </div>
              </div>
            )}
          </div>

          <div className="clip-list" ref={listRef} role="listbox" aria-label="剪贴板历史列表" onContextMenu={openItemMenu} onScroll={handleListScroll}>
            {listError && <div className="list-error" role="alert">{listError}</div>}
            {listLoading && clips.length === 0 && (
              <div className="empty">
                <span>加载中…</span>
              </div>
            )}
            {!listLoading && filtered.length === 0 && (
              <div className="empty">
                <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="9" y="9" width="13" height="13" rx="2" ry="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                </svg>
                <span>{emptyTitle}</span>
                <span className="empty-hint">{emptyHint}</span>
                {!search.trim() && filter === "all" && (
                  <span className="empty-hint">按 <kbd>{hotkeyDisplay}</kbd> 随时唤起，右上角 <kbd>?</kbd> 看全部快捷键</span>
                )}
              </div>
            )}
            {filtered.map((clip, index) => (
              <ClipItem
                key={clip.id}
                clip={clip}
                index={index}
                query={search}
                timeLabel={formatTime(clip.timestamp)}
                imageLabel={clip.type === "image" ? formatImageMeta(imageMeta[clip.id]) : undefined}
                selected={clip.id === selectedClipId}
                copyStatus={copyFeedback.clipId === clip.id ? copyFeedback : null}
                deleteConfirming={deleteConfirmId === clip.id}
                deleting={deleteDeletingId === clip.id}
                onHover={handleHover}
                onPin={handlePin}
                onDelete={requestDelete}
                onDouble={handleDoubleClick}
                onDetail={handleOpenDetail}
              />
            ))}
            {clips.length > 0 && (
              <div className="list-status">
                <span>已加载 {clips.length} / {totalClips}</span>
                {hasMore && (
                  <button className="load-more-btn" onClick={handleLoadMore} disabled={loadingMore || listLoading}>
                    {loadingMore ? "加载中…" : "加载更多"}
                  </button>
                )}
              </div>
            )}
          </div>
          {showBackToTop && (
            <button className="back-to-top" onClick={backToTop} title="回到最新（Home）">↑ 最新</button>
          )}
          {ctxMenu && (
            <div
              className="ctx-menu"
              style={{
                left: Math.min(ctxMenu.x, window.innerWidth - 170),
                top: Math.min(ctxMenu.y, window.innerHeight - 230),
              }}
              onMouseDown={(e) => e.stopPropagation()}
              onContextMenu={(e) => e.preventDefault()}
            >
              {ctxMenu.kind === "search" ? (
                <>
                  <button className="ctx-item" onClick={handlePasteSearch}>粘贴</button>
                  <button className="ctx-item" onClick={() => { searchRef.current?.select(); setCtxMenu(null); }}>全选</button>
                  <button className="ctx-item" onClick={() => { setSearch(""); searchRef.current?.focus(); setCtxMenu(null); }}>清空搜索</button>
                </>
              ) : (
                <>
                  <button className="ctx-item" onClick={() => { handleCopy(ctxMenu.clipId); setCtxMenu(null); }}>复制</button>
                  <button className="ctx-item" onClick={() => { handleDoubleClick(ctxMenu.clipId); setCtxMenu(null); }}>复制并粘贴</button>
                  <button className="ctx-item" onClick={() => { handlePin(ctxMenu.clipId); setCtxMenu(null); }}>置顶 / 取消置顶</button>
                  <button className="ctx-item" onClick={() => { handleOpenDetail(ctxMenu.clipId).catch(console.error); setCtxMenu(null); }}>详情</button>
                  <button className="ctx-item danger" onClick={() => { requestDelete(ctxMenu.clipId); setCtxMenu(null); }}>删除</button>
                </>
              )}
            </div>
          )}
          {detailOpen && (
            <ClipDetailPanel
              clip={detailClip}
              imageUrl={detailImageUrl}
              imageLoading={detailImageLoading}
              loading={detailLoading}
              error={detailError}
              onClose={closeDetail}
              onCopy={handleCopy}
              onSaveMetadata={handleSaveMetadata}
              onExport={handleExportClip}
            />
          )}
        </>
      )}
    </div>
  );
}

export default App;
