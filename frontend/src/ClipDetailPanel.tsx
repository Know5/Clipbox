import { useEffect, useRef, useState } from "react";
import type { ClipEntry } from "./clipHistory";
import { useDialogFocus } from "./useDialogFocus";
import { useUIActivity } from "./useUIActivity";
interface ClipExportResult {
  path?: string;
  type?: string;
  bytes?: number;
  cancelled?: boolean;
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

export default function ClipDetailPanel({
  clip,
  imageUrl,
  imageLoading,
  loading,
  error,
  onClose,
  onCopy,
  onSaveMetadata,
  onExport,
  onTextTools,
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
  onTextTools: (clip: ClipEntry) => void;
}) {
  const [note, setNote] = useState("");
  const panelRef = useRef<HTMLDivElement>(null);
  useDialogFocus(panelRef);
  const [tags, setTags] = useState("");
  const [saving, setSaving] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [saveMessage, setSaveMessage] = useState("");
  const [imageFailed, setImageFailed] = useState(false);

  useEffect(() => {
    setImageFailed(false);
  }, [imageUrl]);

  useEffect(() => {
    setNote(clip?.note || "");
    setTags(clip?.tags || "");
    setSaveMessage("");
  }, [clip?.id, clip?.note, clip?.tags]);

  const metadataChanged =
    Boolean(clip) && (note !== (clip?.note || "") || tags !== (clip?.tags || ""));
  useUIActivity(metadataChanged || saving || exporting);

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
      <div
        ref={panelRef}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-label={clip?.type === "image" ? "图片详情" : "文本详情"}
        className="detail-panel"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="detail-head">
          <div className="detail-title">
            <span className={`dot ${clip?.type || "text"}`} />
            <span>{clip?.type === "image" ? "图片详情" : "文本详情"}</span>
          </div>
          <button className="detail-close" onClick={onClose} title="关闭">
            <svg
              width="15"
              height="15"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
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
                <div>
                  <span>时间</span>
                  <strong>{formatDetailTime(clip.timestamp)}</strong>
                </div>
                {clip.sourceApp && (
                  <div>
                    <span>应用</span>
                    <strong title={clip.sourcePath || clip.sourceApp}>{clip.sourceApp}</strong>
                  </div>
                )}
                {clip.sourceTitle && (
                  <div>
                    <span>窗口</span>
                    <strong title={clip.sourceTitle}>{clip.sourceTitle}</strong>
                  </div>
                )}
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
                  {!imageLoading && imageUrl && !imageFailed && (
                    <img
                      src={imageUrl}
                      className="detail-image"
                      alt="clip-detail"
                      onError={() => setImageFailed(true)}
                    />
                  )}
                  {!imageLoading && (!imageUrl || imageFailed) && (
                    <div className="detail-empty">图片不可用</div>
                  )}
                </div>
              )}
              <div className="detail-text-tool-actions">
                <button className="btn-lite" onClick={() => onTextTools(clip)}>
                  {clip.type === "image" ? "识别文字 / 翻译" : "翻译文字"}
                </button>
              </div>
            </>
          )}
        </div>

        <div className="detail-actions">
          {saveMessage && (
            <span className={`detail-save-message ${saveMessage.endsWith("失败") ? "error" : ""}`}>
              {saveMessage}
            </span>
          )}
          <button className="btn-lite" onClick={onClose}>
            关闭
          </button>
          {clip && (
            <button className="btn-lite" onClick={handleSave} disabled={saving || !metadataChanged}>
              {saving ? "保存中..." : "保存"}
            </button>
          )}
          {clip && (
            <button className="btn-lite" onClick={handleExport} disabled={exporting}>
              {exporting ? "导出中..." : "导出"}
            </button>
          )}
          {clip && (
            <button className="btn-lite primary" onClick={() => onCopy(clip.id)}>
              复制
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
