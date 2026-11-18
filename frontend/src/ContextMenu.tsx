import type { ClipEntry } from "./clipHistory";

export type CtxMenu =
  | { x: number; y: number; kind: "search" }
  | { x: number; y: number; kind: "item"; clipId: number };

interface ContextMenuProps {
  menu: CtxMenu;
  clips: ClipEntry[];
  onClose: () => void;
  onPasteSearch: () => void;
  onSelectSearch: () => void;
  onClearSearch: () => void;
  onCopy: (id: number) => void;
  onCopyAndPaste: (id: number) => void;
  onPin: (id: number) => void;
  onOpenDetail: (id: number) => void;
  onOpenTextTools: (clip: ClipEntry) => void;
  onDelete: (id: number) => void;
}

export default function ContextMenu({
  menu,
  clips,
  onClose,
  onPasteSearch,
  onSelectSearch,
  onClearSearch,
  onCopy,
  onCopyAndPaste,
  onPin,
  onOpenDetail,
  onOpenTextTools,
  onDelete,
}: ContextMenuProps) {
  const targetClip =
    menu.kind === "item" ? clips.find((item) => item.id === menu.clipId) : undefined;

  return (
    <div
      className="ctx-menu"
      style={{
        left: Math.min(menu.x, window.innerWidth - 170),
        top: Math.max(0, Math.min(menu.y, window.innerHeight - 260)),
      }}
      onMouseDown={(e) => e.stopPropagation()}
      onContextMenu={(e) => e.preventDefault()}
    >
      {menu.kind === "search" ? (
        <>
          <button className="ctx-item" onClick={onPasteSearch}>
            粘贴
          </button>
          <button
            className="ctx-item"
            onClick={() => {
              onSelectSearch();
              onClose();
            }}
          >
            全选
          </button>
          <button
            className="ctx-item"
            onClick={() => {
              onClearSearch();
              onClose();
            }}
          >
            清空搜索
          </button>
        </>
      ) : (
        <>
          <button
            className="ctx-item"
            onClick={() => {
              onCopy(menu.clipId);
              onClose();
            }}
          >
            复制
          </button>
          <button
            className="ctx-item"
            onClick={() => {
              onCopyAndPaste(menu.clipId);
              onClose();
            }}
          >
            复制并粘贴
          </button>
          <button
            className="ctx-item"
            onClick={() => {
              onPin(menu.clipId);
              onClose();
            }}
          >
            置顶 / 取消置顶
          </button>
          <button
            className="ctx-item"
            onClick={() => {
              onOpenDetail(menu.clipId);
              onClose();
            }}
          >
            详情
          </button>
          <button
            className="ctx-item"
            onClick={() => {
              if (targetClip) onOpenTextTools(targetClip);
              onClose();
            }}
          >
            {targetClip?.type === "image" ? "识别文字 / 翻译" : "翻译文字"}
          </button>
          <button
            className="ctx-item danger"
            onClick={() => {
              onDelete(menu.clipId);
              onClose();
            }}
          >
            删除
          </button>
        </>
      )}
    </div>
  );
}
