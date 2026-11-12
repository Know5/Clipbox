import type { main, storage } from "../wailsjs/go/models";
export type AppSettings = main.AppSettings;

export type AppInfo = main.AppInfo;

export type StorageStats = storage.Stats;

export type BackupStats = storage.BackupStats;

export type DiagnosticStats = main.DiagnosticStats;

export type UpdateCheckResult = main.UpdateCheckResult;

export type UpdateDownloadResult = main.UpdateDownloadResult;

export type MaintenanceResult = Omit<storage.MaintenanceResult, "convertValues">;

export type TargetWindowInfo = main.TargetWindowInfo;

export const DEFAULT_SETTINGS: AppSettings = {
  capturePaused: false,
  recordImages: true,
  skipSensitiveText: false,
  recordSourceInfo: true,
  startAtLogin: false,
  autoPaste: false,
  uiIdleSeconds: 60,
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
  startAtLoginBlocked: false,
};

export const DEFAULT_APP_INFO: AppInfo = {
  name: "ClipBox",
  version: "",
  dataDir: "",
  databasePath: "",
  imageDir: "",
  exePath: "",
  startHidden: false,
};

export const DEFAULT_STATS: StorageStats = {
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

export const DEFAULT_TARGET_INFO: TargetWindowInfo = {
  available: false,
  processName: "",
  processPath: "",
  title: "",
};

export const MOD_ALT = 0x0001;
export const MOD_CONTROL = 0x0002;
export const MOD_SHIFT = 0x0004;
export const MOD_WIN = 0x0008;

export function getKeyName(keyCode: number): string {
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

export function formatHotkeyParts(modifiers: number, keyCode: number): string[] {
  const parts: string[] = [];
  if (modifiers & MOD_CONTROL) parts.push("Ctrl");
  if (modifiers & MOD_ALT) parts.push("Alt");
  if (modifiers & MOD_SHIFT) parts.push("Shift");
  if (modifiers & MOD_WIN) parts.push("Win");
  if (keyCode > 0) parts.push(getKeyName(keyCode));
  return parts;
}

export function formatBytes(bytes: number): string {
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

export function formatRuleList(rules?: string[]): string {
  return (rules || []).join("\n");
}

export function appendUniqueRule(rules: string[], rule: string): string[] {
  const value = rule.trim();
  if (!value) return rules;
  const exists = rules.some((item) => item.trim().toLowerCase() === value.toLowerCase());
  return exists ? rules : [...rules, value];
}

export function targetSummary(info: TargetWindowInfo): string {
  if (!info.available) return "暂无上个窗口";
  const app = info.processName || info.processPath || "未知应用";
  return info.title ? `${app} · ${info.title}` : app;
}

// Wails 的 Go error 会以字符串形式 reject，err?.message 取不到内容。
export function errText(err: unknown, fallback: string): string {
  if (typeof err === "string" && err.trim()) return err;
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

// 立即把主题应用到 <html data-theme>，不等后端保存往返。
export function applyThemeNow(mode: string) {
  const resolved =
    mode === "light" ||
    (mode === "system" && window.matchMedia("(prefers-color-scheme: light)").matches)
      ? "light"
      : "dark";
  document.documentElement.setAttribute("data-theme", resolved);
}

export type SettingsTab = "general" | "privacy" | "storage" | "advanced" | "textTools";
