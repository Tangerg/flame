import { afterEach, describe, expect, it, vi } from "vitest";
import { createBrowserHost } from "@/platform/browserHost";
import { createClientHost } from "@/platform/clientHost";

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("browser host", () => {
  it("boots against the serving origin without assuming a local filesystem", async () => {
    const host = createClientHost();
    expect(host.kind).toBe("web");
    await expect(host.bootstrap()).resolves.toEqual({
      runtime: { endpoint: window.location.origin },
      localFilesystemEndpoint: null,
    });
    await expect(host.chooseWorkingDirectory()).resolves.toBeNull();
    await expect(host.windowChrome()).resolves.toBeNull();
    await expect(host.openPath("/remote/workspace")).resolves.toBe(false);
    await expect(host.revealPath("/remote/workspace")).resolves.toBe(false);
  });

  it("hands exact exported text and MIME to the browser download", async () => {
    vi.useFakeTimers();
    const create = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:flame-export");
    const revoke = vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
    const names: string[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      names.push(this.download);
    });
    const content = "# Captured result\n\n中文证据 🔥\n";
    const mime = "text/markdown;charset=utf-8";

    await expect(
      createBrowserHost().saveFile({
        filename: "flame-session.md",
        bytes: new TextEncoder().encode(content),
        mime,
      }),
    ).resolves.toBe("requested");

    expect(names).toEqual(["flame-session.md"]);
    const blob = create.mock.calls[0]![0] as Blob;
    expect(blob.type).toBe(mime);
    expect(await blob.text()).toBe(content);
    expect(document.querySelector('a[download="flame-session.md"]')).toBeNull();
    await vi.runAllTimersAsync();
    expect(revoke).toHaveBeenCalledWith("blob:flame-export");
  });

  it("captures arbitrary binary subarrays and reports dispatch separately from a saved file", async () => {
    vi.useFakeTimers();
    const create = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:flame-binary");
    const revoke = vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
    const names: string[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      names.push(this.download);
    });
    const backing = new Uint8Array([1, 0x50, 0x4b, 0, 0xff, 0x80, 2]);
    const saving = createBrowserHost().saveFile({
      filename: "成果包.zip",
      mime: "application/zip",
      bytes: backing.subarray(1, 6),
    });
    backing.fill(0);
    await expect(saving).resolves.toBe("requested");
    expect(names).toEqual(["成果包.zip"]);
    const blob = create.mock.calls[0]![0] as Blob;
    expect(blob.type).toBe("application/zip");
    expect(new Uint8Array(await blob.arrayBuffer())).toEqual(
      new Uint8Array([0x50, 0x4b, 0, 0xff, 0x80]),
    );
    await vi.runAllTimersAsync();
    expect(revoke).toHaveBeenCalledExactlyOnceWith("blob:flame-binary");
  });

  it("refuses destination paths and control characters before dispatching a download", async () => {
    const create = vi.spyOn(URL, "createObjectURL");
    for (const filename of [
      "",
      ".",
      "..",
      "../result.pdf",
      "dir/result.docx",
      "dir\\result.xlsx",
      "C:result.pdf",
      "result\n.pdf",
    ]) {
      await expect(
        createBrowserHost().saveFile({
          filename,
          mime: "application/pdf",
          bytes: new Uint8Array(),
        }),
      ).rejects.toThrow("basename");
    }
    expect(create).not.toHaveBeenCalled();
  });
});
