import { QuitApp } from "./api";
import type { SettingsContext } from "./useSettingsController";
export default function SettingsAdvanced({ context }: { context: SettingsContext }) {
  const {
    appSettings,
    appInfo,
    appMessage,
    setAppMessage,
    openingDataDir,
    backupRunning,
    importRunning,
    autoBackupRunning,
    selectingBackupDir,
    diagnosticsRunning,
    updateChecking,
    updateDownloading,
    updateResult,
    commitEditedSettings,
    blurOnEnter,
    updateBooleanSetting,
    updateNumberSetting,
    updateStringSetting,
    openDataDir,
    exportBackup,
    importBackup,
    selectAutoBackupDir,
    runAutoBackupNow,
    exportDiagnostics,
    checkForUpdates,
    downloadUpdatePackage,
  } = context;
  return (
    <>
      <div className="settings-section">
        <div className="section-label">应用</div>
        <div className="section-card">
          <div className="app-info-lines">
            <div>
              <span>名称</span>
              <strong>{appInfo.name || "ClipBox"}</strong>
            </div>
            <div>
              <span>版本</span>
              <strong>{appInfo.version || "-"}</strong>
            </div>
            <div>
              <span>数据目录</span>
              <strong title={appInfo.dataDir}>{appInfo.dataDir || "-"}</strong>
            </div>
            <div>
              <span>程序路径</span>
              <strong title={appInfo.exePath}>{appInfo.exePath || "-"}</strong>
            </div>
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
                <button
                  className="btn primary"
                  onClick={downloadUpdatePackage}
                  disabled={updateDownloading}
                >
                  {updateDownloading ? "下载中..." : "下载更新包"}
                </button>
              )}
              {updateResult && (
                <div className={`update-result ${updateResult.updateAvailable ? "available" : ""}`}>
                  <strong>{updateResult.message || "检查完成"}</strong>
                  <span>
                    当前 {updateResult.currentVersion || appInfo.version || "-"} · 最新{" "}
                    {updateResult.latestVersion || "-"}
                  </span>
                  {updateResult.downloadURL && (
                    <span title={updateResult.downloadURL}>下载 {updateResult.downloadURL}</span>
                  )}
                  {updateResult.downloadURL && (
                    <span>{updateResult.sha256 ? "SHA256 可校验" : "未提供 SHA256"}</span>
                  )}
                  {updateResult.releaseNotesURL && (
                    <span title={updateResult.releaseNotesURL}>
                      说明 {updateResult.releaseNotesURL}
                    </span>
                  )}
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
              <button
                className="btn"
                onClick={exportBackup}
                disabled={backupRunning || importRunning}
              >
                {backupRunning ? "导出中..." : "导出"}
              </button>
              <button
                className="btn"
                onClick={importBackup}
                disabled={backupRunning || importRunning}
              >
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
              <button
                className="btn"
                onClick={runAutoBackupNow}
                disabled={autoBackupRunning || !appSettings.autoBackupDir}
              >
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
              <div className="hotkey-description">
                导出版本、设置、统计和运行日志，不包含剪贴板正文
              </div>
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
    </>
  );
}
