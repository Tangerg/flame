import { afterEach, describe, expect, it, vi } from "vitest";
import { copyRichText, writeToClipboard } from "@/lib/clipboard";

const present = vi.fn();

afterEach(() => {
  vi.unstubAllGlobals();
  present.mockClear();
});

describe("copyRichText", () => {
  it("writes Markdown and rendered HTML together when the Clipboard API supports it", async () => {
    const write = vi.fn().mockResolvedValue(undefined);
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { write, writeText } });

    const clipboardItem = vi.fn<(formats: Record<string, Blob>) => void>();
    vi.stubGlobal(
      "ClipboardItem",
      class ClipboardItemStub {
        constructor(formats: Record<string, Blob>) {
          clipboardItem(formats);
        }
      },
    );

    await expect(
      copyRichText({
        plainText: "| Name | Count |\n|---|---:|\n| alpha | 12 |",
        htmlText: "<table><tbody><tr><td>alpha</td><td>12</td></tr></tbody></table>",
      }),
    ).resolves.toBe(true);

    expect(write).toHaveBeenCalledOnce();
    expect(writeText).not.toHaveBeenCalled();
    const formats = clipboardItem.mock.calls[0]![0];
    await expect(formats["text/plain"]!.text()).resolves.toContain("| Name | Count |");
    await expect(formats["text/html"]!.text()).resolves.toContain("<table>");
  });
});

describe("writeToClipboard", () => {
  const text = "  第一行  two  spaces  ";
  const labels = { successLabel: "Copied", failureLabel: "Couldn't copy" };

  it("acknowledges an accepted write with the success label only", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });

    await expect(writeToClipboard(present, text, labels)).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledExactlyOnceWith(text);
    expect(present).toHaveBeenCalledExactlyOnceWith({ level: "success", message: "Copied" });
  });

  it("reports a denied write with the failure label, never the error", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("NotAllowedError: secret-token"));
    vi.stubGlobal("navigator", { clipboard: { writeText } });

    await expect(writeToClipboard(present, text, labels)).resolves.toBe(false);

    expect(writeText).toHaveBeenCalledExactlyOnceWith(text);
    expect(present).toHaveBeenCalledExactlyOnceWith({ level: "error", message: "Couldn't copy" });
  });

  it("reports an absent Clipboard API with the failure label", async () => {
    vi.stubGlobal("navigator", {});

    await expect(writeToClipboard(present, text, labels)).resolves.toBe(false);

    expect(present).toHaveBeenCalledExactlyOnceWith({ level: "error", message: "Couldn't copy" });
  });

  it("stays silent for an outcome whose label was not requested", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("denied"));
    vi.stubGlobal("navigator", { clipboard: { writeText } });

    await expect(writeToClipboard(present, text, { successLabel: "Copied" })).resolves.toBe(false);
    writeText.mockResolvedValue(undefined);
    await expect(writeToClipboard(present, text)).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledTimes(2);
    expect(present).not.toHaveBeenCalled();
  });
});
