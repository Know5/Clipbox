import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TextToolsPanel from "../src/TextToolsPanel";

const api = vi.hoisted(() => ({
  settings: vi.fn(),
  details: vi.fn(),
  recognize: vi.fn(),
  translate: vi.fn(),
  copy: vi.fn(),
  cancel: vi.fn(),
}));
vi.mock("../src/api", () => ({
  SetUIActivity: vi.fn(async () => {}),
  GetTextToolsSettings: api.settings,
  GetClipDetails: api.details,
  RecognizeClipText: api.recognize,
  TranslateText: api.translate,
  CopyTextResult: api.copy,
  CancelTextTask: api.cancel,
}));

beforeEach(() => {
  for (const mock of Object.values(api)) mock.mockReset();
  api.settings.mockResolvedValue({
    endpoint: "https://example.com/v1",
    model: "model",
    targetLanguage: "English",
    ocrLanguage: "zh-Hans-CN",
  });
  api.details.mockResolvedValue({ content: "完整原文" });
  api.recognize.mockResolvedValue({ text: "识别文字", language: "zh-Hans-CN" });
  api.translate.mockResolvedValue("Translated text");
  api.copy.mockResolvedValue();
  api.cancel.mockResolvedValue();
});

describe("manual text tools", () => {
  it("does not recognize or translate automatically; translates the edited OCR text only on click", async () => {
    render(
      <TextToolsPanel clip={{ id: 7, type: "image" }} onClose={() => {}} onSettings={() => {}} />,
    );
    await waitFor(() => expect(screen.queryByText("加载中…")).toBeNull());
    expect(api.recognize).not.toHaveBeenCalled();
    expect(api.translate).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "识别文字" }));
    await screen.findByDisplayValue("识别文字");
    expect(api.recognize).toHaveBeenCalledWith(7, "zh-Hans-CN", expect.any(String));
    expect(api.translate).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("待翻译原文"), { target: { value: "修正后的原文" } });
    fireEvent.click(screen.getByRole("button", { name: "翻译", exact: true }));
    await screen.findByDisplayValue("Translated text");
    expect(api.translate).toHaveBeenCalledWith("修正后的原文", "English", expect.any(String));
    expect(api.copy).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "复制译文" }));
    await waitFor(() => expect(api.copy).toHaveBeenCalledWith("Translated text"));
  });

  it("loads full stored text and prevents translation without configuration", async () => {
    api.settings.mockResolvedValue({ endpoint: "", model: "" });
    render(
      <TextToolsPanel
        clip={{ id: 8, type: "text", content: "preview" }}
        onClose={() => {}}
        onSettings={() => {}}
      />,
    );
    await screen.findByDisplayValue("完整原文");
    expect(screen.getByRole("button", { name: "翻译", exact: true }).disabled).toBe(true);
    expect(screen.getByText("先配置翻译服务 →")).toBeTruthy();
    expect(api.translate).not.toHaveBeenCalled();
  });

  it("cancels the request and ignores a late result", async () => {
    let resolve;
    api.translate.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    render(
      <TextToolsPanel clip={{ id: 9, type: "text" }} onClose={() => {}} onSettings={() => {}} />,
    );
    await screen.findByDisplayValue("完整原文");
    fireEvent.click(screen.getByRole("button", { name: "翻译", exact: true }));
    fireEvent.click(await screen.findByRole("button", { name: "取消", exact: true }));
    expect(api.cancel).toHaveBeenCalledWith(expect.any(String));
    await act(async () => resolve("late result"));
    expect(screen.queryByDisplayValue("late result")).toBeNull();
  });
});
