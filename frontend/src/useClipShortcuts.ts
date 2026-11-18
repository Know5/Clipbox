import { useEffect, type RefObject } from "react";
import type { ClipEntry } from "./clipHistory";

interface UseClipShortcutsOptions {
  filtered: ClipEntry[];
  selectedIndex: number;
  selectedClip: ClipEntry | null;
  search: string;
  showSettings: boolean;
  detailOpen: boolean;
  hasTextTools: boolean;
  openPanel: "tags" | "help" | "more" | null;
  hasMore: boolean;
  loadingMore: boolean;
  listLoading: boolean;
  searchRef: RefObject<HTMLInputElement | null>;
  listRef: RefObject<HTMLDivElement | null>;
  selectClipByKeyboard: (id: number | null) => void;
  onLoadNextPage: () => void;
  onCloseTextTools: () => void;
  onClosePanel: () => void;
  onCloseDetail: () => void;
  onCloseSettings: () => void;
  onClearSearch: () => void;
  onHideWindow: () => void;
  onPin: (id: number) => void;
  onCopy: (id: number) => void;
  onPaste: (id: number) => void;
  onRequestDelete: (id: number) => void;
  onOpenDetail: (id: number) => void;
}

/** Handles global Escape hierarchy and list keyboard shortcuts. */
export function useClipShortcuts({
  filtered,
  selectedIndex,
  selectedClip,
  search,
  showSettings,
  detailOpen,
  hasTextTools,
  openPanel,
  hasMore,
  loadingMore,
  listLoading,
  searchRef,
  listRef,
  selectClipByKeyboard,
  onLoadNextPage,
  onCloseTextTools,
  onClosePanel,
  onCloseDetail,
  onCloseSettings,
  onClearSearch,
  onHideWindow,
  onPin,
  onCopy,
  onPaste,
  onRequestDelete,
  onOpenDetail,
}: UseClipShortcutsOptions) {
  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.isComposing) return;
      if (e.key === "Escape") {
        if (hasTextTools) {
          onCloseTextTools();
          return;
        }
        if (openPanel) {
          onClosePanel();
          return;
        }
        if (detailOpen) {
          onCloseDetail();
          return;
        }
        if (showSettings) {
          // Editing settings input: blur first to save, then Esc again to return.
          const active = document.activeElement as HTMLElement | null;
          if (active && (active.tagName === "INPUT" || active.tagName === "TEXTAREA")) {
            active.blur();
            return;
          }
          onCloseSettings();
          return;
        }
        if (search.trim()) {
          onClearSearch();
          return;
        }
        onHideWindow();
        return;
      }
      if (showSettings || hasTextTools || detailOpen) return;

      const target = e.target as HTMLElement | null;
      const isInput =
        target?.tagName === "INPUT" ||
        target?.tagName === "TEXTAREA" ||
        Boolean(target?.isContentEditable);
      const isSearchInput = target === searchRef.current;
      if (isInput && !isSearchInput) return;

      if (e.key === "Home" && !isInput) {
        if (filtered.length === 0) return;
        e.preventDefault();
        selectClipByKeyboard(filtered[0].id);
        if (listRef.current) listRef.current.scrollTop = 0;
        return;
      }
      const selectAt = (index: number) => {
        if (filtered[index]) {
          selectClipByKeyboard(filtered[index].id);
        }
      };

      if (e.key === "ArrowDown") {
        if (filtered.length === 0) return;
        e.preventDefault();
        if (selectedIndex >= filtered.length - 1) {
          if (hasMore && !loadingMore && !listLoading) {
            onLoadNextPage();
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
      if (e.key.toLowerCase() === "p" && !isInput && !e.ctrlKey && !e.altKey) {
        const targetClip = selectedClip || filtered[0];
        if (!targetClip) return;
        e.preventDefault();
        onPin(targetClip.id);
        return;
      }
      if (/^[1-9]$/.test(e.key) && (e.ctrlKey || e.altKey) && !e.metaKey) {
        const index = Number(e.key) - 1;
        const targetClip = filtered[index];
        if (!targetClip) return;
        e.preventDefault();
        selectClipByKeyboard(targetClip.id);
        if (e.altKey) {
          onPaste(targetClip.id);
        } else {
          onCopy(targetClip.id);
        }
        return;
      }
      if (e.key === "Enter") {
        const targetClip = selectedClip || filtered[0];
        if (!targetClip) return;
        e.preventDefault();
        if (e.ctrlKey) {
          onCopy(targetClip.id);
        } else {
          onPaste(targetClip.id);
        }
        return;
      }
      if (e.key === "Delete" && !isInput) {
        const targetClip = selectedClip || filtered[0];
        if (!targetClip) return;
        e.preventDefault();
        onRequestDelete(targetClip.id);
        return;
      }
      if ((e.key === " " || e.key.toLowerCase() === "i") && !isInput) {
        const targetClip = selectedClip || filtered[0];
        if (!targetClip) return;
        e.preventDefault();
        onOpenDetail(targetClip.id);
      }
    };
    window.addEventListener("keydown", down);
    return () => {
      window.removeEventListener("keydown", down);
    };
  }, [
    detailOpen,
    filtered,
    hasMore,
    hasTextTools,
    listLoading,
    listRef,
    loadingMore,
    onClearSearch,
    onCloseDetail,
    onClosePanel,
    onCloseSettings,
    onCloseTextTools,
    onCopy,
    onHideWindow,
    onLoadNextPage,
    onOpenDetail,
    onPaste,
    onPin,
    onRequestDelete,
    openPanel,
    search,
    searchRef,
    selectClipByKeyboard,
    selectedClip,
    selectedIndex,
    showSettings,
  ]);
}
