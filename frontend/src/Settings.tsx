import { useState, useEffect, useCallback, useRef } from "react";
import { CheckForUpdates, DownloadUpdatePackage, ExportBackup, ExportDiagnostics, GetAppInfo, GetAppSettings, GetHotkeySettings, GetLastTargetInfo, GetStorageStats, ImportBackup, OpenDataDir, QuitApp, RunAutoBackupNow, RunCleanupNow, RunStorageMaintenance, SelectAutoBackupDir, UpdateAppSettings, UpdateHotkey } from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime/runtime";
import "./Settings.css";

interface SettingsProps {
  onBack: () => void;
}

interface AppSettings {
  capturePaused: boolean;
  recordImages: boolean;
  skipSensitiveText: boolean;
  recordSourceInfo: boolean;
  startAtLogin: boolean;
  autoPaste: boolean;
  theme: string;
  minTextLength: number;
  maxClips: number;
  retentionDays: number;
  maxImageStorageMB: number;
  updateManifestURL: string;
  autoBackupEnabled: boolean;
  autoBackupDir: string;
  autoBackupIntervalDays: number;
  autoBackupMaxFiles: number;
  excludedApps: string[];
  excludedWindowTitles: string[];
}

interface AppInfo {
  name: string;
  version: string;
  dataDir: string;
  databasePath: string;
  imageDir: string;
  exePath: string;
  startHidden: boolean;
}

interface StorageStats {
  totalClips: number;
  textClips: number;
  imageClips: number;
  pinnedClips: number;
  imageBytes: number;
  databaseBytes: number;
  missingImages: number;
  unpinnedClips: number;
  thumbnailBytes: number;
  settingsEntries: number;
}

interface BackupStats {
  path: string;
  clips: number;
  textClips: number;
  imageClips: number;
  images: number;
  settings: number;
  skippedImages: number;
  cancelled: boolean;
}

interface DiagnosticStats {
  path: string;
  logBytes: number;
  files: number;
  cancelled: boolean;
}

interface UpdateCheckResult {
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
  manifestURL: string;
  downloadURL: string;
  releaseNotesURL: string;
  sha256: string;
  publishedAt: string;
  checkedAt: string;
  message: string;
}

interface UpdateDownloadResult {
  path: string;
  bytes: number;
  sha256: string;
  verified: boolean;
  cancelled: boolean;
  message: string;
}

interface MaintenanceResult {
  missingImagesRemoved: number;
  databaseBytesBefore: number;
  databaseBytesAfter: number;
  totalClipsBefore: number;
  totalClipsAfter: number;
  vacuumed: boolean;
  optimized: boolean;
  stats: StorageStats;
}

interface TargetWindowInfo {
  available: boolean;
  processName: string;
  processPath: string;
  title: string;
}

const DEFAULT_SETTINGS: AppSettings = {
  capturePaused: false,
  recordImages: true,
  skipSensitiveText: false,
  recordSourceInfo: true,
  startAtLogin: false,
  autoPaste: true,
  theme: "dark",
  minTextLength: 1,
  maxClips: 500,
  retentionDays: 0,
  maxImageStorageMB: 1024,
  updateManifestURL: "",
  autoBackupEnabled: false,
  autoBackupDir: "",
  autoBackupIntervalDays: 1,
  autoBackupMaxFiles: 10,
  excludedApps: [],
  excludedWindowTitles: [],
};

const DEFAULT_APP_INFO: AppInfo = {
  name: "ClipBox",
  version: "",
  dataDir: "",
  databasePath: "",
  imageDir: "",
  exePath: "",
  startHidden: false,
};

const DEFAULT_STATS: StorageStats = {
  totalClips: 0,
  textClips: 0,
  imageClips: 0,
  pinnedClips: 0,
  imageBytes: 0,
  databaseBytes: 0,
  missingImages: 0,
  unpinnedClips: 0,
  thumbnailBytes: 0,
  settingsEntries: 0,
};

const DEFAULT_TARGET_INFO: TargetWindowInfo = {
  available: false,
  processName: "",
  processPath: "",
  title: "",
};

const MOD_ALT = 0x0001;
const MOD_CONTROL = 0x0002;
const MOD_SHIFT = 0x0004;
const MOD_WIN = 0x0008;

function getKeyName(keyCode: number): string {
  if (keyCode >= 0x41 && keyCode <= 0x5a) return String.fromCharCode(keyCode); // A-Z
  if (keyCode >= 0x30 && keyCode <= 0x39) return String.fromCharCode(keyCode); // 0-9
  if (keyCode >= 0x70 && keyCode <= 0x7b) return `F${keyCode - 0x70 + 1}`; // F1-F12
  if (keyCode === 0x20) return "Space";
  if (keyCode === 0x0d) return "Enter";
  if (keyCode === 0x09) return "Tab";
  if (keyCode === 0x1b) return "Esc";
  if (keyCode === 0x08) return "Backspace";
  if (keyCode === 0x2e) return "Delete";
  if (keyCode === 0x24) return "Home";
  if (keyCode === 0x23) return "End";
  if (keyCode === 0x21) return "PageUp";
  if (keyCode === 0x22) return "PageDown";
  if (keyCode === 0x26) return "↑";
  if (keyCode === 0x28) return "↓";
  if (keyCode === 0x25) return "←";
  if (keyCode === 0x27) return "→";
  if (keyCode === 0xbc) return ",";
  if (keyCode === 0xbe) return ".";
  if (keyCode === 0xbf) return "/";
  if (keyCode === 0xba) return ";";
  if (keyCode === 0xde) return "'";
  if (keyCode === 0xdb) return "[";
  if (keyCode === 0xdd) return "]";
  if (keyCode === 0xdc) return "\\";
  if (keyCode === 0xbd) return "-";
  if (keyCode === 0xbb) return "=";
  if (keyCode === 0xc0) return "`";
  return `Key(${keyCode})`;
}

function formatHotkeyParts(modifiers: number, keyCode: number): string[] {
  const parts: string[] = [];
  if (modifiers & MOD_CONTROL) parts.push("Ctrl");
  if (modifiers & MOD_ALT) parts.push("Alt");
  if (modifiers & MOD_SHIFT) parts.push("Shift");
  if (modifiers & MOD_WIN) parts.push("Win");
  if (keyCode > 0) parts.push(getKeyName(keyCode));
  return parts;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 10 || unit === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`;
}

function formatRuleList(rules?: string[]): string {
  return (rules || []).join("\n");
}

function appendUniqueRule(rules: string[], rule: string): string[] {
  const value = rule.trim();
  if (!value) return rules;
  const exists = rules.some((item) => item.trim().toLowerCase() === value.toLowerCase());
  return exists ? rules : [...rules, value];
}

function targetSummary(info: TargetWindowInfo): string {
  if (!info.available) return "暂无上个窗口";
  const app = info.processName || info.processPath || "未知应用";
  return info.title ? `${app} · ${info.title}` : app;
}

// Wails 的 Go error 会以字符串形式 reject，err?.message 取不到内容。
function errText(err: unknown, fallback: string): string {
  if (typeof err === "string" && err.trim()) return err;
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

// 立即把主题应用到 <html data-theme>，不等后端保存往返。
function applyThemeNow(mode: string) {
  const resolved =
    mode === "light" || (mode === "system" && window.matchMedia("(prefers-color-scheme: light)").matches)
      ? "light"
      : "dark";
  document.documentElement.setAttribute("data-theme", resolved);
}

export default function Settings({ onBack }: SettingsProps) {
  const [currentDisplay, setCurrentDisplay] = useState("加载中...");
  const [appSettings, setAppSettings] = useState<AppSettings>(DEFAULT_SETTINGS);
  const [appInfo, setAppInfo] = useState<AppInfo>(DEFAULT_APP_INFO);
  const [stats, setStats] = useState<StorageStats>(DEFAULT_STATS);
  const [lastTargetInfo, setLastTargetInfo] = useState<TargetWindowInfo>(DEFAULT_TARGET_INFO);
  const [settingsMessage, setSettingsMessage] = useState("");
  const [rulesMessage, setRulesMessage] = useState("");
  const [storageMessage, setStorageMessage] = useState("");
  const [appMessage, setAppMessage] = useState("");
  const [cleanupRunning, setCleanupRunning] = useState(false);
  const [maintenanceRunning, setMaintenanceRunning] = useState(false);
  const [openingDataDir, setOpeningDataDir] = useState(false);
  const [backupRunning, setBackupRunning] = useState(false);
  const [importRunning, setImportRunning] = useState(false);
  const [autoBackupRunning, setAutoBackupRunning] = useState(false);
  const [selectingBackupDir, setSelectingBackupDir] = useState(false);
  const [diagnosticsRunning, setDiagnosticsRunning] = useState(false);
  const [updateChecking, setUpdateChecking] = useState(false);
  const [updateDownloading, setUpdateDownloading] = useState(false);
  const [updateResult, setUpdateResult] = useState<UpdateCheckResult | null>(null);
  const [targetLoading, setTargetLoading] = useState(false);
  const [recording, setRecording] = useState(false);
  const [recModifiers, setRecModifiers] = useState(0);
  const [recKeyCode, setRecKeyCode] = useState(0);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const recordRef = useRef<HTMLDivElement>(null);
  const saveSeqRef = useRef(0);

  // 立即把设置写入后端；失败时回滚到 prev 并提示。seq 防止乱序响应覆盖新状态。
  const persistSettings = useCallback((next: AppSettings, prev: AppSettings, onError: (msg: string) => void) => {
    const seq = ++saveSeqRef.current;
    setAppSettings(next);
    UpdateAppSettings(next)
      .then((normalized) => {
        if (saveSeqRef.current === seq) setAppSettings(normalized || next);
      })
      .catch((err) => {
        if (saveSeqRef.current !== seq) return;
        setAppSettings(prev);
        onError(errText(err, "保存设置失败"));
      });
  }, []);

  // 输入框失焦时提交当前编辑（数字/文本类设置的保存入口）。
  const commitEditedSettings = (onError: (msg: string) => void) => {
    persistSettings(appSettings, appSettings, onError);
  };

  const blurOnEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") e.currentTarget.blur();
  };

  useEffect(() => {
    GetHotkeySettings()
      .then((config) => {
        setCurrentDisplay(config.display || "未设置");
      })
      .catch(() => {
        setCurrentDisplay("未设置");
      });
  }, []);

  useEffect(() => {
    GetAppSettings()
      .then((settings) => setAppSettings(settings || DEFAULT_SETTINGS))
      .catch(() => setAppSettings(DEFAULT_SETTINGS));
    GetAppInfo()
      .then((info) => setAppInfo(info || DEFAULT_APP_INFO))
      .catch(() => setAppInfo(DEFAULT_APP_INFO));
    refreshStats();
    refreshLastTargetInfo();
  }, []);

  useEffect(() => {
    const unsubscribe = EventsOn("settings:updated", (settings?: AppSettings) => {
      // 用户正在设置页输入时不覆盖表单，避免外部事件打断编辑。
      const active = document.activeElement;
      const editing =
        active instanceof HTMLElement &&
        (active.tagName === "INPUT" || active.tagName === "TEXTAREA") &&
        Boolean(active.closest(".settings-panel"));
      if (editing) return;
      setAppSettings(settings || DEFAULT_SETTINGS);
    });
    return () => unsubscribe();
  }, []);

  const refreshStats = () => {
    GetStorageStats()
      .then((result) => setStats(result || DEFAULT_STATS))
      .catch(() => setStats(DEFAULT_STATS));
  };

  const refreshLastTargetInfo = async (): Promise<TargetWindowInfo> => {
    setTargetLoading(true);
    setRulesMessage("");
    try {
      const info = await GetLastTargetInfo();
      const normalized = info || DEFAULT_TARGET_INFO;
      setLastTargetInfo(normalized);
      return normalized;
    } catch {
      setLastTargetInfo(DEFAULT_TARGET_INFO);
      setRulesMessage("读取上个窗口失败");
      return DEFAULT_TARGET_INFO;
    } finally {
      setTargetLoading(false);
    }
  };

  const reloadAppSettings = () => {
    GetAppSettings()
      .then((settings) => setAppSettings(settings || DEFAULT_SETTINGS))
      .catch(() => setAppSettings(DEFAULT_SETTINGS));
  };

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (!recording) return;
      e.preventDefault();
      e.stopPropagation();

      // Esc 取消录制，而不是把 Esc 录成快捷键。
      if (e.key === "Escape") {
        setRecording(false);
        setRecModifiers(0);
        setRecKeyCode(0);
        return;
      }

      let mods = 0;
      if (e.ctrlKey) mods |= MOD_CONTROL;
      if (e.altKey) mods |= MOD_ALT;
      if (e.shiftKey) mods |= MOD_SHIFT;
      if (e.metaKey) mods |= MOD_WIN;

      const modKeyCodes = new Set([16, 17, 18, 91, 92, 93]);
      const vk = e.keyCode;

      if (modKeyCodes.has(vk)) {
        setRecModifiers(mods);
        return;
      }

      setRecModifiers(mods);
      setRecKeyCode(vk);
      setRecording(false);
    },
    [recording]
  );

  useEffect(() => {
    if (recording) {
      window.addEventListener("keydown", handleKeyDown, true);
      return () => window.removeEventListener("keydown", handleKeyDown, true);
    }
  }, [recording, handleKeyDown]);

  const startRecording = () => {
    setRecModifiers(0);
    setRecKeyCode(0);
    setError("");
    setRecording(true);
  };

  const cancelRecording = () => {
    setRecording(false);
    setRecModifiers(0);
    setRecKeyCode(0);
    setError("");
  };

  const handleSave = async () => {
    if (recKeyCode === 0) return;
    setSaving(true);
    setError("");
    try {
      await UpdateHotkey(recModifiers, recKeyCode);
      const parts = formatHotkeyParts(recModifiers, recKeyCode);
      setCurrentDisplay(parts.join(" + "));
      setRecModifiers(0);
      setRecKeyCode(0);
    } catch (err: any) {
      setError(errText(err, "保存失败，请重试"));
    } finally {
      setSaving(false);
    }
  };

  const updateBooleanSetting = (key: "capturePaused" | "recordImages" | "skipSensitiveText" | "recordSourceInfo" | "startAtLogin" | "autoPaste" | "autoBackupEnabled") => {
    setSettingsMessage("");
    setRulesMessage("");
    setStorageMessage("");
    setAppMessage("");
    // 开关类设置拨动后立即持久化，失败会回滚开关状态。
    const next = { ...appSettings, [key]: !appSettings[key] };
    persistSettings(next, appSettings, key === "autoBackupEnabled" ? setAppMessage : setRulesMessage);
  };

  const updateNumberSetting = (key: "minTextLength" | "maxClips" | "retentionDays" | "maxImageStorageMB" | "autoBackupIntervalDays" | "autoBackupMaxFiles", value: string) => {
    setSettingsMessage("");
    setRulesMessage("");
    setStorageMessage("");
    setAppMessage("");
    const parsed = Number.parseInt(value, 10);
    setAppSettings((prev) => ({ ...prev, [key]: Number.isFinite(parsed) ? parsed : 0 }));
  };

  const updateStringSetting = (key: "updateManifestURL" | "autoBackupDir", value: string) => {
    setSettingsMessage("");
    setRulesMessage("");
    setStorageMessage("");
    setAppMessage("");
    setUpdateResult(null);
    setAppSettings((prev) => ({ ...prev, [key]: value }));
  };

  const updateRuleListSetting = (key: "excludedApps" | "excludedWindowTitles", value: string) => {
    setSettingsMessage("");
    setRulesMessage("");
    setStorageMessage("");
    setAppMessage("");
    const lines = value === "" ? [] : value.replace(/\r/g, "").split("\n");
    setAppSettings((prev) => ({ ...prev, [key]: lines }));
  };

  const addLastTargetAppRule = async () => {
    const info = lastTargetInfo.available ? lastTargetInfo : await refreshLastTargetInfo();
    const value = info.processName || info.processPath;
    if (!value) {
      setRulesMessage("没有可加入的上个应用");
      return;
    }
    const nextRules = appendUniqueRule(appSettings.excludedApps, value);
    if (nextRules === appSettings.excludedApps) {
      setRulesMessage("该应用已在排除列表中");
      return;
    }
    persistSettings({ ...appSettings, excludedApps: nextRules }, appSettings, setRulesMessage);
    setRulesMessage("已加入排除应用");
  };

  const addLastTargetTitleRule = async () => {
    const info = lastTargetInfo.available ? lastTargetInfo : await refreshLastTargetInfo();
    if (!info.title) {
      setRulesMessage("没有可加入的窗口标题");
      return;
    }
    const nextRules = appendUniqueRule(appSettings.excludedWindowTitles, info.title);
    if (nextRules === appSettings.excludedWindowTitles) {
      setRulesMessage("该标题已在排除列表中");
      return;
    }
    persistSettings({ ...appSettings, excludedWindowTitles: nextRules }, appSettings, setRulesMessage);
    setRulesMessage("已加入排除标题");
  };

  const runCleanup = async () => {
    setCleanupRunning(true);
    setSettingsMessage("");
    setStorageMessage("");
    try {
      const result = await RunCleanupNow();
      setStats(result || DEFAULT_STATS);
      setStorageMessage("清理完成");
    } catch (err: any) {
      setStorageMessage(errText(err, "清理失败"));
    } finally {
      setCleanupRunning(false);
    }
  };

  const runMaintenance = async () => {
    setMaintenanceRunning(true);
    setStorageMessage("");
    try {
      const result: MaintenanceResult = await RunStorageMaintenance();
      setStats(result?.stats || DEFAULT_STATS);
      const dbDelta = (result?.databaseBytesBefore || 0) - (result?.databaseBytesAfter || 0);
      const compacted = dbDelta > 0 ? `，回收 ${formatBytes(dbDelta)}` : "";
      setStorageMessage(`已修复，移除 ${result?.missingImagesRemoved || 0} 条缺失图片记录${compacted}`);
    } catch (err: any) {
      setStorageMessage(errText(err, "修复失败"));
    } finally {
      setMaintenanceRunning(false);
    }
  };

  const openDataDir = async () => {
    setOpeningDataDir(true);
    setAppMessage("");
    try {
      await OpenDataDir();
    } catch (err: any) {
      setAppMessage(errText(err, "打开数据目录失败"));
    } finally {
      setOpeningDataDir(false);
    }
  };

  const exportBackup = async () => {
    setBackupRunning(true);
    setAppMessage("");
    try {
      const result: BackupStats = await ExportBackup();
      if (result?.cancelled) {
        setAppMessage("已取消导出");
      } else {
        const skipped = result?.skippedImages ? `，跳过 ${result.skippedImages} 个图片` : "";
        setAppMessage(`已导出 ${result?.clips || 0} 条记录、${result?.images || 0} 个图片${skipped}`);
      }
    } catch (err: any) {
      setAppMessage(errText(err, "导出失败"));
    } finally {
      setBackupRunning(false);
    }
  };

  const importBackup = async () => {
    setImportRunning(true);
    setAppMessage("");
    try {
      const result: BackupStats = await ImportBackup();
      if (result?.cancelled) {
        setAppMessage("已取消导入");
      } else {
        refreshStats();
        reloadAppSettings();
        const skipped = result?.skippedImages ? `，跳过 ${result.skippedImages} 个图片` : "";
        setAppMessage(`已导入 ${result?.clips || 0} 条新记录、${result?.settings || 0} 项设置${skipped}`);
      }
    } catch (err: any) {
      setAppMessage(errText(err, "导入失败"));
    } finally {
      setImportRunning(false);
    }
  };

  const selectAutoBackupDir = async () => {
    setSelectingBackupDir(true);
    setAppMessage("");
    try {
      const path = await SelectAutoBackupDir();
      if (path) {
        persistSettings({ ...appSettings, autoBackupDir: path }, appSettings, setAppMessage);
        setAppMessage("已选择自动备份目录");
      }
    } catch (err: any) {
      setAppMessage(errText(err, "选择自动备份目录失败"));
    } finally {
      setSelectingBackupDir(false);
    }
  };

  const runAutoBackupNow = async () => {
    setAutoBackupRunning(true);
    setAppMessage("");
    try {
      const normalized = await UpdateAppSettings(appSettings);
      setAppSettings(normalized || appSettings);
      const result: BackupStats = await RunAutoBackupNow();
      const skipped = result?.skippedImages ? `，跳过 ${result.skippedImages} 个图片` : "";
      setAppMessage(`已自动备份 ${result?.clips || 0} 条记录：${result?.path || ""}${skipped}`);
    } catch (err: any) {
      setAppMessage(errText(err, "自动备份失败"));
    } finally {
      setAutoBackupRunning(false);
    }
  };

  const exportDiagnostics = async () => {
    setDiagnosticsRunning(true);
    setAppMessage("");
    try {
      const result: DiagnosticStats = await ExportDiagnostics();
      if (result?.cancelled) {
        setAppMessage("已取消导出诊断包");
      } else {
        setAppMessage(`已导出诊断包：${result?.files || 0} 个文件，日志 ${formatBytes(result?.logBytes || 0)}`);
      }
    } catch (err: any) {
      setAppMessage(errText(err, "导出诊断包失败"));
    } finally {
      setDiagnosticsRunning(false);
    }
  };

  const checkForUpdates = async () => {
    setUpdateChecking(true);
    setAppMessage("");
    setUpdateResult(null);
    try {
      const result: UpdateCheckResult = await CheckForUpdates(appSettings.updateManifestURL || "");
      setUpdateResult(result || null);
      setAppMessage(result?.message || "检查完成");
    } catch (err: any) {
      setAppMessage(errText(err, "检查更新失败"));
    } finally {
      setUpdateChecking(false);
    }
  };

  const downloadUpdatePackage = async () => {
    if (!updateResult?.downloadURL) return;
    setUpdateDownloading(true);
    setAppMessage("");
    try {
      const result: UpdateDownloadResult = await DownloadUpdatePackage(updateResult.downloadURL, updateResult.sha256 || "");
      if (result?.cancelled) {
        setAppMessage("已取消下载更新包");
      } else {
        const verified = result?.verified ? "，SHA256 已校验" : "";
        setAppMessage(`已下载 ${formatBytes(result?.bytes || 0)}${verified}：${result?.path || ""}`);
      }
    } catch (err: any) {
      setAppMessage(errText(err, "下载更新包失败"));
    } finally {
      setUpdateDownloading(false);
    }
  };

  const recParts = formatHotkeyParts(recModifiers, recKeyCode);
  const hasRecordedKeys = recModifiers > 0 || recKeyCode > 0;
  const canSave = recKeyCode > 0 && !saving;

  const currentParts = currentDisplay.split(/\s*\+\s*/);

  return (
    <div className="settings-panel">
      <div className="settings-header">
        <button className="back-btn" onClick={onBack} title="返回">
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <polyline points="15 18 9 12 15 6" />
          </svg>
        </button>
        <span className="settings-title">设置</span>
      </div>

      <div className="settings-content">
        <div className="settings-section">
          <div className="section-label">状态</div>
          <div className="section-card">
            <div className="stats-grid">
              <div className="stat-item">
                <span className="stat-value">{stats.totalClips}</span>
                <span className="stat-label">总记录</span>
              </div>
              <div className="stat-item">
                <span className="stat-value">{stats.textClips}</span>
                <span className="stat-label">文本</span>
              </div>
              <div className="stat-item">
                <span className="stat-value">{stats.imageClips}</span>
                <span className="stat-label">图片</span>
              </div>
              <div className="stat-item">
                <span className="stat-value">{stats.pinnedClips}</span>
                <span className="stat-label">置顶</span>
              </div>
            </div>
            <div className="storage-lines">
              <div><span>图片文件</span><strong>{formatBytes(stats.imageBytes)}</strong></div>
              <div><span>数据库</span><strong>{formatBytes(stats.databaseBytes)}</strong></div>
              <div><span>缩略图缓存</span><strong>{formatBytes(stats.thumbnailBytes)}</strong></div>
              {stats.missingImages > 0 && <div><span>缺失图片</span><strong>{stats.missingImages}</strong></div>}
            </div>
            <div className="settings-actions inline">
              {storageMessage && <span className="save-message">{storageMessage}</span>}
              <button className="btn" onClick={refreshStats}>刷新</button>
              <button className="btn" onClick={runMaintenance} disabled={maintenanceRunning}>
                {maintenanceRunning ? "修复中..." : "修复存储"}
              </button>
              <button className="btn primary" onClick={runCleanup} disabled={cleanupRunning}>
                {cleanupRunning ? "清理中..." : "立即清理"}
              </button>
            </div>
          </div>
        </div>

        <div className="settings-section">
          <div className="section-label">外观</div>
          <div className="section-card">
            <div className="setting-row">
              <div>
                <div className="hotkey-label">主题</div>
                <div className="hotkey-description">深色、浅色或跟随 Windows 系统</div>
              </div>
              <div className="theme-options">
                {([["dark", "深色"], ["light", "浅色"], ["system", "跟随系统"]] as const).map(([value, label]) => (
                  <button
                    key={value}
                    className={`btn theme-option ${appSettings.theme === value ? "active" : ""}`}
                    onClick={() => {
                      if (appSettings.theme === value) return;
                      const prevTheme = appSettings.theme;
                      applyThemeNow(value);
                      persistSettings({ ...appSettings, theme: value }, appSettings, (msg) => {
                        applyThemeNow(prevTheme);
                        setAppMessage(msg);
                      });
                    }}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </div>

        <div className="settings-section">
          <div className="section-label">记录</div>
          <div className="section-card">
            <label className="setting-row">
              <div>
                <div className="hotkey-label">暂停记录</div>
                <div className="hotkey-description">临时停止写入新的剪贴板历史</div>
              </div>
              <input
                className="toggle-input"
                type="checkbox"
                checked={appSettings.capturePaused}
                onChange={() => updateBooleanSetting("capturePaused")}
              />
            </label>
            <label className="setting-row">
              <div>
                <div className="hotkey-label">记录图片</div>
                <div className="hotkey-description">关闭后只保留文本历史</div>
              </div>
              <input
                className="toggle-input"
                type="checkbox"
                checked={appSettings.recordImages}
                onChange={() => updateBooleanSetting("recordImages")}
              />
            </label>
            <label className="setting-row">
              <div>
                <div className="hotkey-label">跳过疑似敏感文本</div>
                <div className="hotkey-description">包含 password、token、secret 等片段时不记录</div>
              </div>
              <input
                className="toggle-input"
                type="checkbox"
                checked={appSettings.skipSensitiveText}
                onChange={() => updateBooleanSetting("skipSensitiveText")}
              />
            </label>
            <label className="setting-row">
              <div>
                <div className="hotkey-label">记录来源窗口</div>
                <div className="hotkey-description">为新记录保存来源应用和窗口标题</div>
              </div>
              <input
                className="toggle-input"
                type="checkbox"
                checked={appSettings.recordSourceInfo}
                onChange={() => updateBooleanSetting("recordSourceInfo")}
              />
            </label>
            <div className="setting-row stack">
              <div>
                <div className="hotkey-label">排除应用</div>
                <div className="hotkey-description">每行一个进程名或路径关键词，命中前台应用时不记录</div>
              </div>
              <textarea
                className="text-list-input"
                rows={3}
                spellCheck={false}
                value={formatRuleList(appSettings.excludedApps)}
                placeholder={"1Password.exe\nKeePassXC.exe"}
                onChange={(e) => updateRuleListSetting("excludedApps", e.target.value)}
                onBlur={() => commitEditedSettings(setRulesMessage)}
              />
            </div>
            <div className="setting-row stack">
              <div>
                <div className="hotkey-label">排除窗口标题</div>
                <div className="hotkey-description">每行一个标题关键词，命中前台窗口时不记录</div>
              </div>
              <textarea
                className="text-list-input"
                rows={3}
                spellCheck={false}
                value={formatRuleList(appSettings.excludedWindowTitles)}
                placeholder={"密码\nSecret"}
                onChange={(e) => updateRuleListSetting("excludedWindowTitles", e.target.value)}
                onBlur={() => commitEditedSettings(setRulesMessage)}
              />
            </div>
            <div className="target-rule-panel">
              <div className="target-rule-info">
                <span>上个窗口</span>
                <strong title={targetSummary(lastTargetInfo)}>{targetSummary(lastTargetInfo)}</strong>
              </div>
              <div className="target-rule-actions">
                <button className="btn" onClick={addLastTargetAppRule} disabled={targetLoading}>
                  加入应用
                </button>
                <button className="btn" onClick={addLastTargetTitleRule} disabled={targetLoading}>
                  加入标题
                </button>
                <button className="btn" onClick={() => refreshLastTargetInfo()} disabled={targetLoading}>
                  {targetLoading ? "刷新中..." : "刷新"}
                </button>
              </div>
              {rulesMessage && <div className="rule-action-message">{rulesMessage}</div>}
            </div>
            <label className="setting-row compact">
              <div>
                <div className="hotkey-label">最小文本长度</div>
                <div className="hotkey-description">短于该长度的文本不会记录</div>
              </div>
              <input
                className="number-input"
                type="number"
                min={1}
                max={5000}
                value={appSettings.minTextLength}
                onChange={(e) => updateNumberSetting("minTextLength", e.target.value)}
                onBlur={() => commitEditedSettings(setRulesMessage)}
                onKeyDown={blurOnEnter}
              />
            </label>
            <label className="setting-row">
              <div>
                <div className="hotkey-label">开机自启</div>
                <div className="hotkey-description">登录 Windows 后在后台启动 ClipBox</div>
              </div>
              <input
                className="toggle-input"
                type="checkbox"
                checked={appSettings.startAtLogin}
                onChange={() => updateBooleanSetting("startAtLogin")}
              />
            </label>
            <label className="setting-row">
              <div>
                <div className="hotkey-label">自动粘贴</div>
                <div className="hotkey-description">选择历史项后模拟 Ctrl+V 到上一个窗口</div>
              </div>
              <input
                className="toggle-input"
                type="checkbox"
                checked={appSettings.autoPaste}
                onChange={() => updateBooleanSetting("autoPaste")}
              />
            </label>
          </div>
        </div>

        <div className="settings-section">
          <div className="section-label">保留</div>
          <div className="section-card">
            <label className="setting-row compact">
              <div>
                <div className="hotkey-label">最多记录数</div>
                <div className="hotkey-description">超过后自动删除最旧的非置顶记录</div>
              </div>
              <input
                className="number-input"
                type="number"
                min={50}
                max={10000}
                value={appSettings.maxClips}
                onChange={(e) => updateNumberSetting("maxClips", e.target.value)}
                onBlur={() => commitEditedSettings(setSettingsMessage)}
                onKeyDown={blurOnEnter}
              />
            </label>
            <label className="setting-row compact">
              <div>
                <div className="hotkey-label">保留天数</div>
                <div className="hotkey-description">0 表示不按时间清理</div>
              </div>
              <input
                className="number-input"
                type="number"
                min={0}
                max={3650}
                value={appSettings.retentionDays}
                onChange={(e) => updateNumberSetting("retentionDays", e.target.value)}
                onBlur={() => commitEditedSettings(setSettingsMessage)}
                onKeyDown={blurOnEnter}
              />
            </label>
            <label className="setting-row compact">
              <div>
                <div className="hotkey-label">图片空间上限 MB</div>
                <div className="hotkey-description">超出后优先清理最旧的非置顶图片</div>
              </div>
              <input
                className="number-input"
                type="number"
                min={32}
                max={102400}
                value={appSettings.maxImageStorageMB}
                onChange={(e) => updateNumberSetting("maxImageStorageMB", e.target.value)}
                onBlur={() => commitEditedSettings(setSettingsMessage)}
                onKeyDown={blurOnEnter}
              />
            </label>
            {settingsMessage && (
              <div className="settings-actions inline">
                <span className="save-message">{settingsMessage}</span>
              </div>
            )}
          </div>
        </div>

        <div className="settings-section">
          <div className="section-label">快捷键</div>
          <div className="section-card">
            <div className="hotkey-row">
              <div>
                <div className="hotkey-label">唤起快捷键</div>
                <div className="hotkey-description">
                  设置全局快捷键以唤起 ClipBox
                </div>
              </div>
              <div className="hotkey-keys">
                {currentParts.map((part, i) => (
                  <span key={i} style={{ display: "flex", alignItems: "center", gap: 4 }}>
                    {i > 0 && <span className="key-separator">+</span>}
                    <span className="key-badge">{part.trim()}</span>
                  </span>
                ))}
              </div>
            </div>

            <div className="hotkey-record-area">
              <div
                ref={recordRef}
                className={`record-box ${recording ? "recording" : ""} ${
                  hasRecordedKeys && !recording ? "has-keys" : ""
                }`}
              >
                {recording && !hasRecordedKeys && (
                  <span className="record-placeholder recording-text">
                    请按下新的快捷键组合...
                  </span>
                )}
                {recording && hasRecordedKeys && (
                  <div className="recorded-keys">
                    {recParts.map((part, i) => (
                      <span key={i} style={{ display: "flex", alignItems: "center", gap: 4 }}>
                        {i > 0 && <span className="key-separator">+</span>}
                        <span className="key-badge">{part}</span>
                      </span>
                    ))}
                  </div>
                )}
                {!recording && hasRecordedKeys && (
                  <div className="recorded-keys">
                    {recParts.map((part, i) => (
                      <span key={i} style={{ display: "flex", alignItems: "center", gap: 4 }}>
                        {i > 0 && <span className="key-separator">+</span>}
                        <span className="key-badge">{part}</span>
                      </span>
                    ))}
                  </div>
                )}
                {!recording && !hasRecordedKeys && (
                  <span className="record-placeholder">
                    点击「录制」按钮开始设置新的快捷键
                  </span>
                )}
              </div>

              <div className="settings-actions">
                {!recording && !hasRecordedKeys && (
                  <button className="btn record-btn" onClick={startRecording}>
                    录制
                  </button>
                )}
                {recording && (
                  <button
                    className="btn record-btn recording"
                    onClick={cancelRecording}
                  >
                    取消录制
                  </button>
                )}
                {!recording && hasRecordedKeys && (
                  <>
                    <button className="btn" onClick={cancelRecording}>
                      取消
                    </button>
                    <button
                      className="btn primary"
                      onClick={handleSave}
                      disabled={!canSave}
                    >
                      {saving ? "保存中..." : "保存"}
                    </button>
                  </>
                )}
              </div>

              {error && (
                <div className="error-message">
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
                    <circle cx="12" cy="12" r="10" />
                    <line x1="15" y1="9" x2="9" y2="15" />
                    <line x1="9" y1="9" x2="15" y2="15" />
                  </svg>
                  {error}
                </div>
              )}
            </div>
          </div>

          <div className="section-info">
            <p>建议使用 Ctrl、Alt、Shift 等修饰键与字母或功能键的组合。</p>
            <p>快捷键修改后立即生效，无需重启应用。</p>
          </div>
        </div>

        <div className="settings-section">
          <div className="section-label">应用</div>
          <div className="section-card">
            <div className="app-info-lines">
              <div><span>名称</span><strong>{appInfo.name || "ClipBox"}</strong></div>
              <div><span>版本</span><strong>{appInfo.version || "-"}</strong></div>
              <div><span>数据目录</span><strong title={appInfo.dataDir}>{appInfo.dataDir || "-"}</strong></div>
              <div><span>程序路径</span><strong title={appInfo.exePath}>{appInfo.exePath || "-"}</strong></div>
            </div>
            <div className="setting-row stack">
              <div>
                <div className="hotkey-label">更新检查</div>
                <div className="hotkey-description">从发布清单读取最新版本，不会自动下载</div>
              </div>
              <input
                className="url-input"
                type="url"
                spellCheck={false}
                value={appSettings.updateManifestURL || ""}
                placeholder="https://example.com/clipbox/release-manifest.json"
                onChange={(e) => updateStringSetting("updateManifestURL", e.target.value)}
                onBlur={() => commitEditedSettings(setAppMessage)}
                onKeyDown={blurOnEnter}
              />
              <div className="update-actions">
                <button className="btn" onClick={checkForUpdates} disabled={updateChecking}>
                  {updateChecking ? "检查中..." : "检查更新"}
                </button>
                {updateResult?.downloadURL && (
                  <button className="btn primary" onClick={downloadUpdatePackage} disabled={updateDownloading}>
                    {updateDownloading ? "下载中..." : "下载更新包"}
                  </button>
                )}
                {updateResult && (
                  <div className={`update-result ${updateResult.updateAvailable ? "available" : ""}`}>
                    <strong>{updateResult.message || "检查完成"}</strong>
                    <span>当前 {updateResult.currentVersion || appInfo.version || "-"} · 最新 {updateResult.latestVersion || "-"}</span>
                    {updateResult.downloadURL && <span title={updateResult.downloadURL}>下载 {updateResult.downloadURL}</span>}
                    {updateResult.downloadURL && <span>{updateResult.sha256 ? "SHA256 可校验" : "未提供 SHA256"}</span>}
                    {updateResult.releaseNotesURL && <span title={updateResult.releaseNotesURL}>说明 {updateResult.releaseNotesURL}</span>}
                  </div>
                )}
              </div>
            </div>
            <div className="setting-row compact">
              <div>
                <div className="hotkey-label">备份与恢复</div>
                <div className="hotkey-description">导出或合并导入历史记录、图片和本地设置</div>
              </div>
              <div className="backup-actions">
                <button className="btn" onClick={exportBackup} disabled={backupRunning || importRunning}>
                  {backupRunning ? "导出中..." : "导出"}
                </button>
                <button className="btn" onClick={importBackup} disabled={backupRunning || importRunning}>
                  {importRunning ? "导入中..." : "导入"}
                </button>
              </div>
            </div>
            <label className="setting-row">
              <div>
                <div className="hotkey-label">自动本地备份</div>
                <div className="hotkey-description">启动时按间隔写入 .clipbox-backup 文件</div>
              </div>
              <input
                className="toggle-input"
                type="checkbox"
                checked={appSettings.autoBackupEnabled}
                onChange={() => updateBooleanSetting("autoBackupEnabled")}
              />
            </label>
            <div className="setting-row stack">
              <div>
                <div className="hotkey-label">自动备份目录</div>
                <div className="hotkey-description">只清理自动备份文件，不影响手动备份</div>
              </div>
              <div className="path-picker-row">
                <input
                  className="url-input"
                  type="text"
                  spellCheck={false}
                  value={appSettings.autoBackupDir || ""}
                  placeholder="选择一个本地备份目录"
                  onChange={(e) => updateStringSetting("autoBackupDir", e.target.value)}
                  onBlur={() => commitEditedSettings(setAppMessage)}
                  onKeyDown={blurOnEnter}
                />
                <button className="btn" onClick={selectAutoBackupDir} disabled={selectingBackupDir}>
                  {selectingBackupDir ? "选择中..." : "选择"}
                </button>
              </div>
            </div>
            <div className="setting-row compact">
              <div>
                <div className="hotkey-label">自动备份策略</div>
                <div className="hotkey-description">间隔天数和自动备份保留数量</div>
              </div>
              <div className="number-pair">
                <input
                  className="number-input"
                  type="number"
                  min={1}
                  max={365}
                  value={appSettings.autoBackupIntervalDays}
                  onChange={(e) => updateNumberSetting("autoBackupIntervalDays", e.target.value)}
                  onBlur={() => commitEditedSettings(setAppMessage)}
                  onKeyDown={blurOnEnter}
                  title="间隔天数"
                />
                <input
                  className="number-input"
                  type="number"
                  min={1}
                  max={100}
                  value={appSettings.autoBackupMaxFiles}
                  onChange={(e) => updateNumberSetting("autoBackupMaxFiles", e.target.value)}
                  onBlur={() => commitEditedSettings(setAppMessage)}
                  onKeyDown={blurOnEnter}
                  title="保留数量"
                />
                <button className="btn" onClick={runAutoBackupNow} disabled={autoBackupRunning || !appSettings.autoBackupDir}>
                  {autoBackupRunning ? "备份中..." : "立即备份"}
                </button>
              </div>
            </div>
            <div className="setting-row compact">
              <div>
                <div className="hotkey-label">数据目录</div>
                <div className="hotkey-description">查看数据库、图片缓存和本地配置</div>
              </div>
              <button className="btn" onClick={openDataDir} disabled={openingDataDir}>
                {openingDataDir ? "打开中..." : "打开"}
              </button>
            </div>
            <div className="setting-row compact">
              <div>
                <div className="hotkey-label">诊断包</div>
                <div className="hotkey-description">导出版本、设置、统计和运行日志，不包含剪贴板正文</div>
              </div>
              <button className="btn" onClick={exportDiagnostics} disabled={diagnosticsRunning}>
                {diagnosticsRunning ? "导出中..." : "导出"}
              </button>
            </div>
            <div className="setting-row compact">
              <div>
                <div className="hotkey-label">退出 ClipBox</div>
                <div className="hotkey-description">停止剪贴板监听和全局快捷键</div>
              </div>
              <button className="btn danger-btn" onClick={() => QuitApp()}>
                退出
              </button>
            </div>
            {appMessage && <div className="app-action-message">{appMessage}</div>}
          </div>
        </div>
      </div>
    </div>
  );
}
