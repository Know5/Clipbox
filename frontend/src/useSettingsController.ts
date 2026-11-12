import { useCallback, useEffect, useRef, useState } from "react";
import { EventsOn } from "../wailsjs/runtime/runtime";
import {
  CheckForUpdates,
  DownloadUpdatePackage,
  ExportBackup,
  ExportDiagnostics,
  GetAppInfo,
  GetAppSettings,
  GetHotkeySettings,
  GetLastTargetInfo,
  GetStorageStats,
  ImportBackup,
  OpenDataDir,
  PatchAppSettings,
  RepairStartAtLogin,
  RunAutoBackupNow,
  RunCleanupNow,
  RunStorageMaintenance,
  SelectAutoBackupDir,
  UpdateHotkey,
} from "./api";
import type {
  AppInfo,
  AppSettings,
  BackupStats,
  DiagnosticStats,
  MaintenanceResult,
  StorageStats,
  TargetWindowInfo,
  UpdateCheckResult,
  UpdateDownloadResult,
} from "./settingsModel";
import {
  DEFAULT_APP_INFO,
  DEFAULT_SETTINGS,
  DEFAULT_STATS,
  DEFAULT_TARGET_INFO,
  MOD_ALT,
  MOD_CONTROL,
  MOD_SHIFT,
  MOD_WIN,
  appendUniqueRule,
  errText,
  formatBytes,
  formatHotkeyParts,
} from "./settingsModel";
import { useUIActivity } from "./useUIActivity";
export function useSettingsController() {
  const [currentDisplay, setCurrentDisplay] = useState("加载中...");

  const [appSettings, setAppSettings] = useState<AppSettings>(DEFAULT_SETTINGS);

  const [appInfo, setAppInfo] = useState<AppInfo>(DEFAULT_APP_INFO);

  const [stats, setStats] = useState<StorageStats>(DEFAULT_STATS);

  const [lastTargetInfo, setLastTargetInfo] = useState<TargetWindowInfo>(DEFAULT_TARGET_INFO);

  const [settingsMessage, setSettingsMessage] = useState("");

  const [rulesMessage, setRulesMessage] = useState("");

  const [repairing, setRepairing] = useState(false);

  const [repairMessage, setRepairMessage] = useState("");

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

  const dirtyFields = useRef(new Set<keyof AppSettings>());

  const saveQueue = useRef<Promise<unknown>>(Promise.resolve());

  useUIActivity(
    dirtyFields.current.size > 0 ||
      saving ||
      backupRunning ||
      importRunning ||
      diagnosticsRunning ||
      updateDownloading,
  );

  // 立即把设置写入后端；失败时回滚到 prev 并提示。seq 防止乱序响应覆盖新状态。
  const persistSettings = useCallback(
    (next: AppSettings, prev: AppSettings, onError: (msg: string) => void) => {
      const seq = ++saveSeqRef.current;
      setAppSettings(next);
      const patch: Partial<AppSettings> = {};
      for (const key of Object.keys(next) as (keyof AppSettings)[]) {
        if (
          key !== "startAtLoginBlocked" &&
          (dirtyFields.current.has(key) || JSON.stringify(next[key]) !== JSON.stringify(prev[key]))
        ) {
          Object.assign(patch, { [key]: next[key] });
        }
      }
      const request = saveQueue.current.catch(() => {}).then(() => PatchAppSettings(patch));
      saveQueue.current = request;
      request
        .then((normalized) => {
          if (saveSeqRef.current === seq) {
            setAppSettings((draft) => {
              const accepted = { ...draft };
              for (const key of Object.keys(patch) as (keyof AppSettings)[]) {
                if (JSON.stringify(draft[key]) === JSON.stringify(next[key])) {
                  dirtyFields.current.delete(key);
                  Object.assign(accepted, { [key]: normalized[key] });
                }
              }
              return accepted;
            });
          }
        })
        .catch((err) => {
          if (saveSeqRef.current !== seq) return;
          GetAppSettings()
            .then((saved) => {
              dirtyFields.current.clear();
              setAppSettings(saved);
            })
            .catch(() => setAppSettings(prev));
          onError(errText(err, "保存设置失败"));
        });
    },
    [],
  );

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
      setAppSettings((draft) => {
        const next = { ...(settings || DEFAULT_SETTINGS) };
        for (const key of dirtyFields.current) Object.assign(next, { [key]: draft[key] });
        return next;
      });
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
    [recording],
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
    } catch (err) {
      setError(errText(err, "保存失败，请重试"));
    } finally {
      setSaving(false);
    }
  };

  const updateBooleanSetting = (
    key:
      | "capturePaused"
      | "recordImages"
      | "skipSensitiveText"
      | "recordSourceInfo"
      | "startAtLogin"
      | "autoPaste"
      | "autoBackupEnabled",
  ) => {
    setSettingsMessage("");
    setRulesMessage("");
    setStorageMessage("");
    setAppMessage("");
    // 开关类设置拨动后立即持久化，失败会回滚开关状态。
    const next = { ...appSettings, [key]: !appSettings[key] };
    persistSettings(
      next,
      appSettings,
      key === "autoBackupEnabled" ? setAppMessage : setRulesMessage,
    );
  };

  const updateNumberSetting = (
    key:
      | "minTextLength"
      | "maxClips"
      | "retentionDays"
      | "maxImageStorageMB"
      | "autoBackupIntervalDays"
      | "autoBackupMaxFiles",
    value: string,
  ) => {
    dirtyFields.current.add(key);
    setSettingsMessage("");
    setRulesMessage("");
    setStorageMessage("");
    setAppMessage("");
    const parsed = Number.parseInt(value, 10);
    setAppSettings((prev) => ({ ...prev, [key]: Number.isFinite(parsed) ? parsed : 0 }));
  };

  const updateStringSetting = (key: "updateManifestURL" | "autoBackupDir", value: string) => {
    dirtyFields.current.add(key);
    setSettingsMessage("");
    setRulesMessage("");
    setStorageMessage("");
    setAppMessage("");
    setUpdateResult(null);
    setAppSettings((prev) => ({ ...prev, [key]: value }));
  };

  const updateRuleListSetting = (key: "excludedApps" | "excludedWindowTitles", value: string) => {
    dirtyFields.current.add(key);
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
    persistSettings(
      { ...appSettings, excludedWindowTitles: nextRules },
      appSettings,
      setRulesMessage,
    );
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
    } catch (err) {
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
      setStorageMessage(
        `已修复，移除 ${result?.missingImagesRemoved || 0} 条缺失图片记录${compacted}`,
      );
    } catch (err) {
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
    } catch (err) {
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
        setAppMessage(
          `已导出 ${result?.clips || 0} 条记录、${result?.images || 0} 个图片${skipped}`,
        );
      }
    } catch (err) {
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
        setAppMessage(
          `已导入 ${result?.clips || 0} 条新记录、${result?.settings || 0} 项设置${skipped}`,
        );
      }
    } catch (err) {
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
    } catch (err) {
      setAppMessage(errText(err, "选择自动备份目录失败"));
    } finally {
      setSelectingBackupDir(false);
    }
  };

  const runAutoBackupNow = async () => {
    setAutoBackupRunning(true);
    setAppMessage("");
    try {
      const normalized = await PatchAppSettings({
        autoBackupDir: appSettings.autoBackupDir,
        autoBackupEnabled: appSettings.autoBackupEnabled,
        autoBackupIntervalDays: appSettings.autoBackupIntervalDays,
        autoBackupMaxFiles: appSettings.autoBackupMaxFiles,
      });
      setAppSettings(normalized || appSettings);
      const result: BackupStats = await RunAutoBackupNow();
      const skipped = result?.skippedImages ? `，跳过 ${result.skippedImages} 个图片` : "";
      setAppMessage(`已自动备份 ${result?.clips || 0} 条记录：${result?.path || ""}${skipped}`);
    } catch (err) {
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
        setAppMessage(
          `已导出诊断包：${result?.files || 0} 个文件，日志 ${formatBytes(result?.logBytes || 0)}`,
        );
      }
    } catch (err) {
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
    } catch (err) {
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
      const result: UpdateDownloadResult = await DownloadUpdatePackage(
        updateResult.downloadURL,
        updateResult.sha256 || "",
      );
      if (result?.cancelled) {
        setAppMessage("已取消下载更新包");
      } else {
        const verified = result?.verified ? "，SHA256 已校验" : "";
        setAppMessage(
          `已下载 ${formatBytes(result?.bytes || 0)}${verified}：${result?.path || ""}`,
        );
      }
    } catch (err) {
      setAppMessage(errText(err, "下载更新包失败"));
    } finally {
      setUpdateDownloading(false);
    }
  };

  const recParts = formatHotkeyParts(recModifiers, recKeyCode);

  const hasRecordedKeys = recModifiers > 0 || recKeyCode > 0;

  const canSave = recKeyCode > 0 && !saving;

  const currentParts = currentDisplay.split(/\s*\+\s*/);

  const repairStartAtLogin = async () => {
    if (repairing) return;
    setRepairing(true);
    setRepairMessage("");
    try {
      const ok = await RepairStartAtLogin();
      setAppSettings((prev) => ({ ...prev, startAtLogin: true, startAtLoginBlocked: !ok }));
      setRepairMessage(ok ? "已启用" : "仍被 Windows 阻止，请在任务管理器 → 启动 里启用");
    } catch (e) {
      setRepairMessage(errText(e, "启用失败"));
    } finally {
      setRepairing(false);
    }
  };
  return {
    appSettings,
    repairing,
    repairMessage,
    setAppMessage,
    recording,
    error,
    saving,
    recordRef,
    persistSettings,
    startRecording,
    cancelRecording,
    handleSave,
    updateBooleanSetting,
    recParts,
    hasRecordedKeys,
    canSave,
    currentParts,
    repairStartAtLogin,
    lastTargetInfo,
    rulesMessage,
    setRulesMessage,
    targetLoading,
    commitEditedSettings,
    blurOnEnter,
    refreshLastTargetInfo,
    updateNumberSetting,
    updateRuleListSetting,
    addLastTargetAppRule,
    addLastTargetTitleRule,
    stats,
    settingsMessage,
    setSettingsMessage,
    storageMessage,
    cleanupRunning,
    maintenanceRunning,
    refreshStats,
    runCleanup,
    runMaintenance,
    appInfo,
    appMessage,
    openingDataDir,
    backupRunning,
    importRunning,
    autoBackupRunning,
    selectingBackupDir,
    diagnosticsRunning,
    updateChecking,
    updateDownloading,
    updateResult,
    updateStringSetting,
    openDataDir,
    exportBackup,
    importBackup,
    selectAutoBackupDir,
    runAutoBackupNow,
    exportDiagnostics,
    checkForUpdates,
    downloadUpdatePackage,
  };
}
export type SettingsContext = ReturnType<typeof useSettingsController>;
