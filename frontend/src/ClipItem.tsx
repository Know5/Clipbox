import { memo, useEffect, useState } from "react";
import { GetSourceIcon } from "./api";
import type { ClipEntry } from "./clipHistory";
export interface CopyFeedback {
  clipId: number | null;
  message: string;
  error: boolean;
}
function clipSourceTitle(clip: ClipEntry) {
  return [clip.sourceApp, clip.sourceTitle]
    .map((value) => (value || "").trim())
    .filter(Boolean)
    .join(" · ");
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
      {parts.map((part, index) =>
        index % 2 === 1 ? (
          <strong className="hl" key={index}>
            {part}
          </strong>
        ) : (
          part
        ),
      )}
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
      if (sourceIconCache.size > 64)
        sourceIconCache.delete(sourceIconCache.keys().next().value as string);
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
      <svg
        className="clip-type"
        viewBox="0 0 16 16"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.2"
      >
        <rect x="2" y="2.5" width="12" height="11" rx="1.5" />
        <circle cx="5.6" cy="6" r="1" />
        <path d="M3 12l3.2-3.4 2.4 2.2L11 8l2.2 4" />
      </svg>
    );
  }
  return (
    <svg
      className="clip-type"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.2"
    >
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
  return <img className="clip-source-icon" src={src} alt="" loading="lazy" decoding="async" />;
}

export function PinIcon({ className }: { className?: string }) {
  return (
    <svg className={className} width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
      <path d="M15.6 3H8.4v2.1l1.1.9v4.5L7 13.1V15h4.6v6h.8v-6H17v-1.9l-2.5-2.6V6l1.1-.9z" />
    </svg>
  );
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

  const status = copyStatus
    ? copyStatus.message
    : deleteConfirming || deleting
      ? deleting
        ? "删除中…"
        : "再点删除"
      : "";
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
        <img className="clip-thumb" src={clip.thumbnail} alt="" loading="lazy" decoding="async" />
      ) : (
        <SourceIcon path={clip.sourcePath} type={clip.type} />
      )}
      <span className="clip-preview">
        <Highlight text={previewText} query={query} />
      </span>
      {clip.pinned && <PinIcon className="clip-pin" />}
      <span className={`clip-meta ${copyStatus?.error || deleteConfirming ? "error" : ""}`}>
        {status || timeLabel}
      </span>
      <div className="clip-actions">
        <button
          className="act"
          onClick={(e) => {
            e.stopPropagation();
            onDetail(clip.id);
          }}
          title="查看详情 (Space)"
        >
          <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <circle cx="12" cy="12" r="3" />
            <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
          </svg>
        </button>
        <button
          className={`act ${clip.pinned ? "on" : ""}`}
          onClick={(e) => {
            e.stopPropagation();
            onPin(clip.id);
          }}
          title={clip.pinned ? "取消置顶 (P)" : "置顶 (P)"}
        >
          <PinIcon />
        </button>
        <button
          className={`act danger ${deleteConfirming ? "confirming" : ""}`}
          onClick={(e) => {
            e.stopPropagation();
            onDelete(clip.id);
          }}
          disabled={deleting}
          title={deleteConfirming ? "再次点击删除" : "删除 (Del)"}
        >
          <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <polyline points="3 6 5 6 21 6" />
            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
          </svg>
        </button>
      </div>
    </div>
  );
});

export default ClipItem;
