import { useEffect, useState } from "react";
import { GetOCRLanguages, GetTextToolsSettings, SaveTextToolsSettings } from "./api";
import { TARGET_LANGUAGES, textToolError } from "./textTools";
import "./TextTools.css";
import { useUIActivity } from "./useUIActivity";

export default function TextToolsSettings() {
  const [endpoint, setEndpoint] = useState("");
  const [model, setModel] = useState("");
  const [target, setTarget] = useState("简体中文");
  const [ocrLanguage, setOCRLanguage] = useState("");
  const [languages, setLanguages] = useState<{ tag: string; name: string }[]>([]);
  const [languagesMessage, setLanguagesMessage] = useState("正在检查系统 OCR 语言…");
  const [key, setKey] = useState("");
  const [hasKey, setHasKey] = useState(false);
  const [clearKey, setClearKey] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [dirty, setDirty] = useState(false);
  useUIActivity(dirty || saving);

  useEffect(() => {
    let live = true;
    GetTextToolsSettings()
      .then((settings) => {
        if (!live) return;
        setEndpoint(settings.endpoint);
        setModel(settings.model);
        setTarget(settings.targetLanguage || "简体中文");
        setOCRLanguage(settings.ocrLanguage);
        setHasKey(settings.hasAPIKey);
        setLoaded(true);
      })
      .catch((e) => {
        if (live) setMessage(textToolError(e, "加载设置失败"));
      });
    GetOCRLanguages()
      .then((items) => {
        if (!live) return;
        setLanguages(items || []);
        setLanguagesMessage(
          items?.length
            ? `可用语言：${items.map((item) => item.name).join("、")}`
            : "没有可用的 OCR 语言，请在 Windows 设置 → 语言和区域中安装文字识别组件",
        );
      })
      .catch((e) => {
        if (live) setLanguagesMessage(textToolError(e, "无法检查 OCR 语言"));
      });
    return () => {
      live = false;
    };
  }, []);

  const save = async () => {
    if (!loaded || saving) return;
    setSaving(true);
    setMessage("");
    try {
      const settings = await SaveTextToolsSettings({
        endpoint,
        model,
        targetLanguage: target,
        ocrLanguage,
        apiKey: key,
        hasAPIKey: hasKey,
        clearAPIKey: clearKey,
      });
      setEndpoint(settings.endpoint);
      setModel(settings.model);
      setHasKey(settings.hasAPIKey);
      setKey("");
      setClearKey(false);
      setMessage("已保存");
      setDirty(false);
    } catch (e) {
      setMessage(textToolError(e, "保存失败"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <div className="settings-section">
        <div className="section-label">本地文字识别</div>
        <div className="section-card text-tools-form" onChange={() => setDirty(true)}>
          <p className="text-tools-hint">
            右键图片 → 识别文字。使用 Windows OCR，无需 Key，图片保留在本机。
          </p>
          <label className="text-tools-field">
            <span>识别语言</span>
            <select
              value={ocrLanguage}
              onChange={(e) => setOCRLanguage(e.target.value)}
              disabled={!loaded || saving}
            >
              <option value="">跟随系统语言</option>
              {ocrLanguage && !languages.some((item) => item.tag === ocrLanguage) && (
                <option value={ocrLanguage}>{ocrLanguage}（未检测到）</option>
              )}
              {languages.map((item) => (
                <option key={item.tag} value={item.tag}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <p className="text-tools-hint">{languagesMessage}</p>
          <p className="text-tools-hint">
            识别英文、日文等文本时，请安装对应语言的 Windows OCR
            组件并在上方选择；只安装中文组件会降低其他语言的识别率。
          </p>
        </div>
      </div>
      <div className="settings-section">
        <div className="section-label">手动翻译</div>
        <div className="section-card text-tools-form" onChange={() => setDirty(true)}>
          <p className="text-tools-hint">
            支持 Chat Completions 兼容接口。由你点击翻译后发送原文，费用由所填服务收取。
          </p>
          <label className="text-tools-field">
            <span>接口地址</span>
            <input
              className="url-input"
              value={endpoint}
              disabled={!loaded || saving}
              maxLength={1000}
              placeholder="服务商提供的 API 地址，通常以 /v1 结尾"
              onChange={(e) => setEndpoint(e.target.value)}
            />
          </label>
          <label className="text-tools-field">
            <span>模型名称</span>
            <input
              className="url-input"
              value={model}
              disabled={!loaded || saving}
              maxLength={200}
              placeholder="填写服务商提供的模型 ID"
              onChange={(e) => setModel(e.target.value)}
            />
          </label>
          <label className="text-tools-field">
            <span>API Key {hasKey && !clearKey ? "· 已保存" : ""}</span>
            <input
              className="url-input"
              type="password"
              autoComplete="off"
              spellCheck={false}
              value={key}
              disabled={!loaded || saving}
              maxLength={4096}
              placeholder={
                clearKey
                  ? "保存后移除现有 Key"
                  : hasKey
                    ? "留空保留现有 Key，填写以替换"
                    : "填写你的 Key；本机无鉴权服务可留空"
              }
              onChange={(e) => {
                setKey(e.target.value);
                setClearKey(false);
              }}
            />
          </label>
          {hasKey && (
            <label className="text-tools-hint">
              <input
                type="checkbox"
                checked={clearKey}
                disabled={saving}
                onChange={(e) => {
                  setClearKey(e.target.checked);
                  setKey("");
                }}
              />{" "}
              移除已保存的 Key
            </label>
          )}
          <label className="text-tools-field">
            <span>默认目标语言</span>
            <select
              value={target}
              disabled={!loaded || saving}
              onChange={(e) => setTarget(e.target.value)}
            >
              {!TARGET_LANGUAGES.includes(target) && <option>{target}</option>}
              {TARGET_LANGUAGES.map((language) => (
                <option key={language}>{language}</option>
              ))}
            </select>
          </label>
          <p className="text-tools-hint">
            Key 使用 Windows 账户加密，不随备份或诊断包导出。修改后点击保存。
          </p>
          <div className="text-tools-toolbar">
            <button className="btn primary" disabled={!loaded || saving} onClick={save}>
              {saving ? "保存中…" : "保存识别与翻译设置"}
            </button>
          </div>
          {message && (
            <p className="text-tools-hint" role="status">
              {message}
            </p>
          )}
        </div>
      </div>
    </>
  );
}
