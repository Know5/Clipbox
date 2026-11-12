import { formatRuleList, targetSummary } from "./settingsModel";
import type { SettingsContext } from "./useSettingsController";
export default function SettingsPrivacy({ context }: { context: SettingsContext }) {
  const {
    appSettings,
    lastTargetInfo,
    rulesMessage,
    setRulesMessage,
    targetLoading,
    commitEditedSettings,
    blurOnEnter,
    refreshLastTargetInfo,
    updateBooleanSetting,
    updateNumberSetting,
    updateRuleListSetting,
    addLastTargetAppRule,
    addLastTargetTitleRule,
  } = context;
  return (
    <>
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
              <div className="hotkey-description">
                每行一个进程名或路径关键词，命中前台应用时不记录
              </div>
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
              <button
                className="btn"
                onClick={() => refreshLastTargetInfo()}
                disabled={targetLoading}
              >
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
        </div>
      </div>
    </>
  );
}
