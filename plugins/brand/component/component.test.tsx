// @vitest-environment happy-dom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { Brand } from "./component";

vi.mock("@flame/brand-assets", () => ({
  mark: "data:image/png;base64,AQID",
  companion: "data:image/png;base64,BAUG",
  plush: "data:image/png;base64,BwgJ",
  FLAME_ORANGE: "#fb6e30",
}));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function fixture() {
  const controller = new AbortController();
  const saveFile = vi
    .fn<ComponentContext["saveFile"]>()
    .mockResolvedValue("saved");
  const context = {
    locale: "en",
    signal: controller.signal,

    saveFile,
  } as unknown as ComponentContext;
  return { controller, saveFile, context };
}

it("saves the selected embedded bytes through the captured host and preserves failure", async () => {
  const f = fixture();
  f.saveFile.mockRejectedValueOnce(new Error("destination unavailable"));
  render(<Brand context={f.context} />);
  fireEvent.click(
    screen.getByRole("button", { name: "Download PNG — Primary logo" }),
  );
  await screen.findByRole("alert");
  expect(f.saveFile).toHaveBeenCalledExactlyOnceWith({
    filename: "flame-mark.png",
    mime: "image/png",
    bytes: new Uint8Array([1, 2, 3]),
  });
  expect(screen.getByRole("alert").textContent).toContain(
    "destination unavailable",
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Download PNG — 3D companion" }),
  );
  await waitFor(() => expect(f.saveFile).toHaveBeenCalledTimes(2));
  expect(f.saveFile.mock.calls[1]![0].bytes).toEqual(new Uint8Array([4, 5, 6]));
});

it("joins an accepted save through locale changes and refuses new work after retirement", async () => {
  const accepted = Promise.withResolvers<"saved">();
  const f = fixture();
  f.saveFile.mockReturnValue(accepted.promise);
  const view = render(<Brand context={f.context} />);
  fireEvent.click(
    screen.getByRole("button", { name: "Download PNG — Primary logo" }),
  );
  view.rerender(<Brand context={{ ...f.context, locale: "zh" }} />);
  expect(
    screen.getByRole("button", { name: "下载 PNG — 立体形象" }).textContent,
  ).toContain("下载 PNG");
  for (const button of screen.getAllByRole("button"))
    if (button.textContent?.includes("PNG")) fireEvent.click(button);
  expect(f.saveFile).toHaveBeenCalledOnce();
  await act(async () => accepted.resolve("saved"));
  f.controller.abort();
  for (const button of screen.getAllByRole("button")) fireEvent.click(button);
  expect(f.saveFile).toHaveBeenCalledOnce();
});

it("projects a deferred save refusal in the current language and preserves its original detail", async () => {
  const accepted = Promise.withResolvers<"saved">();
  const detail = "  Destination unavailable — 保留\n    original cause  ";
  const f = fixture();
  f.saveFile.mockReturnValue(accepted.promise);
  const view = render(<Brand context={f.context} />);
  const download = screen.getByRole("button", {
    name: "Download PNG — Primary logo",
  });
  act(() => download.focus());
  fireEvent.click(download);
  expect(f.saveFile).toHaveBeenCalledExactlyOnceWith({
    filename: "flame-mark.png",
    mime: "image/png",
    bytes: new Uint8Array([1, 2, 3]),
  });

  view.rerender(<Brand context={{ ...f.context, locale: "zh" }} />);
  expect(screen.getByRole("button", { name: "下载 PNG — 主 logo" })).toBe(
    download,
  );
  expect(document.activeElement).toBe(download);
  const later = screen.getByRole("button", { name: "复制 HEX" });
  act(() => later.focus());
  await act(async () => accepted.reject(new Error(detail)));

  expect(screen.getByRole("alert").textContent).toBe(
    `无法保存图片，请重试。 ${detail}`,
  );
  expect(document.activeElement).toBe(later);
  view.rerender(<Brand context={f.context} />);
  expect(screen.getByRole("alert").textContent).toBe(
    `Couldn't save the image. Try again. ${detail}`,
  );
  expect(
    screen.getByRole("button", { name: "Download PNG — Primary logo" }),
  ).toBe(download);
  expect(screen.getByRole("button", { name: "Copy HEX" })).toBe(later);
  expect(document.activeElement).toBe(later);
  expect(f.saveFile).toHaveBeenCalledOnce();
});

it("projects a deferred copy refusal and settled guidance in the current language", async () => {
  const accepted = Promise.withResolvers<void>();
  const writeText = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockReturnValue(accepted.promise);
  const f = fixture();
  const view = render(<Brand context={f.context} />);
  const copy = screen.getByRole("button", { name: "Copy HEX" });
  act(() => copy.focus());
  fireEvent.click(copy);
  expect(writeText).toHaveBeenCalledExactlyOnceWith("#FB6E30");

  view.rerender(<Brand context={{ ...f.context, locale: "zh" }} />);
  expect(screen.getByRole("button", { name: "复制 HEX" })).toBe(copy);
  expect(document.activeElement).toBe(copy);
  await act(async () =>
    accepted.reject(new DOMException("Clipboard refused", "NotAllowedError")),
  );

  expect(screen.getByRole("alert").textContent).toBe(
    "无法复制颜色，请选中 HEX 色值后复制。",
  );
  expect(document.activeElement).toBe(copy);
  view.rerender(<Brand context={f.context} />);
  expect(screen.getByRole("alert").textContent).toBe(
    "Couldn't copy the color. Select the HEX value to copy it.",
  );
  expect(screen.getByRole("button", { name: "Copy HEX" })).toBe(copy);
  expect(document.activeElement).toBe(copy);
  expect(writeText).toHaveBeenCalledOnce();
  expect(f.saveFile).not.toHaveBeenCalled();
});
