import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TextToolsSettings from "../src/TextToolsSettings";

const api = vi.hoisted(() => ({ get: vi.fn(), languages: vi.fn(), save: vi.fn() }));
vi.mock("../src/api", () => ({
  SetUIActivity: vi.fn(async () => {}),
  GetTextToolsSettings: api.get,
  GetOCRLanguages: api.languages,
  SaveTextToolsSettings: api.save,
}));

beforeEach(() => {
  api.get.mockReset().mockResolvedValue({
    endpoint: "https://example.com/v1",
    model: "model",
    targetLanguage: "English",
    ocrLanguage: "",
    hasAPIKey: true,
  });
  api.languages.mockReset().mockResolvedValue([{ tag: "zh-Hans-CN", name: "中文" }]);
  api.save.mockReset().mockImplementation(async (settings) => ({
    ...settings,
    apiKey: "",
    hasAPIKey: !settings.clearAPIKey,
  }));
});

describe("text tool settings", () => {
  it("saves only explicitly and uses an empty key to retain the protected key", async () => {
    render(<TextToolsSettings />);
    await screen.findByDisplayValue("model");
    const endpoint = screen.getByLabelText("接口地址");
    fireEvent.change(endpoint, { target: { value: "https://example.com/custom/v1" } });
    fireEvent.blur(endpoint);
    expect(api.save).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "保存识别与翻译设置" }));
    await screen.findByText("已保存");
    expect(api.save).toHaveBeenCalledWith(
      expect.objectContaining({
        endpoint: "https://example.com/custom/v1",
        apiKey: "",
        clearAPIKey: false,
      }),
    );
  });

  it("clears an existing key only when selected and saved", async () => {
    render(<TextToolsSettings />);
    await screen.findByDisplayValue("model");
    fireEvent.click(screen.getByRole("checkbox", { name: "移除已保存的 Key" }));
    expect(api.save).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "保存识别与翻译设置" }));
    await waitFor(() =>
      expect(api.save).toHaveBeenCalledWith(
        expect.objectContaining({ apiKey: "", clearAPIKey: true }),
      ),
    );
    expect(screen.queryByText("API Key · 已保存")).toBeNull();
  });
});
