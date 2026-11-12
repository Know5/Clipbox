import { lazy, Suspense, useState } from "react";
import "./Settings.css";
import SettingsAdvanced from "./SettingsAdvanced";
import SettingsGeneral from "./SettingsGeneral";
import type { SettingsTab } from "./settingsModel";
import SettingsPrivacy from "./SettingsPrivacy";
import SettingsStorage from "./SettingsStorage";
import { useSettingsController } from "./useSettingsController";
const TextToolsSettings = lazy(() => import("./TextToolsSettings"));
interface SettingsProps {
  onBack: () => void;
  initialTab?: SettingsTab;
}
export default function Settings({ onBack, initialTab = "general" }: SettingsProps) {
  const context = useSettingsController();
  const [tab, setTab] = useState<SettingsTab>(initialTab);
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

      <div className="settings-tabs" role="tablist" aria-label="设置分类">
        {(
          [
            ["general", "常规"],
            ["privacy", "隐私"],
            ["storage", "存储"],
            ["textTools", "识别与翻译"],
            ["advanced", "高级"],
          ] as [SettingsTab, string][]
        ).map(([value, label]) => (
          <button
            key={value}
            role="tab"
            aria-selected={tab === value}
            className={`settings-tab ${tab === value ? "active" : ""}`}
            onClick={() => setTab(value)}
          >
            {label}
          </button>
        ))}
      </div>

      <div className="settings-content">
        {tab === "textTools" && (
          <Suspense fallback={<div className="text-tools-hint">加载识别与翻译设置…</div>}>
            <TextToolsSettings />
          </Suspense>
        )}

        {tab === "general" && <SettingsGeneral context={context} />}

        {tab === "privacy" && <SettingsPrivacy context={context} />}

        {tab === "storage" && <SettingsStorage context={context} />}

        {tab === "advanced" && <SettingsAdvanced context={context} />}
      </div>
    </div>
  );
}
