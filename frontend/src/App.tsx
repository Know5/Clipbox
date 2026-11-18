import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import { EventsOn } from "../wailsjs/runtime/runtime";
import "./App.css";
import {
  ClearAll,
  CopyAndPaste,
  CopyToClipboard,
  DeleteClip,
  FrontendReady,
  GetClipPageByTypeAndTag,
  GetClipTags,
  GetHotkeySettings,
  GetViewState,
  GetWindowPinned,
  HideWindow,
  ReadClipboardText,
  SaveViewState,
  SearchClipPageByTypeAndTag,
  TogglePin,
  ToggleWindowPin,
} from "./api";
import type { ClearResult, ClipEntry, ClipHistoryChange, ClipPage, ClipTag } from "./clipHistory";
import { applyHistoryChange, mergeClips } from "./clipHistory";
import { refreshAccent } from "./theme";
import { useImageMetadata } from "./useImageMetadata";
import { useClipDetail } from "./useClipDetail";
import { useClipShortcuts } from "./useClipShortcuts";
import { useThemeSync } from "./useThemeSync";

import ClipDetailPanel from "./ClipDetailPanel";
import type { CopyFeedback } from "./ClipItem";
import ClipItem, { PinIcon } from "./ClipItem";
import ContextMenu, { type CtxMenu } from "./ContextMenu";
import ToolbarPopovers from "./ToolbarPopovers";
import { formatTime } from "./clipFormat";

const Settings = lazy(() => import("./Settings"));
const TextToolsPanel = lazy(() => import("./TextToolsPanel"));

const PAGE_SIZE = 50;
function formatImageMeta(meta?: { width: number; height: number; bytes: number }) {
  if (!meta) return "";
  const size = meta.width && meta.height ? `${meta.width}×${meta.height}` : "";
  const bytes =
    meta.bytes < 1024
      ? `${meta.bytes} B`
      : meta.bytes < 1048576
        ? `${Math.round(meta.bytes / 1024)} KB`
        : `${(meta.bytes / 1048576).toFixed(1)} MB`;
  return [size, bytes].filter(Boolean).join(" · ");
}
type ClipFilter = "all" | "text" | "image";

function describeClearResult(result?: ClearResult) {
  const deleted = result?.deletedClips ?? 0;
  const pinned = result?.pinnedKept ?? result?.stats?.pinnedClips ?? 0;
  if (deleted > 0) return `已清空 ${deleted} 条，保留 ${pinned} 条置顶`;
  if (pinned > 0) return `没有可清空记录，保留 ${pinned} 条置顶`;
  return "没有可清空记录";
}

function App() {
  const [clips, setClips] = useState<ClipEntry[]>([]);
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<ClipFilter>("all");
  const [selectedTag, setSelectedTag] = useState("");
  const [tagOptions, setTagOptions] = useState<ClipTag[]>([]);
  const [tagsLoading, setTagsLoading] = useState(false);
  const [tagsError, setTagsError] = useState("");
  const [windowVisible, setWindowVisible] = useState(!document.hidden);
  const [showSettings, setShowSettings] = useState(false);
  const [settingsInitialTab, setSettingsInitialTab] = useState<"general" | "textTools">("general");
  const [settingsOpenSignal, setSettingsOpenSignal] = useState(0);
  const [textToolsClip, setTextToolsClip] = useState<ClipEntry | null>(null);
  const [openPanel, setOpenPanel] = useState<null | "tags" | "help" | "more">(null);
  const imageMeta = useImageMetadata(clips);
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
  const scrollSelectionIntoViewRef = useRef(false);
  const [deleteConfirmId, setDeleteConfirmId] = useState<number | null>(null);
  const [deleteDeletingId, setDeleteDeletingId] = useState<number | null>(null);
  const [copyFeedback, setCopyFeedback] = useState<CopyFeedback>({
    clipId: null,
    message: "",
    error: false,
  });
  const {
    detailClip,
    detailLoading,
    detailError,
    detailImageUrl,
    detailImageLoading,
    detailOpen,
    closeDetail,
    handleOpenDetail,
    handleSaveMetadata,
    handleExportClip,
    refreshDetailImageOnShow,
    releaseDetailOnHide,
    clearDetailIfDeleted,
  } = useClipDetail();
  const dialogOpenRef = useRef(false);
  dialogOpenRef.current = Boolean(detailOpen || textToolsClip);
  const [viewStateLoaded, setViewStateLoaded] = useState(false);
  const [historyRevision, setHistoryRevision] = useState(0);
  // 每分钟自增一次，触发相对时间标签（"x 分钟前"）重新计算
  const [, setTimeTick] = useState(0);
  const searchRef = useRef<HTMLInputElement>(null);
  const searchValueRef = useRef("");
  const filterValueRef = useRef<ClipFilter>("all");
  const selectedTagRef = useRef("");
  const clipsRef = useRef<ClipEntry[]>([]);
  const totalClipsRef = useRef(0);
  const requestSeq = useRef(0);
  const activePageRequest = useRef<{ append: boolean } | null>(null);
  const requestedCriteria = useRef({ query: "", clipFilter: "all" as ClipFilter, tag: "" });
  const clearConfirmTimer = useRef<number | null>(null);
  const clearMessageTimer = useRef<number | null>(null);
  const deleteConfirmTimer = useRef<number | null>(null);
  const copyFeedbackTimer = useRef<number | null>(null);

  const commitClips = useCallback(
    (update: ClipEntry[] | ((current: ClipEntry[]) => ClipEntry[])) => {
      const next = typeof update === "function" ? update(clipsRef.current) : update;
      clipsRef.current = next;
      setClips(next);
    },
    [],
  );

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
    copyFeedbackTimer.current = window.setTimeout(
      () => {
        setCopyFeedback({ clipId: null, message: "", error: false });
        copyFeedbackTimer.current = null;
      },
      error ? 3600 : 1800,
    );
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

  const selectClipByKeyboard = useCallback((id: number | null) => {
    if (id != null) scrollSelectionIntoViewRef.current = true;
    setSelectedClipId(id);
  }, []);

  const loadClipPage = useCallback(
    async (
      offset = 0,
      append = false,
      query = searchValueRef.current.trim(),
      clipFilter = filterValueRef.current,
      tag = selectedTagRef.current,
    ) => {
      const seq = requestSeq.current + 1;
      requestSeq.current = seq;
      activePageRequest.current = { append };
      requestedCriteria.current = { query, clipFilter, tag };
      if (append) {
        setLoadingMore(true);
      } else {
        setListLoading(true);
      }
      setListError("");
      try {
        const page: ClipPage = await (query
          ? SearchClipPageByTypeAndTag(query, clipFilter, tag, PAGE_SIZE, offset)
          : GetClipPageByTypeAndTag(clipFilter, tag, PAGE_SIZE, offset));
        if (seq !== requestSeq.current) return;
        const items = page?.items || [];
        commitClips((prev) => (append ? mergeClips(prev, items) : items));
        if (!append) {
          pendingSelectNext.current = false;
        } else if (pendingSelectNext.current && items.length > 0) {
          // 方向键翻页：选到下一页的第一条
          pendingSelectNext.current = false;
          selectClipByKeyboard(items[0].id);
        }
        totalClipsRef.current = page?.total || 0;
        setTotalClips(totalClipsRef.current);
        setHasMore(Boolean(page?.hasMore));
      } catch (e) {
        console.error(e);
        if (seq === requestSeq.current) {
          setListError("加载历史失败");
        }
      } finally {
        if (seq === requestSeq.current) {
          activePageRequest.current = null;
          setListLoading(false);
          setLoadingMore(false);
        }
      }
    },
    [commitClips, selectClipByKeyboard],
  );

  useEffect(() => {
    if (!viewStateLoaded) return;
    const timer = window.setTimeout(() => {
      SaveViewState(
        JSON.stringify({ search: search.slice(0, 2000), filter, tag: selectedTag }),
      ).catch(() => {});
    }, 120);
    return () => window.clearTimeout(timer);
  }, [viewStateLoaded, search, filter, selectedTag]);

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
    let active = true;
    let handledBeforeBoot = false;
    Promise.resolve()
      .then(GetViewState)
      .then((raw) => {
        if (!active) return;
        const userEdited =
          searchValueRef.current !== "" ||
          filterValueRef.current !== "all" ||
          selectedTagRef.current !== "";
        let nextSearch = searchValueRef.current;
        let nextFilter = filterValueRef.current;
        let nextTag = selectedTagRef.current;
        if (raw && !userEdited) {
          const state = JSON.parse(raw) as {
            search?: string;
            filter?: ClipFilter;
            tag?: string;
          };
          if (typeof state.search === "string") {
            nextSearch = state.search;
            searchValueRef.current = state.search;
            setSearch(state.search);
          }
          if (state.filter === "all" || state.filter === "text" || state.filter === "image") {
            nextFilter = state.filter;
            filterValueRef.current = state.filter;
            setFilter(state.filter);
          }
          if (typeof state.tag === "string") {
            nextTag = state.tag;
            selectedTagRef.current = state.tag;
            setSelectedTag(state.tag);
          }
        }
        const restoredCustom =
          !userEdited && Boolean(nextSearch.trim() || nextFilter !== "all" || nextTag);
        if (!handledBeforeBoot || restoredCustom) {
          handledBeforeBoot = true;
          loadClipPage(0, false, nextSearch.trim(), nextFilter, nextTag);
        }
      })
      .catch(() => {
        if (!active) return;
        if (!handledBeforeBoot) {
          handledBeforeBoot = true;
          loadClipPage(
            0,
            false,
            searchValueRef.current.trim(),
            filterValueRef.current,
            selectedTagRef.current,
          );
        }
      })
      .finally(() => {
        if (active) setViewStateLoaded(true);
      });
    const unsub1 = EventsOn("clips:updated", (change?: ClipHistoryChange) => {
      setHistoryRevision((rev) => rev + 1);
      if (!handledBeforeBoot) return;
      const activeSearch = searchValueRef.current.trim();
      const activeFilter = filterValueRef.current;
      const activeTag = selectedTagRef.current;
      if (
        !change ||
        !Number.isFinite(change.total) ||
        !Array.isArray(change.deletedIds) ||
        activeSearch ||
        activeTag ||
        activeFilter !== "all"
      ) {
        loadClipPage(0, false, activeSearch, activeFilter, activeTag);
        return;
      }
      if (
        change.entry &&
        !change.entry.pinned &&
        clipsRef.current.length < change.total &&
        clipsRef.current.some((clip) => clip.id === change.entry?.id && clip.pinned)
      ) {
        // Unpinning can move an old row past the loaded prefix. Fetch its new
        // boundary so the next offset does not skip the displaced record.
        loadClipPage(0, false, activeSearch, activeFilter, activeTag);
        return;
      }
      const pending = activePageRequest.current;
      // A response started before this update could resurrect deleted rows.
      // Restart it from the updated offset while retaining the loaded pages.
      requestSeq.current++;
      activePageRequest.current = null;
      setListLoading(false);
      setLoadingMore(false);
      const next = applyHistoryChange(clipsRef.current, change, PAGE_SIZE);
      commitClips(next.items);
      totalClipsRef.current = next.total;
      setTotalClips(next.total);
      setHasMore(next.hasMore);
      clearDetailIfDeleted(change.deletedIds);
      setTextToolsClip((current) =>
        current && change.deletedIds.includes(current.id) ? null : current,
      );
      if (pending) {
        loadClipPage(
          pending.append ? next.items.length : 0,
          pending.append,
          activeSearch,
          activeFilter,
          activeTag,
        );
      }
    });
    const unsub2 = EventsOn("window:shown", () => {
      setWindowVisible(true);
      if (!dialogOpenRef.current) searchRef.current?.focus();
      refreshDetailImageOnShow();
      refreshAccent().catch(console.error);
      if (activePageRequest.current?.append) {
        requestSeq.current++;
        activePageRequest.current = null;
        pendingSelectNext.current = false;
        setLoadingMore(false);
      }
      // 每次重新打开都回到最新一页，避免滚到底后加载的分页"常驻"成 100/500、200/500。
      if (clipsRef.current.length > PAGE_SIZE) {
        commitClips((prev) => prev.slice(0, PAGE_SIZE));
        setHasMore(totalClipsRef.current > PAGE_SIZE);
        setSelectedClipId(null);
      }
      if (listRef.current) listRef.current.scrollTop = 0;
      setShowBackToTop(false);
    });
    const unsub3 = EventsOn("clips:changed", () => {
      handledBeforeBoot = true;
      setHistoryRevision((rev) => rev + 1);
      const activeSearch = searchValueRef.current.trim();
      loadClipPage(0, false, activeSearch, filterValueRef.current, selectedTagRef.current);
    });
    const unsub4 = EventsOn("clip:skipped", (payload?: { message?: string }) => {
      const message = typeof payload?.message === "string" ? payload.message : "";
      if (message) showClearFeedback(message);
    });
    const unsub5 = EventsOn("settings:open", () => {
      setTextToolsClip(null);
      setSettingsInitialTab("general");
      setSettingsOpenSignal((signal) => signal + 1);
      setShowSettings(true);
    });
    const unsub6 = EventsOn("window:hidden", () => {
      setWindowVisible(false);
      setOpenPanel(null);
      releaseDetailOnHide();
      if (clipsRef.current.length > PAGE_SIZE) {
        commitClips((current) => current.slice(0, PAGE_SIZE));
        setHasMore(totalClipsRef.current > PAGE_SIZE);
      }
      if (activePageRequest.current?.append) {
        requestSeq.current++;
        activePageRequest.current = null;
        pendingSelectNext.current = false;
        setLoadingMore(false);
      }
    });
    const onFocus = () => {
      const active = document.activeElement;
      if (!active || active === document.body || active === document.documentElement)
        searchRef.current?.focus();
    };
    window.addEventListener("focus", onFocus);
    FrontendReady().catch(console.error);
    return () => {
      active = false;
      requestSeq.current++;
      activePageRequest.current = null;
      unsub1();
      unsub2();
      unsub3();
      unsub4();
      unsub5();
      unsub6();
      window.removeEventListener("focus", onFocus);
    };
  }, [
    clearDetailIfDeleted,
    commitClips,
    loadClipPage,
    refreshDetailImageOnShow,
    releaseDetailOnHide,
    showClearFeedback,
  ]);

  useEffect(() => {
    const query = search.trim();
    const matchesRequested = () =>
      requestedCriteria.current.query === query &&
      requestedCriteria.current.clipFilter === filter &&
      requestedCriteria.current.tag === selectedTag;
    if (matchesRequested()) return;
    const timer = setTimeout(() => {
      if (!matchesRequested()) loadClipPage(0, false, query, filter, selectedTag);
    }, 40);
    return () => clearTimeout(timer);
  }, [filter, search, selectedTag, loadClipPage]);

  useEffect(() => {
    GetHotkeySettings()
      .then((config) => {
        setHotkeyDisplay(config.display);
      })
      .catch(console.error);
  }, [showSettings]);

  useEffect(() => {
    GetWindowPinned().then(setWindowPinned).catch(console.error);
  }, []);

  const togglePanel = useCallback((panel: "tags" | "help" | "more") => {
    setOpenPanel((current) => (current === panel ? null : panel));
  }, []);

  // Tag aggregation is independent of list loading and runs only while its
  // dropdown is visible. Paging and background capture do not query all tags.
  useEffect(() => {
    if (openPanel !== "tags") return;
    let active = true;
    setTagsLoading(true);
    setTagsError("");
    const timer = window.setTimeout(() => {
      GetClipTags(search.trim(), filter)
        .then((tags) => {
          if (active) setTagOptions(tags || []);
        })
        .catch(() => {
          if (active) setTagsError("标签加载失败，请重新打开重试");
        })
        .finally(() => {
          if (active) setTagsLoading(false);
        });
    }, 40);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [openPanel, search, filter, historyRevision]);

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

  useEffect(() => {
    if (!windowVisible) return;
    const timer = window.setInterval(() => setTimeTick((tick) => tick + 1), 60_000);
    return () => window.clearInterval(timer);
  }, [windowVisible]);

  useThemeSync();

  const handleToggleWindowPin = async () => {
    try {
      const pinned = await ToggleWindowPin();
      setWindowPinned(pinned);
    } catch (e) {
      console.error(e);
    }
  };

  const handleCopy = useCallback(
    async (id: number) => {
      try {
        await CopyToClipboard(id);
        setListError("");
        showCopyFeedback(id, "已复制");
      } catch (e) {
        console.error(e);
        showCopyFeedback(
          id,
          typeof e === "string" && e.startsWith("已复制") ? e : "复制失败",
          true,
        );
      }
    },
    [showCopyFeedback],
  );

  const handleDoubleClick = useCallback(
    async (id: number) => {
      try {
        await CopyAndPaste(id);
        setListError("");
        showCopyFeedback(id, "已粘贴");
      } catch (e) {
        console.error(e);
        showCopyFeedback(id, "粘贴失败", true);
      }
    },
    [showCopyFeedback],
  );
  const handleHover = useCallback((id: number) => {
    setSelectedClipId(id);
  }, []);

  const handlePin = useCallback(async (id: number) => {
    try {
      await TogglePin(id);
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

  const handleDelete = useCallback(
    async (id: number) => {
      cancelDeleteConfirmTimer();
      setDeleteConfirmId(null);
      setDeleteDeletingId(id);
      setListError("");
      try {
        await DeleteClip(id);
        clearDetailIfDeleted([id]);
        setTextToolsClip((clip) => (clip?.id === id ? null : clip));
      } catch (e) {
        console.error(e);
        setListError("删除失败");
      } finally {
        setDeleteDeletingId((current) => (current === id ? null : current));
      }
    },
    [cancelDeleteConfirmTimer, clearDetailIfDeleted],
  );

  const requestDelete = useCallback(
    (id: number) => {
      if (deleteDeletingId === id) return;
      setListError("");

      if (deleteConfirmId === id) {
        handleDelete(id).catch(console.error);
        return;
      }

      cancelDeleteConfirmTimer();
      setDeleteConfirmId(id);
      deleteConfirmTimer.current = window.setTimeout(() => {
        setDeleteConfirmId((current) => (current === id ? null : current));
        deleteConfirmTimer.current = null;
      }, 3000);
    },
    [cancelDeleteConfirmTimer, deleteConfirmId, deleteDeletingId, handleDelete],
  );

  const handleSaveMetadataWithRevision = useCallback(
    async (id: number, note: string, tags: string) => {
      const updated = await handleSaveMetadata(id, note, tags);
      setHistoryRevision((rev) => rev + 1);
      return updated;
    },
    [handleSaveMetadata],
  );

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
      await loadClipPage(
        0,
        false,
        searchValueRef.current.trim(),
        filterValueRef.current,
        selectedTagRef.current,
      );
      setHistoryRevision((rev) => rev + 1);
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
      loadClipPage(
        clips.length,
        true,
        searchValueRef.current.trim(),
        filterValueRef.current,
        selectedTagRef.current,
      );
    }
  };

  // 滚动到底自动加载下一页（剩 240px 时提前触发），不用再点"加载更多"。
  const handleListScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const el = e.currentTarget;
    setShowBackToTop(el.scrollTop > 120);
    if (!hasMore || loadingMore || listLoading) return;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 240) {
      loadClipPage(
        clipsRef.current.length,
        true,
        searchValueRef.current.trim(),
        filterValueRef.current,
        selectedTagRef.current,
      );
    }
  };

  const backToTop = () => {
    if (listRef.current) listRef.current.scrollTop = 0;
    setShowBackToTop(false);
  };

  const filtered = clips;
  const selectedIndex =
    selectedClipId == null ? -1 : filtered.findIndex((clip) => clip.id === selectedClipId);
  const selectedClip = selectedIndex >= 0 ? filtered[selectedIndex] : null;

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
    if (selectedClipId == null || !scrollSelectionIntoViewRef.current) return;
    scrollSelectionIntoViewRef.current = false;
    const element = document.querySelector(`[data-clip-id="${selectedClipId}"]`);
    element?.scrollIntoView?.({ block: "nearest" });
  }, [selectedClipId]);

  const loadNextClipPage = useCallback(() => {
    pendingSelectNext.current = true;
    loadClipPage(
      clipsRef.current.length,
      true,
      searchValueRef.current.trim(),
      filterValueRef.current,
      selectedTagRef.current,
    );
  }, [loadClipPage]);
  const closeTextTools = useCallback(() => setTextToolsClip(null), []);
  const closeToolbarPanel = useCallback(() => setOpenPanel(null), []);
  const closeSettingsView = useCallback(() => setShowSettings(false), []);
  const clearSearchQuery = useCallback(() => setSearch(""), []);
  const hideAppWindow = useCallback(() => {
    HideWindow();
  }, []);
  const openClipDetailSafe = useCallback(
    (id: number) => {
      handleOpenDetail(id).catch(console.error);
    },
    [handleOpenDetail],
  );

  useClipShortcuts({
    filtered,
    selectedIndex,
    selectedClip,
    search,
    showSettings,
    detailOpen,
    hasTextTools: Boolean(textToolsClip),
    openPanel,
    hasMore,
    loadingMore,
    listLoading,
    searchRef,
    listRef,
    selectClipByKeyboard,
    onLoadNextPage: loadNextClipPage,
    onCloseTextTools: closeTextTools,
    onClosePanel: closeToolbarPanel,
    onCloseDetail: closeDetail,
    onCloseSettings: closeSettingsView,
    onClearSearch: clearSearchQuery,
    onHideWindow: hideAppWindow,
    onPin: handlePin,
    onCopy: handleCopy,
    onPaste: handleDoubleClick,
    onRequestDelete: requestDelete,
    onOpenDetail: openClipDetailSafe,
  });

  const emptyTitle = search.trim() ? "没有匹配记录" : "暂无剪贴板记录";
  const emptyHint = search.trim()
    ? "换个关键词试试"
    : filter === "all"
      ? "复制任意内容即可记录"
      : `还没有${filter === "text" ? "文本" : "图片"}记录`;
  const visibleTags =
    selectedTag && !tagOptions.some((tag) => tag.name === selectedTag)
      ? [{ name: selectedTag, count: 0 }, ...tagOptions]
      : tagOptions;

  return (
    <div className="app">
      <header className="app-header">
        {showSettings ? (
          <span className="header-drag" />
        ) : (
          <div className="search-field">
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <circle cx="11" cy="11" r="7.5" />
              <line x1="21" y1="21" x2="16.5" y2="16.5" />
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
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <circle cx="12" cy="12" r="9" />
                <path d="M9.6 9.2a2.5 2.5 0 1 1 3.5 2.3c-.7.4-1.1.9-1.1 1.8" />
                <circle cx="12" cy="17" r="0.7" fill="currentColor" />
              </svg>
            </button>
            <button
              className="header-btn"
              onClick={() => {
                setTextToolsClip(null);
                setSettingsInitialTab("general");
                setSettingsOpenSignal((signal) => signal + 1);
                setShowSettings(true);
              }}
              title="设置"
              aria-label="设置"
            >
              <svg
                width="15"
                height="15"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
                <circle cx="12" cy="12" r="3" />
              </svg>
            </button>
          </>
        )}
        <button
          className="header-btn danger"
          onClick={() => HideWindow()}
          title="关闭 (Esc)"
          aria-label="关闭"
        >
          <svg
            width="12"
            height="12"
            viewBox="0 0 12 12"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.4"
            strokeLinecap="round"
          >
            <line x1="3" y1="3" x2="9" y2="9" />
            <line x1="9" y1="3" x2="3" y2="9" />
          </svg>
        </button>
      </header>

      {showSettings ? (
        <Suspense fallback={<div className="detail-empty">加载设置…</div>}>
          <Settings
            initialTab={settingsInitialTab}
            openSignal={settingsOpenSignal}
            onBack={() => setShowSettings(false)}
          />
        </Suspense>
      ) : (
        <>
          <div className="toolbar-area" onMouseDown={(e) => e.stopPropagation()}>
            <div className="toolbar">
              <div className="segmented" role="tablist" aria-label="类型筛选">
                {(
                  [
                    ["all", "全部"],
                    ["text", "文本"],
                    ["image", "图片"],
                  ] as [ClipFilter, string][]
                ).map(([value, label]) => (
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
                <svg
                  width="10"
                  height="10"
                  viewBox="0 0 12 12"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                  strokeLinecap="round"
                >
                  <path d="M3 4.5l3 3 3-3" />
                </svg>
              </button>
              <span className="toolbar-spacer" />
              {(copyFeedback.message || clearMessage) && (
                <span
                  className={`toolbar-feedback ${copyFeedback.error ? "error" : ""}`}
                  title={copyFeedback.message || clearMessage}
                >
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
                  <circle cx="3" cy="8" r="1.4" />
                  <circle cx="8" cy="8" r="1.4" />
                  <circle cx="13" cy="8" r="1.4" />
                </svg>
              </button>
            </div>

            <ToolbarPopovers
              openPanel={openPanel}
              selectedTag={selectedTag}
              tagsLoading={tagsLoading}
              tagsError={tagsError}
              visibleTags={visibleTags}
              hotkeyDisplay={hotkeyDisplay}
              clearConfirming={clearConfirming}
              clearing={clearing}
              onSelectTag={setSelectedTag}
              onClose={() => setOpenPanel(null)}
              onClear={handleClear}
            />
          </div>

          <div
            className="clip-list"
            ref={listRef}
            role="listbox"
            aria-label="剪贴板历史列表"
            onContextMenu={openItemMenu}
            onScroll={handleListScroll}
          >
            {listError && (
              <div className="list-error" role="alert">
                {listError}
              </div>
            )}
            {listLoading && clips.length === 0 && (
              <div className="empty">
                <span>加载中…</span>
              </div>
            )}
            {!listLoading && filtered.length === 0 && (
              <div className="empty">
                <svg
                  width="32"
                  height="32"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                </svg>
                <span>{emptyTitle}</span>
                <span className="empty-hint">{emptyHint}</span>
                {!search.trim() && filter === "all" && (
                  <span className="empty-hint">
                    按 <kbd>{hotkeyDisplay}</kbd> 随时唤起，右上角 <kbd>?</kbd> 看全部快捷键
                  </span>
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
                <span>
                  已加载 {clips.length} / {totalClips}
                </span>
                {hasMore && (
                  <button
                    className="load-more-btn"
                    onClick={handleLoadMore}
                    disabled={loadingMore || listLoading}
                  >
                    {loadingMore ? "加载中…" : "加载更多"}
                  </button>
                )}
              </div>
            )}
          </div>
          {showBackToTop && (
            <button className="back-to-top" onClick={backToTop} title="回到最新（Home）">
              ↑ 最新
            </button>
          )}
          {ctxMenu && (
            <ContextMenu
              menu={ctxMenu}
              clips={clips}
              onClose={() => setCtxMenu(null)}
              onPasteSearch={handlePasteSearch}
              onSelectSearch={() => searchRef.current?.select()}
              onClearSearch={() => {
                setSearch("");
                searchRef.current?.focus();
              }}
              onCopy={handleCopy}
              onCopyAndPaste={handleDoubleClick}
              onPin={handlePin}
              onOpenDetail={(id) => {
                handleOpenDetail(id).catch(console.error);
              }}
              onOpenTextTools={(clip) => {
                closeDetail();
                setTextToolsClip(clip);
              }}
              onDelete={requestDelete}
            />
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
              onSaveMetadata={handleSaveMetadataWithRevision}
              onExport={handleExportClip}
              onTextTools={(clip) => {
                closeDetail();
                setTextToolsClip(clip);
              }}
            />
          )}
          {textToolsClip && (
            <Suspense
              fallback={
                <div className="detail-layer">
                  <div className="detail-empty">加载识别与翻译…</div>
                </div>
              }
            >
              <TextToolsPanel
                key={textToolsClip.id}
                clip={textToolsClip}
                onClose={() => setTextToolsClip(null)}
                onSettings={() => {
                  setTextToolsClip(null);
                  setSettingsInitialTab("textTools");
                  setSettingsOpenSignal((signal) => signal + 1);
                  setShowSettings(true);
                }}
              />
            </Suspense>
          )}
        </>
      )}
    </div>
  );
}

export default App;
