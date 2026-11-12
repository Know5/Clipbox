import React from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "../src/App";

const mocks = vi.hoisted(() => ({
  handlers: new Map(),
  getPage: vi.fn(),
  getTags: vi.fn(),
  getDetails: vi.fn(),
  getImage: vi.fn(),
  copy: vi.fn(),
  paste: vi.fn(),
  togglePin: vi.fn(),
  deleteClip: vi.fn(),
}));

vi.mock("../src/api", async () => ({
  ...(await vi.importActual("../src/api")),
  GetClipPageByTypeAndTag: mocks.getPage,
  SearchClipPageByTypeAndTag: mocks.getPage,
  GetClipTags: mocks.getTags,
  GetClipDetails: mocks.getDetails,
  GetImageURL: mocks.getImage,
  CopyToClipboard: mocks.copy,
  CopyAndPaste: mocks.paste,
  TogglePin: mocks.togglePin,
  DeleteClip: mocks.deleteClip,
  GetAppSettings: vi.fn(async () => ({ theme: "dark" })),
  GetHotkeySettings: vi.fn(async () => ({ display: "Ctrl + Alt + V" })),
  GetWindowPinned: vi.fn(async () => false),
  FrontendReady: vi.fn(async () => {}),
  GetViewState: vi.fn(async () => ""),
  SaveViewState: vi.fn(async () => {}),
  SetUIActivity: vi.fn(async () => {}),
  GetClipImageMeta: vi.fn(async () => []),
  GetSourceIcon: vi.fn(async () => ""),
}));

vi.mock("../wailsjs/runtime/runtime", () => ({
  EventsOn(name, handler) {
    mocks.handlers.set(name, handler);
    return () => {
      if (mocks.handlers.get(name) === handler) mocks.handlers.delete(name);
    };
  },
}));
vi.mock("../src/theme", () => ({
  refreshAccent: vi.fn(async () => {}),
  syncWindowBackground: vi.fn(),
}));
vi.mock("../src/Settings", () => ({ default: () => null }));

const clip = (id, extra = {}) => ({
  id,
  type: "text",
  content: `row-${id}`,
  preview: `row-${id}`,
  timestamp: id,
  pinned: false,
  ...extra,
});
let backendItems;
let backendTotal;

function page(_type, _tag, limit, offset) {
  const items = backendItems.slice(offset, offset + limit);
  return Promise.resolve({
    items,
    total: backendTotal,
    hasMore: offset + items.length < backendTotal,
  });
}

function deferred() {
  let resolve;
  const promise = new Promise((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}

async function emit(change) {
  await act(async () => mocks.handlers.get("clips:updated")(change));
}

beforeEach(() => {
  backendItems = [];
  backendTotal = 0;
  mocks.handlers.clear();
  mocks.getPage.mockReset().mockImplementation(page);
  mocks.getTags.mockReset().mockResolvedValue([]);
  mocks.getDetails
    .mockReset()
    .mockImplementation(async (id) => ({ ...backendItems.find((item) => item.id === id) }));
  mocks.getImage.mockReset().mockResolvedValue("/clip-image/session/image.bmp");
  mocks.copy.mockReset().mockResolvedValue(undefined);
  mocks.paste.mockReset().mockResolvedValue(undefined);
  mocks.togglePin.mockReset().mockImplementation(async (id) => {
    backendItems = backendItems.map((item) =>
      item.id === id ? { ...item, pinned: !item.pinned } : item,
    );
    backendItems.sort(
      (left, right) =>
        Number(right.pinned) - Number(left.pinned) || right.timestamp - left.timestamp,
    );
    mocks.handlers.get("clips:updated")({
      entry: backendItems.find((item) => item.id === id),
      deletedIds: [],
      total: backendTotal,
    });
  });
  mocks.deleteClip.mockReset().mockImplementation(async (id) => {
    backendItems = backendItems.filter((item) => item.id !== id);
    backendTotal = backendItems.length;
    mocks.handlers.get("clips:updated")({ entry: null, deletedIds: [id], total: backendTotal });
  });
});

describe("live history list", () => {
  it("keeps the pin and the actual total when an existing entry is copied again", async () => {
    backendItems = [clip(1, { pinned: true, note: "saved note", tags: "Work" })];
    backendTotal = 1;
    render(<App />);
    await screen.findByText("row-1");
    const updated = { ...backendItems[0], timestamp: 100 };
    backendItems = [updated];
    await emit({ entry: updated, deletedIds: [], total: 1 });
    expect(screen.getByRole("option").classList.contains("pinned")).toBe(true);
    expect(screen.getByText("已加载 1 / 1")).toBeTruthy();
    expect(mocks.getPage).toHaveBeenCalledTimes(1);
  });

  it("removes cleaned rows from the rendered list", async () => {
    backendItems = Array.from({ length: 50 }, (_, index) => clip(50 - index));
    backendTotal = 50;
    render(<App />);
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(50));
    backendItems = [clip(51), ...backendItems.filter((item) => item.id !== 1)];
    await emit({ entry: clip(51), deletedIds: [1], total: 50 });
    expect(screen.queryByText("row-1")).toBeNull();
    expect(screen.getAllByRole("option")).toHaveLength(50);
    expect(screen.getByText("已加载 50 / 50")).toBeTruthy();
  });

  it("does not let an old initial response resurrect a deleted record", async () => {
    const oldRequest = deferred();
    mocks.getPage.mockReturnValueOnce(oldRequest.promise);
    render(<App />);
    await waitFor(() => expect(mocks.getPage).toHaveBeenCalledTimes(1));
    backendItems = [clip(2)];
    backendTotal = 1;
    await emit({ entry: clip(2), deletedIds: [1], total: 1 });
    await screen.findByText("row-2");
    await act(async () => oldRequest.resolve({ items: [clip(1)], total: 1, hasMore: false }));
    expect(screen.queryByText("row-1")).toBeNull();
    expect(screen.getByText("row-2")).toBeTruthy();
  });

  it("restarts an in-flight next page at the updated offset", async () => {
    backendItems = Array.from({ length: 120 }, (_, index) => clip(120 - index));
    backendTotal = 120;
    const oldItems = backendItems.slice(50, 100);
    const oldRequest = deferred();
    let delayed = false;
    mocks.getPage.mockImplementation((type, tag, limit, offset) => {
      if (offset === 50 && !delayed) {
        delayed = true;
        return oldRequest.promise;
      }
      return page(type, tag, limit, offset);
    });
    render(<App />);
    await screen.findByText("row-120");
    fireEvent.click(screen.getByRole("button", { name: "加载更多" }));
    await waitFor(() => expect(mocks.getPage).toHaveBeenCalledTimes(2));
    backendItems = [clip(121), ...backendItems];
    backendTotal = 121;
    await emit({ entry: clip(121), deletedIds: [], total: 121 });
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(100));
    await act(async () => oldRequest.resolve({ items: oldItems, total: 120, hasMore: true }));
    expect(screen.getByText("row-71")).toBeTruthy();
    expect(screen.queryByText("row-21")).toBeNull();
    expect(screen.getByText("已加载 100 / 121")).toBeTruthy();
  });

  it("does not subtract a deletion again after capture already supplied the total", async () => {
    backendItems = [clip(2), clip(1)];
    backendTotal = 2;
    const deletion = deferred();
    mocks.deleteClip.mockReturnValueOnce(deletion.promise);
    render(<App />);
    const row = (await screen.findByText("row-1")).closest('[role="option"]');
    fireEvent.click(within(row).getByTitle("删除 (Del)"));
    fireEvent.click(within(row).getByTitle("再次点击删除"));
    await waitFor(() => expect(mocks.deleteClip).toHaveBeenCalledWith(1));
    backendItems = [clip(3), clip(2)];
    await emit({ entry: clip(3), deletedIds: [], total: 2 });
    await act(async () => deletion.resolve());
    expect(screen.getByText("已加载 2 / 2")).toBeTruthy();
    expect(screen.queryByText("row-1")).toBeNull();
  });

  it("keeps the latest pin event when an older details response arrives", async () => {
    backendItems = [clip(2), clip(1)];
    backendTotal = 2;
    const oldDetails = deferred();
    mocks.getDetails.mockReturnValueOnce(oldDetails.promise);
    render(<App />);
    const row = (await screen.findByText("row-1")).closest('[role="option"]');
    fireEvent.click(within(row).getByTitle("置顶 (P)"));
    await waitFor(() => expect(row.classList.contains("pinned")).toBe(true));
    fireEvent.click(within(row).getByTitle("取消置顶 (P)"));
    await waitFor(() => expect(row.classList.contains("pinned")).toBe(false));
    await act(async () => oldDetails.resolve(clip(1, { pinned: true })));
    expect(row.classList.contains("pinned")).toBe(false);
  });

  it("does not restore a manually deleted record from an earlier refresh", async () => {
    backendItems = [clip(2), clip(1)];
    backendTotal = 2;
    render(<App />);
    const row = (await screen.findByText("row-1")).closest('[role="option"]');
    const refresh = deferred();
    mocks.getPage.mockReturnValueOnce(refresh.promise);
    await act(async () => mocks.handlers.get("clips:changed")());
    fireEvent.click(within(row).getByTitle("删除 (Del)"));
    fireEvent.click(within(row).getByTitle("再次点击删除"));
    await waitFor(() => expect(screen.queryByText("row-1")).toBeNull());
    await act(async () => refresh.resolve({ items: [clip(2), clip(1)], total: 2, hasMore: false }));
    expect(screen.queryByText("row-1")).toBeNull();
    expect(screen.getByText("已加载 1 / 1")).toBeTruthy();
  });

  it("refreshes the page boundary when an old pinned row moves outside the loaded list", async () => {
    backendItems = [
      clip(1, { pinned: true }),
      ...Array.from({ length: 120 }, (_, index) => clip(121 - index)),
    ];
    backendTotal = 121;
    render(<App />);
    const oldPinned = (await screen.findByText("row-1")).closest('[role="option"]');
    fireEvent.click(within(oldPinned).getByTitle("取消置顶 (P)"));
    await screen.findByText("row-72");
    expect(screen.queryByText("row-1")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "加载更多" }));
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(100));
    expect(screen.getByText("row-22")).toBeTruthy();
    expect(screen.getByText("已加载 100 / 121")).toBeTruthy();
  });

  it("cancels a pending next page when the popup is shown again", async () => {
    backendItems = Array.from({ length: 120 }, (_, index) => clip(120 - index));
    backendTotal = 120;
    const nextPage = deferred();
    mocks.getPage.mockImplementation((type, tag, limit, offset) =>
      offset === 50 ? nextPage.promise : page(type, tag, limit, offset),
    );
    render(<App />);
    await screen.findByText("row-120");
    fireEvent.click(screen.getByRole("button", { name: "加载更多" }));
    await waitFor(() => expect(mocks.getPage).toHaveBeenCalledTimes(2));
    await act(async () => mocks.handlers.get("window:shown")());
    await act(async () =>
      nextPage.resolve({ items: backendItems.slice(50, 100), total: 120, hasMore: true }),
    );
    expect(screen.getAllByRole("option")).toHaveLength(50);
    expect(screen.getByText("已加载 50 / 120")).toBeTruthy();
    expect(screen.getByRole("button", { name: "加载更多" }).disabled).toBe(false);
  });

  it("routes Alt+number to paste and Ctrl+Enter to copy", async () => {
    backendItems = [clip(1)];
    backendTotal = 1;
    render(<App />);
    await screen.findByText("row-1");
    const input = screen.getByPlaceholderText("搜索剪贴板历史…");
    fireEvent.keyDown(input, { key: "1", altKey: true });
    await waitFor(() => expect(mocks.paste).toHaveBeenCalledWith(1));
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    await waitFor(() => expect(mocks.copy).toHaveBeenCalledWith(1));
  });

  it("shows failure when the backend cannot complete paste", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    backendItems = [clip(1)];
    backendTotal = 1;
    mocks.paste.mockRejectedValueOnce("目标窗口未获得焦点");
    render(<App />);
    await screen.findByText("row-1");
    fireEvent.click(screen.getByRole("option"));
    await waitFor(() =>
      expect(within(screen.getByRole("option")).getByText("粘贴失败")).toBeTruthy(),
    );
  });

  it("does not load tag aggregation during initial display or pagination", async () => {
    backendItems = Array.from({ length: 100 }, (_, index) => clip(100 - index));
    backendTotal = 100;
    render(<App />);
    await screen.findByText("row-100");
    fireEvent.click(screen.getByRole("button", { name: "加载更多" }));
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(100));
    expect(mocks.getTags).not.toHaveBeenCalled();
  });

  it("keeps list loading independent from a stalled tag request", async () => {
    backendItems = [clip(1)];
    backendTotal = 1;
    const stalled = deferred();
    mocks.getTags.mockReturnValue(stalled.promise);
    render(<App />);
    await screen.findByText("row-1");
    fireEvent.click(screen.getByRole("button", { name: "标签" }));
    await waitFor(() => expect(mocks.getTags).toHaveBeenCalledTimes(1));
    backendItems = [clip(2), clip(1)];
    backendTotal = 2;
    await emit({ entry: clip(2), deletedIds: [], total: 2 });
    await screen.findByText("row-2");
    expect(screen.queryByText("加载历史失败")).toBeNull();
  });

  it("ignores tag results from a closed dropdown", async () => {
    backendItems = [clip(1)];
    backendTotal = 1;
    const tags = deferred();
    mocks.getTags.mockReturnValueOnce(tags.promise);
    render(<App />);
    await screen.findByText("row-1");
    fireEvent.click(screen.getByRole("button", { name: "标签" }));
    await waitFor(() => expect(mocks.getTags).toHaveBeenCalledTimes(1));
    fireEvent.keyDown(window, { key: "Escape" });
    await act(async () => tags.resolve([{ name: "old-tag", count: 1 }]));
    expect(screen.queryByText("old-tag")).toBeNull();
  });

  it("focuses search immediately when shown", async () => {
    backendItems = [clip(1)];
    backendTotal = 1;
    render(<App />);
    await screen.findByText("row-1");
    await act(async () => mocks.handlers.get("window:shown")());
    expect(document.activeElement).toBe(screen.getByPlaceholderText("搜索剪贴板历史…"));
  });

  it("does not reopen closed details when their delayed request resolves", async () => {
    backendItems = [clip(1)];
    backendTotal = 1;
    const pending = deferred();
    mocks.getDetails.mockReturnValueOnce(pending.promise);
    render(<App />);
    const row = (await screen.findByText("row-1")).closest('[role="option"]');
    fireEvent.click(within(row).getByTitle("查看详情 (Space)"));
    await waitFor(() => expect(mocks.getDetails).toHaveBeenCalled());
    fireEvent.keyDown(window, { key: "Escape" });
    await act(async () => pending.resolve(clip(1)));
    expect(document.querySelector(".detail-panel")).toBeNull();
  });

  it("keeps the latest image URL when a prior request resolves late", async () => {
    backendItems = [clip(2, { type: "image" }), clip(1, { type: "image" })];
    backendTotal = 2;
    const older = deferred();
    mocks.getImage.mockReturnValueOnce(older.promise).mockResolvedValueOnce("/clip-image/new.bmp");
    render(<App />);
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
    fireEvent.mouseEnter(document.querySelector('[data-clip-id="1"]'));
    fireEvent.click(
      within(document.querySelector('[data-clip-id="1"]')).getByTitle("查看详情 (Space)"),
    );
    await waitFor(() => expect(mocks.getImage).toHaveBeenCalledWith(1));
    fireEvent.click(document.querySelector(".detail-close"));
    fireEvent.mouseEnter(document.querySelector('[data-clip-id="2"]'));
    fireEvent.click(
      within(document.querySelector('[data-clip-id="2"]')).getByTitle("查看详情 (Space)"),
    );
    await waitFor(() =>
      expect(screen.getByAltText("clip-detail").getAttribute("src")).toBe("/clip-image/new.bmp"),
    );
    await act(async () => older.resolve("/clip-image/old.bmp"));
    expect(screen.getByAltText("clip-detail").getAttribute("src")).toBe("/clip-image/new.bmp");
  });

  it("releases detail image state when hidden", async () => {
    backendItems = [clip(1, { type: "image" })];
    backendTotal = 1;
    render(<App />);
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(1));
    fireEvent.click(screen.getByTitle("查看详情 (Space)"));
    await screen.findByAltText("clip-detail");
    await act(async () => mocks.handlers.get("window:hidden")());
    expect(screen.queryByAltText("clip-detail")).toBeNull();
  });

  it("preserves pagination after releasing fully loaded pages on hide", async () => {
    backendItems = Array.from({ length: 100 }, (_, index) => clip(100 - index));
    backendTotal = 100;
    render(<App />);
    await screen.findByText("row-100");
    fireEvent.click(screen.getByRole("button", { name: "加载更多" }));
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(100));
    await act(async () => mocks.handlers.get("window:hidden")());
    await act(async () => mocks.handlers.get("window:shown")());
    expect(screen.getAllByRole("option")).toHaveLength(50);
    fireEvent.click(screen.getByRole("button", { name: "加载更多" }));
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(100));
  });
});
