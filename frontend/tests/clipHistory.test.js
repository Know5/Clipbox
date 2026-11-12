import { describe, expect, it } from "vitest";
import { applyHistoryChange } from "../src/clipHistory";

const clip = (id, extra = {}) => ({
  id,
  type: "text",
  content: "text",
  preview: "text",
  timestamp: id,
  pinned: false,
  ...extra,
});

describe("history updates", () => {
  it("uses the persisted pin, note, and tags when the same content is copied again", () => {
    const existing = clip(1, { pinned: true, note: "saved note", tags: "Work" });
    const updated = { ...existing, timestamp: 100 };
    const next = applyHistoryChange([existing], { entry: updated, deletedIds: [], total: 1 });
    expect(next.items).toEqual([updated]);
    expect(next.total).toBe(1);
  });

  it("removes the records deleted by cleanup and uses the database total", () => {
    const existing = Array.from({ length: 50 }, (_, index) => clip(index + 1));
    const next = applyHistoryChange(existing, { entry: clip(51), deletedIds: [1], total: 50 });
    expect(next.items).toHaveLength(50);
    expect(next.items.some((item) => item.id === 1)).toBe(false);
    expect(next.total).toBe(50);
    expect(next.hasMore).toBe(false);
  });

  it("does not count an existing record outside the loaded page as a new database row", () => {
    const next = applyHistoryChange([clip(2), clip(3)], {
      entry: clip(1, { timestamp: 10 }),
      deletedIds: [],
      total: 100,
    });
    expect(next.items[0].id).toBe(1);
    expect(next.total).toBe(100);
    expect(next.hasMore).toBe(true);
  });

  it("keeps the loaded page capacity bounded during repeated background captures", () => {
    let items = Array.from({ length: 50 }, (_, index) => clip(index + 1));
    for (let id = 51; id <= 1000; id++) {
      items = applyHistoryChange(items, { entry: clip(id), deletedIds: [], total: 1000 }).items;
    }
    expect(items).toHaveLength(50);
    expect(items[0].id).toBe(1000);
  });

  it("retains explicitly loaded pages when an update arrives", () => {
    const existing = Array.from({ length: 100 }, (_, index) => clip(index + 1));
    const next = applyHistoryChange(existing, { entry: clip(101), deletedIds: [], total: 200 });
    expect(next.items).toHaveLength(100);
    expect(next.items[0].id).toBe(101);
  });

  it("does not display a newly captured entry immediately removed by cleanup", () => {
    const existing = clip(1, { pinned: true });
    expect(
      applyHistoryChange([existing], { entry: null, deletedIds: [2], total: 1 }).items,
    ).toEqual([existing]);
  });
});
