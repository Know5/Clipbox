import { useEffect, useRef, useState } from "react";
import {
  CancelTextTask,
  CopyTextResult,
  GetClipDetails,
  GetTextToolsSettings,
  RecognizeClipText,
  TranslateText,
} from "./api";
import type { ClipEntry } from "./clipHistory";
import { TARGET_LANGUAGES, textToolError } from "./textTools";
import "./TextTools.css";
import { useDialogFocus } from "./useDialogFocus";
import { useUIActivity } from "./useUIActivity";

export default function TextToolsPanel({
  clip,
  onClose,
  onSettings,
}: {
  clip: ClipEntry;
  onClose: () => void;
  onSettings: () => void;
}) {
  const [source, setSource] = useState("");
  const panelRef = useRef<HTMLElement>(null);
  useDialogFocus(panelRef);
  const [result, setResult] = useState("");
  const [resultCopied, setResultCopied] = useState(false);
  const [target, setTarget] = useState("简体中文");
  const [ocrLanguage, setOCRLanguage] = useState("");
  const [ready, setReady] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<"ocr" | "translate" | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const activeTask = useRef("");
  const alive = useRef(true);
  const original = useRef("");
  useUIActivity(Boolean(busy) || (Boolean(result) && !resultCopied) || source !== original.current);

  useEffect(() => {
    let current = true;
    alive.current = true;
    Promise.all([
      GetTextToolsSettings(),
      clip.type === "text" ? GetClipDetails(clip.id) : Promise.resolve(null),
    ])
      .then(([settings, details]) => {
        if (!current) return;
        setReady(Boolean(settings.endpoint && settings.model));
        setTarget(settings.targetLanguage || "简体中文");
        setOCRLanguage(settings.ocrLanguage || "");
        original.current = details?.content || "";
        setSource(original.current);
      })
      .catch((e) => {
        if (current) setError(textToolError(e, "加载失败"));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
      alive.current = false;
      if (activeTask.current) CancelTextTask(activeTask.current).catch(() => {});
      activeTask.current = "";
    };
  }, [clip.id, clip.type]);

  const run = async (kind: "ocr" | "translate") => {
    if (activeTask.current || loading) return;
    const task = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    activeTask.current = task;
    setBusy(kind);
    setResultCopied(false);
    setError("");
    setNotice("");
    try {
      if (kind === "ocr") {
        const recognized = await RecognizeClipText(clip.id, ocrLanguage, task);
        if (!alive.current || activeTask.current !== task) return;
        setSource(recognized.text);
        setResult("");
        if (!recognized.text.trim()) setNotice("没有识别到文字，请检查截图清晰度或调整 OCR 语言");
      } else {
        setResult("");
        const translated = await TranslateText(source, target, task);
        if (!alive.current || activeTask.current !== task) return;
        setResult(translated);
      }
    } catch (e) {
      if (alive.current && activeTask.current === task) setError(textToolError(e, "处理失败"));
    } finally {
      if (activeTask.current === task) {
        activeTask.current = "";
        if (alive.current) setBusy(null);
      }
    }
  };

  const cancel = () => {
    const task = activeTask.current;
    activeTask.current = "";
    setBusy(null);
    setNotice("已取消");
    if (task) CancelTextTask(task).catch(() => {});
  };

  const copy = async (text: string) => {
    try {
      await CopyTextResult(text);
      if (alive.current) {
        original.current = source;
        if (text === result) setResultCopied(true);
        setNotice("已复制");
      }
    } catch (e) {
      if (alive.current) setError(textToolError(e, "复制失败"));
    }
  };

  return (
    <div className="detail-layer" onClick={onClose}>
      <section
        ref={panelRef}
        tabIndex={-1}
        className="detail-panel text-tools-panel"
        role="dialog"
        aria-modal="true"
        aria-label="识别与翻译"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="detail-head">
          <span className="detail-title">
            {clip.type === "image" ? "图片文字识别" : "文字翻译"}
          </span>
          <button className="detail-close" onClick={onClose} aria-label="关闭识别与翻译">
            ×
          </button>
        </div>
        <div className="detail-body text-tools-body">
          {loading && <p className="text-tools-hint">加载中…</p>}
          {clip.type === "image" && (
            <div className="text-tools-toolbar">
              <span className="text-tools-hint">
                Windows 本地 OCR · {ocrLanguage || "系统语言"}
              </span>
              <button
                className="btn-lite"
                disabled={loading || Boolean(busy)}
                onClick={() => run("ocr")}
              >
                {busy === "ocr" ? "识别中…" : source ? "重新识别" : "识别文字"}
              </button>
            </div>
          )}
          <label className="text-tools-field">
            <span>{clip.type === "image" ? "识别结果 · 可编辑" : "原文 · 可编辑"}</span>
            <textarea
              aria-label="待翻译原文"
              value={source}
              disabled={loading || Boolean(busy)}
              onChange={(e) => {
                setSource(e.target.value);
                setResult("");
              }}
              placeholder={
                clip.type === "image" ? "点击「识别文字」，也可以粘贴文字" : "要翻译的文字"
              }
            />
          </label>
          <div className="text-tools-toolbar">
            <select
              aria-label="目标语言"
              value={target}
              disabled={Boolean(busy)}
              onChange={(e) => {
                setTarget(e.target.value);
                setResult("");
              }}
            >
              {!TARGET_LANGUAGES.includes(target) && <option>{target}</option>}
              {TARGET_LANGUAGES.map((language) => (
                <option key={language}>{language}</option>
              ))}
            </select>
            <button
              className="btn-lite primary"
              disabled={loading || Boolean(busy) || !source.trim() || !ready}
              onClick={() => run("translate")}
            >
              {busy === "translate" ? "翻译中…" : "翻译"}
            </button>
            {busy && (
              <button className="btn-lite" onClick={cancel}>
                取消
              </button>
            )}
          </div>
          <p className="text-tools-hint">
            点击翻译后，原文会发送至你配置的服务。单次最多 20,000 字符。
          </p>
          {!loading && !ready && (
            <button className="text-tools-link" onClick={onSettings}>
              先配置翻译服务 →
            </button>
          )}
          {error && (
            <p className="text-tools-error" role="alert">
              {error}
            </p>
          )}
          {notice && (
            <p className="text-tools-hint" role="status">
              {notice}
            </p>
          )}
          {result && (
            <label className="text-tools-field">
              <span>译文</span>
              <textarea aria-label="译文" readOnly value={result} />
            </label>
          )}
        </div>
        <div className="detail-actions">
          <button className="btn-lite" onClick={onSettings}>
            设置
          </button>
          <button className="btn-lite" disabled={!source.trim()} onClick={() => copy(source)}>
            复制原文
          </button>
          <button
            className="btn-lite primary"
            disabled={!result.trim()}
            onClick={() => copy(result)}
          >
            复制译文
          </button>
        </div>
      </section>
    </div>
  );
}
