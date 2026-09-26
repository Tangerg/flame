import { afterEach, describe, expect, it, vi } from "vitest";
import { createBrowserHost } from "./browserHost";
import { createClientHost } from "./clientHost";

const image = "data:image/png;base64,aW1hZ2U=";

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

  it("downloads image bytes and releases its object URL", async () => {
    vi.useFakeTimers();
    const create = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:flame-image");
    const revoke = vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
    let name = "";
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      name = this.download;
    });
    await expect(createBrowserHost().saveImage(image)).resolves.toBe(true);
    expect(name).toBe("flame-image.png");
    expect(click).toHaveBeenCalledOnce();
    const blob = create.mock.calls[0]![0] as Blob;
    expect(blob.type).toBe("image/png");
    expect(await blob.text()).toBe("image");
    expect(document.querySelector('a[download="flame-image.png"]')).toBeNull();
    await vi.runAllTimersAsync();
    expect(revoke).toHaveBeenCalledWith("blob:flame-image");
  });

  it("does not turn remote URLs or executable data into downloaded images", async () => {
    const create = vi.spyOn(URL, "createObjectURL");
    for (const source of [
      "https://remote.example/image.png",
      "data:text/html;base64,aW1hZ2U=",
      "data:image/svg+xml;base64,aW1hZ2U=",
    ]) {
      await expect(createBrowserHost().saveImage(source)).rejects.toThrow("Invalid string");
    }
    expect(create).not.toHaveBeenCalled();
  });
});
