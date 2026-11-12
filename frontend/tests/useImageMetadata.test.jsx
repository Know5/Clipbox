import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useImageMetadata } from "../src/useImageMetadata";

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

const image = (id) => ({
  id,
  type: "image",
  content: "",
  preview: "image",
  timestamp: id,
  pinned: false,
});
const meta = (id) => ({ id, width: 1920, height: 1080, bytes: 8294400 });

describe("image metadata requests", () => {
  it("retries a missing file on a full refresh even if its timestamp is unchanged", async () => {
    const load = vi
      .fn()
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([meta(1)]);
    const { result, rerender } = renderHook(({ clips }) => useImageMetadata(clips, load), {
      initialProps: { clips: [image(1)] },
    });
    await act(async () => {});
    rerender({ clips: [image(1)] });
    await waitFor(() => expect(result.current[1]).toEqual(meta(1)));
  });
  it("keeps a pending result when another clip changes the list", async () => {
    const request = deferred();
    const load = vi.fn(() => request.promise);
    const { result, rerender } = renderHook(({ clips }) => useImageMetadata(clips, load), {
      initialProps: { clips: [image(1)] },
    });
    await waitFor(() => expect(load).toHaveBeenCalledTimes(1));
    rerender({ clips: [image(1), { ...image(2), type: "text" }] });
    await act(async () => request.resolve([meta(1)]));
    await waitFor(() => expect(result.current[1]).toEqual(meta(1)));
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("handles overlapping batches resolving in reverse order", async () => {
    const first = deferred();
    const second = deferred();
    const load = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result, rerender } = renderHook(({ clips }) => useImageMetadata(clips, load), {
      initialProps: { clips: [image(1)] },
    });
    await waitFor(() => expect(load).toHaveBeenCalledWith([1]));
    rerender({ clips: [image(1), image(2)] });
    await waitFor(() => expect(load).toHaveBeenCalledWith([2]));
    await act(async () => second.resolve([meta(2)]));
    expect(result.current[2]).toEqual(meta(2));
    await act(async () => first.resolve([meta(1)]));
    expect(result.current).toEqual({ 1: meta(1), 2: meta(2) });
  });

  it("shares the request across StrictMode's effect cleanup", async () => {
    const request = deferred();
    const load = vi.fn(() => request.promise);
    const wrapper = ({ children }) => <React.StrictMode>{children}</React.StrictMode>;
    const { result } = renderHook(() => useImageMetadata([image(1)], load), { wrapper });
    await waitFor(() => expect(load).toHaveBeenCalledTimes(1));
    await act(async () => request.resolve([meta(1)]));
    expect(result.current[1]).toEqual(meta(1));
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("allows a failed request to retry on the next list update", async () => {
    const errorLog = vi.spyOn(console, "error").mockImplementation(() => {});
    const load = vi
      .fn()
      .mockRejectedValueOnce(new Error("temporary failure"))
      .mockResolvedValueOnce([meta(1)]);
    const { result, rerender } = renderHook(({ clips }) => useImageMetadata(clips, load), {
      initialProps: { clips: [image(1)] },
    });
    await waitFor(() => expect(errorLog).toHaveBeenCalled());
    rerender({ clips: [image(1), { ...image(2), type: "text" }] });
    await waitFor(() => expect(result.current[1]).toEqual(meta(1)));
    expect(load).toHaveBeenCalledTimes(2);
  });

  it("does not restore metadata for a removed image after a late response", async () => {
    const request = deferred();
    const load = vi.fn(() => request.promise);
    const { result, rerender } = renderHook(({ clips }) => useImageMetadata(clips, load), {
      initialProps: { clips: [image(1)] },
    });
    await waitFor(() => expect(load).toHaveBeenCalled());
    rerender({ clips: [] });
    await act(async () => request.resolve([meta(1)]));
    expect(result.current).toEqual({});
  });
});
