import type { ClipTag } from "./clipHistory";

interface ToolbarPopoversProps {
  openPanel: "tags" | "help" | "more" | null;
  selectedTag: string;
  tagsLoading: boolean;
  tagsError: string;
  visibleTags: ClipTag[];
  hotkeyDisplay: string;
  clearConfirming: boolean;
  clearing: boolean;
  onSelectTag: (tag: string) => void;
  onClose: () => void;
  onClear: () => void;
}

export default function ToolbarPopovers({
  openPanel,
  selectedTag,
  tagsLoading,
  tagsError,
  visibleTags,
  hotkeyDisplay,
  clearConfirming,
  clearing,
  onSelectTag,
  onClose,
  onClear,
}: ToolbarPopoversProps) {
  if (!openPanel) return null;

  if (openPanel === "tags") {
    return (
      <div className="popover" role="dialog" aria-label="标签筛选">
        <div className="popover-title">
          <span>按标签筛选</span>
          {selectedTag && (
            <button
              className="toolbar-btn"
              onClick={() => {
                onSelectTag("");
                onClose();
              }}
            >
              清除
            </button>
          )}
        </div>
        {tagsLoading ? (
          <div className="empty-hint">加载标签…</div>
        ) : tagsError ? (
          <div className="empty-hint">{tagsError}</div>
        ) : visibleTags.length === 0 ? (
          <div className="empty-hint">还没有标签，详情里可以给记录加标签</div>
        ) : (
          <div className="tag-chips">
            <button
              className={`tag-chip ${selectedTag === "" ? "active" : ""}`}
              onClick={() => {
                onSelectTag("");
                onClose();
              }}
            >
              全部标签
            </button>
            {visibleTags.map((tag) => (
              <button
                key={tag.name}
                className={`tag-chip ${selectedTag === tag.name ? "active" : ""}`}
                onClick={() => {
                  onSelectTag(selectedTag === tag.name ? "" : tag.name);
                  onClose();
                }}
                title={tag.count ? `${tag.name} (${tag.count})` : tag.name}
              >
                <span>{tag.name}</span>
                {tag.count > 0 && <strong>{tag.count}</strong>}
              </button>
            ))}
          </div>
        )}
      </div>
    );
  }

  if (openPanel === "help") {
    return (
      <div className="popover" role="dialog" aria-label="快捷键">
        <div className="popover-title">
          <span>快捷键</span>
          <button className="toolbar-btn" onClick={onClose}>
            关闭
          </button>
        </div>
        <div className="help-grid">
          <span>
            <kbd>{hotkeyDisplay}</kbd>
          </span>
          <span>唤起 / 隐藏 ClipBox</span>
          <span>
            <kbd>↑</kbd>
            <kbd>↓</kbd>
          </span>
          <span>
            选择条目（按 <kbd>Tab</kbd> 移出搜索框后可用单键操作）
          </span>
          <span>
            <kbd>Enter</kbd>
          </span>
          <span>复制并粘贴回原窗口</span>
          <span>
            <kbd>Ctrl</kbd>
            <kbd>Enter</kbd>
          </span>
          <span>只复制</span>
          <span>
            <kbd>Ctrl</kbd>
            <kbd>1</kbd>–<kbd>9</kbd>
          </span>
          <span>复制第 n 条</span>
          <span>
            <kbd>Alt</kbd>
            <kbd>1</kbd>–<kbd>9</kbd>
          </span>
          <span>复制并粘贴第 n 条</span>
          <span>
            <kbd>Home</kbd>
          </span>
          <span>回到最新（列表顶部）</span>
          <span>
            <kbd>Space</kbd>
          </span>
          <span>详情、备注与标签</span>
          <span>
            <kbd>P</kbd>
          </span>
          <span>置顶 / 取消置顶</span>
          <span>
            <kbd>Del</kbd>
          </span>
          <span>删除（再按一次确认）</span>
          <span>
            <kbd>Esc</kbd>
          </span>
          <span>关闭</span>
        </div>
      </div>
    );
  }

  return (
    <div className="popover" role="dialog" aria-label="更多操作">
      <div className="menu-list">
        <button
          className={`menu-item danger ${clearConfirming ? "confirming" : ""}`}
          onClick={onClear}
          disabled={clearing}
        >
          {clearing ? "清空中…" : clearConfirming ? "再次点击确认清空" : "清空未置顶记录"}
        </button>
      </div>
    </div>
  );
}
