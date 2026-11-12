import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Settings from "../src/Settings";
import { DEFAULT_SETTINGS } from "../src/settingsModel";

const api = vi.hoisted(() => ({ handlers: new Map(), get: vi.fn(), patch: vi.fn() }));
vi.mock("../src/api", async () => ({
  ...(await vi.importActual("../src/api")),
  GetAppSettings: api.get,
  PatchAppSettings: api.patch,
  SetUIActivity: vi.fn(async () => {}),
  GetHotkeySettings: vi.fn(async () => ({ display: "Ctrl + Alt + V" })),
  GetAppInfo: vi.fn(async () => ({ name: "ClipBox", version: "0.4.0" })),
  GetStorageStats: vi.fn(async () => ({})),
  GetLastTargetInfo: vi.fn(async () => ({ available: false })),
}));
vi.mock("../wailsjs/runtime/runtime", () => ({
  EventsOn(name, handler) {
    api.handlers.set(name, handler);
    return () => api.handlers.delete(name);
  },
}));
let stored;
beforeEach(() => {
  stored = { ...DEFAULT_SETTINGS };
  api.handlers.clear();
  api.get.mockReset().mockImplementation(async () => ({ ...stored }));
  api.patch.mockReset().mockImplementation(async (patch) => {
    stored = { ...stored, ...patch };
    return { ...stored };
  });
});

async function openPrivacy() {
  render(<Settings onBack={() => {}} />);
  await waitFor(() => expect(api.get).toHaveBeenCalled());
  fireEvent.click(screen.getByRole("tab", { name: "隐私" }));
  return document.querySelector('input[max="5000"]');
}

describe("settings drafts", () => {
  it("patches only the edited field and preserves a tray pause", async () => {
    const input = await openPrivacy();
    await act(async () => input.focus());
    fireEvent.change(input, { target: { value: "2" } });
    stored.capturePaused = true;
    await act(async () => api.handlers.get("settings:updated")({ ...stored }));
    await act(async () => input.blur());
    await waitFor(() => expect(api.patch).toHaveBeenCalledWith({ minTextLength: 2 }));
    expect(stored.capturePaused).toBe(true);
  });
  it("keeps a newer draft when an older save returns", async () => {
    const input = await openPrivacy();
    let resolve;
    api.patch.mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    fireEvent.change(input, { target: { value: "2" } });
    fireEvent.blur(input);
    await waitFor(() => expect(api.patch).toHaveBeenCalled());
    fireEvent.change(input, { target: { value: "3" } });
    await act(async () => resolve({ ...stored, minTextLength: 2 }));
    expect(input.value).toBe("3");
  });
});
