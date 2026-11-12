import { formatBytes } from "./settingsModel";
import type { SettingsContext } from "./useSettingsController";
export default function SettingsStorage({ context }: { context: SettingsContext }) {
  const {
    appSettings,
    stats,
    settingsMessage,
    setSettingsMessage,
    storageMessage,
    cleanupRunning,
    maintenanceRunning,
    commitEditedSettings,
    blurOnEnter,
    refreshStats,
    updateNumberSetting,
    runCleanup,
    runMaintenance,
  } = context;
  return (
    <>
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
            <div>
              <span>图片文件</span>
              <strong>{formatBytes(stats.imageBytes)}</strong>
            </div>
            <div>
              <span>数据库</span>
              <strong>{formatBytes(stats.databaseBytes)}</strong>
            </div>
            <div>
              <span>缩略图缓存</span>
              <strong>{formatBytes(stats.thumbnailBytes)}</strong>
            </div>
            {stats.missingImages > 0 && (
              <div>
                <span>缺失图片</span>
                <strong>{stats.missingImages}</strong>
              </div>
            )}
          </div>
          <div className="settings-actions inline">
            {storageMessage && <span className="save-message">{storageMessage}</span>}
            <button className="btn" onClick={refreshStats}>
              刷新
            </button>
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
    </>
  );
}
