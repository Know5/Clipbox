import { applyThemeNow } from "./settingsModel";
import type { SettingsContext } from "./useSettingsController";
export default function SettingsGeneral({ context }: { context: SettingsContext }) {
  const {
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
  } = context;
  return (
    <>
      <div className="settings-section">
        <div className="section-label">外观</div>
        <div className="section-card">
          <div className="setting-row">
            <div>
              <div className="hotkey-label">主题</div>
              <div className="hotkey-description">深色、浅色或跟随 Windows 系统</div>
            </div>
            <div className="theme-options">
              {(
                [
                  ["dark", "深色"],
                  ["light", "浅色"],
                  ["system", "跟随系统"],
                ] as const
              ).map(([value, label]) => (
                <button
                  key={value}
                  className={`btn ${appSettings.theme === value ? "active" : ""}`}
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
        <div className="section-label">启动与粘贴</div>
        <div className="section-card">
          <label className="setting-row">
            <div>
              <div className="hotkey-label">隐藏后保留界面</div>
              <div className="hotkey-description">
                到时释放界面内存，后台继续记录。未保存的修改会保留。
              </div>
            </div>
            <select
              className="url-input"
              style={{ width: 100 }}
              value={appSettings.uiIdleSeconds}
              onChange={(event) =>
                persistSettings(
                  { ...appSettings, uiIdleSeconds: Number(event.target.value) },
                  appSettings,
                  setAppMessage,
                )
              }
            >
              {[15, 60, 180, 600].map((seconds) => (
                <option key={seconds} value={seconds}>
                  {seconds < 60 ? `${seconds} 秒` : `${seconds / 60} 分钟`}
                </option>
              ))}
            </select>
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
          {appSettings.startAtLogin && appSettings.startAtLoginBlocked && (
            <div className="setting-row stack">
              <div>
                <div className="hotkey-label">Windows 禁用了这个启动项</div>
                <div className="hotkey-description">
                  任务管理器 →「启动」里 ClipBox 被关掉了，登录时会被跳过。点「启用启动项」写回 Run
                  值并允许它运行。
                </div>
              </div>
              <div className="settings-actions inline">
                {repairMessage && <span className="save-message">{repairMessage}</span>}
                <button className="btn" onClick={repairStartAtLogin} disabled={repairing}>
                  {repairing ? "处理中…" : "启用启动项"}
                </button>
              </div>
            </div>
          )}
          <label className="setting-row">
            <div>
              <div className="hotkey-label">复制时顺带粘贴</div>
              <div className="hotkey-description">
                仅影响 Ctrl+回车 与 Ctrl+数字；回车/单击始终粘贴
              </div>
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
        <div className="section-label">快捷键</div>
        <div className="section-card">
          <div className="hotkey-row">
            <div>
              <div className="hotkey-label">唤起快捷键</div>
              <div className="hotkey-description">设置全局快捷键以唤起 ClipBox</div>
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
                <span className="record-placeholder recording-text">请按下新的快捷键组合...</span>
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
                <span className="record-placeholder">点击「录制」按钮开始设置新的快捷键</span>
              )}
            </div>

            <div className="settings-actions">
              {!recording && !hasRecordedKeys && (
                <button className="btn record-btn" onClick={startRecording}>
                  录制
                </button>
              )}
              {recording && (
                <button className="btn record-btn recording" onClick={cancelRecording}>
                  取消录制
                </button>
              )}
              {!recording && hasRecordedKeys && (
                <>
                  <button className="btn" onClick={cancelRecording}>
                    取消
                  </button>
                  <button className="btn primary" onClick={handleSave} disabled={!canSave}>
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
    </>
  );
}
