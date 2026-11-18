import { useCallback, useRef, useState } from "react";
import { ExportClip, GetClipDetails, GetImageURL, UpdateClipMetadata } from "./api";
import type { ClipEntry, ClipExportResult } from "./clipHistory";

export function useClipDetail() {
  const [detailClip, setDetailClip] = useState<ClipEntry | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [detailImageUrl, setDetailImageUrl] = useState("");
  const [detailImageLoading, setDetailImageLoading] = useState(false);
  const detailRequestSeq = useRef(0);
  const detailClipRef = useRef<ClipEntry | null>(null);
  detailClipRef.current = detailClip;

  const closeDetail = useCallback(() => {
    detailRequestSeq.current++;
    setDetailClip(null);
    setDetailLoading(false);
    setDetailError("");
    setDetailImageUrl("");
    setDetailImageLoading(false);
  }, []);

  const handleOpenDetail = useCallback(async (id: number) => {
    const request = ++detailRequestSeq.current;
    setDetailClip(null);
    setDetailError("");
    setDetailImageUrl("");
    setDetailLoading(true);
    try {
      const detail = (await GetClipDetails(id)) as ClipEntry;
      if (request !== detailRequestSeq.current) return;
      setDetailClip(detail);
      if (detail.type === "image") {
        setDetailImageLoading(true);
        try {
          const url = await GetImageURL(id);
          if (request !== detailRequestSeq.current) return;
          setDetailImageUrl(url || "");
        } catch (e) {
          if (request !== detailRequestSeq.current) return;
          console.error(e);
          setDetailError("图片加载失败");
        } finally {
          if (request === detailRequestSeq.current) setDetailImageLoading(false);
        }
      }
    } catch (e) {
      if (request !== detailRequestSeq.current) return;
      console.error(e);
      setDetailError("加载详情失败");
    } finally {
      if (request === detailRequestSeq.current) setDetailLoading(false);
    }
  }, []);

  const handleSaveMetadata = useCallback(async (id: number, note: string, tags: string) => {
    const request = detailRequestSeq.current;
    const updated = (await UpdateClipMetadata(id, note, tags)) as ClipEntry;
    if (request === detailRequestSeq.current) {
      setDetailClip((current) => (current?.id === id ? updated : current));
    }
    return updated;
  }, []);

  const handleExportClip = useCallback(async (id: number) => {
    return (await ExportClip(id)) as ClipExportResult;
  }, []);

  const refreshDetailImageOnShow = useCallback(() => {
    const detail = detailClipRef.current;
    if (detail?.type !== "image") return;
    const request = ++detailRequestSeq.current;
    GetImageURL(detail.id)
      .then((url) => {
        if (request === detailRequestSeq.current) setDetailImageUrl(url || "");
      })
      .catch(() => {});
  }, []);

  const releaseDetailOnHide = useCallback(() => {
    detailRequestSeq.current++;
    setDetailLoading(false);
    setDetailImageUrl("");
    setDetailImageLoading(false);
  }, []);

  const clearDetailIfDeleted = useCallback((deletedIds: number[]) => {
    if (deletedIds.length === 0) return;
    setDetailClip((current) => (current && deletedIds.includes(current.id) ? null : current));
  }, []);

  const detailOpen = detailLoading || Boolean(detailClip) || Boolean(detailError);

  return {
    detailClip,
    setDetailClip,
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
  };
}
