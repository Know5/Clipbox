import { useState, useEffect, useCallback, useRef, memo } from "react";
import { GetClipPageByTypeAndTag, SearchClipPageByTypeAndTag, GetClipTags, CopyToClipboard, CopyAndPaste, TogglePin, DeleteClip, ClearAll, GetHotkeySettings, GetAppSettings, HideWindow, ToggleWindowPin, GetWindowPinned, GetImageDataURL, GetClipDetails, UpdateClipMetadata, ExportClip } from "../wailsjs/go/main/App";
import { EventsOn, WindowMinimise } from "../wailsjs/runtime/runtime";
import Settings from "./Settings";
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
type ClipFilter = "all" | "text" | "image";

function describeClearResult(result?: ClearResult) {
  const deleted = result?.deletedClips ?? 0;
  const pinned = result?.pinnedKept ?? result?.stats?.pinnedClips ?? 0;
  if (deleted > 0) return `已清空 ${deleted} 条，保留 ${pinned} 条置顶`;
  if (pinned > 0) return `没有可清空记录，保留 ${pinned} 条置顶`;
  return "没有可清空记录";
}

function clipSourceLabel(clip: ClipEntry) {
  return (clip.sourceApp || "").trim();
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

/** Click-to-load image: never auto-loads. User clicks placeholder to view,
 *  clicks again on the loaded image to unload it (free ~10+ MB browser memory). */
function ClickToLoadImage({ clipId, thumbnail }: { clipId: number; thumbnail?: string }) {  const [dataUrl, setDataUrl] = useState("");
  const [loading, setLoading] = useState(false);

  const handleToggle = async (e: React.MouseEvent) => {
    e.stopPropagation(); // Don't trigger the parent clip-item's copy action.
    if (dataUrl) {
      // Unload the image to free browser memory.
      setDataUrl("");
      return;
    }
    setLoading(true);
    try {
      const url = await GetImageDataURL(clipId);
      setDataUrl(url || "");
    } catch {
      setDataUrl("");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="clip-img-container" onClick={handleToggle}>
      {dataUrl ? (
        <>
          <img src={dataUrl} className="clip-img" alt="clip-img" />
          <span className="img-unload-hint">点击收起图片</span>
        </>
      ) : (
        thumbnail ? (
          <div className="clip-thumbnail-preview">
            <img src={thumbnail} className="clip-img thumbnail" alt="clip-thumbnail" />
            <span className="img-unload-hint">{loading ? "加载中..." : "点击查看原图"}</span>
          </div>
        ) : (
          <div className="clip-img-placeholder clickable">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/>
            </svg>
            <span>{loading ? "加载中..." : "点击查看图片"}</span>
          </div>
        )
      )}
    </div>
  );
}

interface ClipItemProps {
  clip: ClipEntry;
  timeLabel: string;
  selected: boolean;
  copyStatus: CopyFeedback | null;
  deleteConfirming: boolean;
  deleting: boolean;
  onHover: (id: number) => void;
  onCopy: (id: number) => void;
  onDouble: (id: number) => void;
  onPin: (id: number) => void;
  onDelete: (id: number) => void;
  onDetail: (id: number) => void;
}

/** 列表项。memo 化后，悬停/选中/反馈只会重渲染受影响的少数几项，
 *  而不是整个列表（长列表滑动卡顿的主要来源）。 */
const ClipItem = memo(function ClipItem({
  clip,
  timeLabel,
  selected,
  copyStatus,
  deleteConfirming,
  deleting,
  onHover,
  onCopy,
  onDouble,
  onPin,
  onDelete,
  onDetail,
}: ClipItemProps) {
  const clickTimer = useRef<number>(0);

  const handleClick = () => {
    if (clickTimer.current) {
      window.clearTimeout(clickTimer.current);
      clickTimer.current = 0;
      onDouble(clip.id);
      return;
    }
    clickTimer.current = window.setTimeout(() => {
      clickTimer.current = 0;
      onCopy(clip.id);
    }, 300);
  };

  const sourceLabel = clipSourceLabel(clip);
  return (
    <div
      data-clip-id={clip.id}
      className={`clip-item ${clip.pinned ? "pinned" : ""} ${selected ? "selected" : ""} ${copyStatus ? (copyStatus.error ? "copy-error" : "copied") : ""} ${deleteConfirming ? "delete-confirming" : ""}`}
      onMouseEnter={() => onHover(clip.id)}
      onClick={handleClick}
    >
      <div className="clip-head">
        <span className={`dot ${clip.type}`} />
        <div className="clip-meta">
          <span className="clip-time">{timeLabel}</span>
          {sourceLabel && <span className="clip-source" title={clipSourceTitle(clip)}>{sourceLabel}</span>}
          {clip.tags && <span className="clip-tags" title={clip.tags}>{clip.tags}</span>}
        </div>
        {copyStatus && (
          <span className={`clip-copy-status ${copyStatus.error ? "error" : ""}`}>
            {copyStatus.message}
          </span>
        )}
        {(deleteConfirming || deleting) && (
          <span className="clip-delete-status">
            {deleting ? "删除中..." : "再点删除"}
          </span>
        )}
        <div className="clip-actions">
          <button
            className="act"
            onClick={(e) => { e.stopPropagation(); onDetail(clip.id); }}
            title="查看详情"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="3" />
              <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
            </svg>
          </button>
          <button
            className={`act ${clip.pinned ? "on" : ""}`}
            onClick={(e) => { e.stopPropagation(); onPin(clip.id); }}
            title={clip.pinned ? "取消置顶" : "置顶"}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill={clip.pinned ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="12" y1="17" x2="12" y2="22"/><path d="M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z"/>
            </svg>
          </button>
          <button
            className={`act danger ${deleteConfirming ? "confirming" : ""}`}
            onClick={(e) => { e.stopPropagation(); onDelete(clip.id); }}
            disabled={deleting}
            title={deleteConfirming ? "再次点击删除" : "删除"}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
          </button>
        </div>
      </div>
      <div className="clip-body">
        {clip.type === "text" ? (
          <span className="clip-text">{clip.preview}</span>
        ) : (
          <ClickToLoadImage clipId={clip.id} thumbnail={clip.thumbnail} />
        )}
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
    });
    const unsub3 = EventsOn("clips:changed", () => {
      const activeSearch = searchValueRef.current.trim();
      loadClipPage(0, false, activeSearch, filterValueRef.current, selectedTagRef.current);
    });
    const unsub4 = EventsOn("settings:open", () => {
      setShowSettings(true);
    });
    return () => { unsub1(); unsub2(); unsub3(); unsub4(); };
  }, [loadClipPage]);

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
      const selectAt = (index: number) => {
        if (filtered[index]) {
          setSelectedClipId(filtered[index].id);
        }
      };

      if (e.key === "ArrowDown") {
        if (filtered.length === 0) return;
        e.preventDefault();
        selectAt(selectedIndex < 0 ? 0 : Math.min(selectedIndex + 1, filtered.length - 1));
        return;
      }
      if (e.key === "ArrowUp") {
        if (filtered.length === 0) return;
        e.preventDefault();
        selectAt(selectedIndex < 0 ? 0 : Math.max(selectedIndex - 1, 0));
        return;
      }
      if (e.key === "Enter") {
        const targetClip = selectedClip || filtered[0];
        if (!targetClip) return;
        e.preventDefault();
        handleCopy(targetClip.id);
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
  }, [closeDetail, detailOpen, filtered, handleCopy, handleOpenDetail, requestDelete, search, selectedClip, selectedIndex, showSettings]);

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
      <div className="title-bar">
        <div className="traffic-lights">
          <button className="traffic-light close" onClick={() => HideWindow()} title="关闭" />
          <button className="traffic-light minimize" onClick={() => WindowMinimise()} title="最小化" />
        </div>
        <span className="title-text">ClipBox</span>
        <button className="settings-btn" onClick={() => setShowSettings(true)} title="设置">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/>
            <circle cx="12" cy="12" r="3"/>
          </svg>
        </button>
      </div>

      {showSettings ? (
        <Settings onBack={() => setShowSettings(false)} />
      ) : (
      <>
      <div className="search-bar">
        <svg className="search-icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
        </svg>
        <input
          ref={searchRef}
          type="text"
          className="search"
          placeholder="搜索剪贴板历史..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      <div className="filter-row">
        <button className={`chip ${filter === "all" ? "active" : ""}`} onClick={() => setFilter("all")}>全部</button>
        <button className={`chip ${filter === "text" ? "active" : ""}`} onClick={() => setFilter("text")}>文本</button>
        <button className={`chip ${filter === "image" ? "active" : ""}`} onClick={() => setFilter("image")}>图片</button>
        <span className="spacer" />
        {copyFeedback.message && <span className={`copy-feedback ${copyFeedback.error ? "error" : ""}`} title={copyFeedback.message}>{copyFeedback.message}</span>}
        {clearMessage && <span className="clear-feedback" title={clearMessage}>{clearMessage}</span>}
        <button
          className={`chip danger ${clearConfirming ? "confirming" : ""}`}
          onClick={handleClear}
          disabled={clearing}
          title={clearConfirming ? "再次点击清空未置顶记录" : "清空未置顶记录"}
        >
          {clearing ? "清空中..." : clearConfirming ? "确认清空" : "清空"}
        </button>
      </div>
      {(visibleTags.length > 0 || selectedTag) && (
        <div
          className="tag-row"
          onWheel={(e) => {
            // 标签溢出时支持鼠标滚轮横向滚动
            if (e.deltaY !== 0) e.currentTarget.scrollLeft += e.deltaY;
          }}
        >
          <button className={`tag-chip ${selectedTag === "" ? "active" : ""}`} onClick={() => setSelectedTag("")}>全部标签</button>
          {visibleTags.map((tag) => (
            <button
              key={tag.name}
              className={`tag-chip ${selectedTag === tag.name ? "active" : ""}`}
              onClick={() => setSelectedTag((current) => current === tag.name ? "" : tag.name)}
              title={tag.count ? `${tag.name} (${tag.count})` : tag.name}
            >
              <span>{tag.name}</span>
              {tag.count > 0 && <strong>{tag.count}</strong>}
            </button>
          ))}
        </div>
      )}

      <div className="clip-list">
        {listError && <div className="list-error">{listError}</div>}
        {listLoading && clips.length === 0 && (
          <div className="empty">
            <span>加载中...</span>
          </div>
        )}
        {!listLoading && filtered.length === 0 && (
          <div className="empty">
            <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
            </svg>
            <span>{emptyTitle}</span>
            <span className="empty-hint">{emptyHint}</span>
            {!search.trim() && filter === "all" && (
              <span className="empty-hint">按 <kbd>{hotkeyDisplay}</kbd> 随时唤起 ClipBox</span>
            )}
          </div>
        )}
        {filtered.map((clip) => (
          <ClipItem
            key={clip.id}
            clip={clip}
            timeLabel={formatTime(clip.timestamp)}
            selected={clip.id === selectedClipId}
            copyStatus={copyFeedback.clipId === clip.id ? copyFeedback : null}
            deleteConfirming={deleteConfirmId === clip.id}
            deleting={deleteDeletingId === clip.id}
            onHover={handleHover}
            onCopy={handleCopy}
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
                {loadingMore ? "加载中..." : "加载更多"}
              </button>
            )}
          </div>
        )}
      </div>
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

      <div className="footer">
        <div className="kbd-group">
          <kbd>↑↓</kbd> 选择
        </div>
        <div className="kbd-group">
          <kbd>↵</kbd> 粘贴
        </div>
        <div className="kbd-group">
          <kbd>Space</kbd> 详情
        </div>
        <div className="kbd-group">
          <kbd>Del</kbd> 删除
        </div>
        <div className="kbd-group">
          <kbd>Esc</kbd> 关闭
        </div>
        <button
          className={`window-pin-btn ${windowPinned ? "pinned" : ""}`}
          onClick={handleToggleWindowPin}
          title={windowPinned ? "已固定 — 点击取消固定" : "未固定 — 点击固定窗口"}
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill={windowPinned ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <line x1="12" y1="17" x2="12" y2="22"/>
            <path d="M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z"/>
          </svg>
        </button>
      </div>
      </>
      )}
    </div>
  );
}

export default App;
