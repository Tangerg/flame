import { afterEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { copyRichText, writeToClipboard } from "@/lib/clipboard";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

afterEach(() => {
  vi.unstubAllGlobals();
  vi.mocked(toast.success).mockClear();
  vi.mocked(toast.error).mockClear();
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

    await expect(writeToClipboard(text, labels)).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledExactlyOnceWith(text);
    expect(toast.success).toHaveBeenCalledExactlyOnceWith("Copied");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("reports a denied write with the failure label, never the error", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("NotAllowedError: secret-token"));
    vi.stubGlobal("navigator", { clipboard: { writeText } });

    await expect(writeToClipboard(text, labels)).resolves.toBe(false);

    expect(writeText).toHaveBeenCalledExactlyOnceWith(text);
    expect(toast.success).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalledExactlyOnceWith("Couldn't copy");
  });

  it("reports an absent Clipboard API with the failure label", async () => {
    vi.stubGlobal("navigator", {});

    await expect(writeToClipboard(text, labels)).resolves.toBe(false);

    expect(toast.error).toHaveBeenCalledExactlyOnceWith("Couldn't copy");
  });

  it("stays silent for an outcome whose label was not requested", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("denied"));
    vi.stubGlobal("navigator", { clipboard: { writeText } });

    await expect(writeToClipboard(text, { successLabel: "Copied" })).resolves.toBe(false);
    writeText.mockResolvedValue(undefined);
    await expect(writeToClipboard(text)).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledTimes(2);
    expect(toast.success).not.toHaveBeenCalled();
    expect(toast.error).not.toHaveBeenCalled();
  });
});
