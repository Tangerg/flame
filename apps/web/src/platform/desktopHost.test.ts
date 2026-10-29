import { IMAGE_FILENAME_STEM } from "@/app/product";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "@flame/runtime-client";
import { createClientHost } from "@/platform/clientHost";
import { createDesktopHostClient, type DesktopHostBinding } from "@/platform/desktopHost";
import { DesktopMethods, NotificationOpenedEvent } from "@/platform/desktop.generated";
const wailsCall = vi.hoisted(() => vi.fn());

vi.mock("@wailsio/runtime", () => ({
  Call: { ByName: wailsCall },
  Events: { On: () => () => {} },
}));

const BOOTSTRAP = DesktopMethods.Bootstrap.name;

const CHOOSE_WORKING_DIRECTORY = DesktopMethods.ChooseWorkingDirectory.name;

const SAVE_IMAGE = DesktopMethods.SaveImage.name;

const SAVE_FILE = DesktopMethods.SaveFile.name;

const WINDOW_CHROME = DesktopMethods.WindowChrome.name;

const NOTIFICATION_AUTHORIZATION = DesktopMethods.NotificationAuthorization.name;

const SEND_NOTIFICATION = DesktopMethods.SendNotification.name;

function hostBinding(
  answers: Partial<Record<string, () => Promise<unknown>>> = {},
): DesktopHostBinding & {
  call: ReturnType<typeof vi.fn>;
  emit: (event: string, data: unknown) => void;
} {
  const defaults: Record<string, () => Promise<unknown>> = {
    [BOOTSTRAP]: async () => ({
      localRuntime: { endpoint: "http://127.0.0.1:17171" },
    }),
    [CHOOSE_WORKING_DIRECTORY]: async () => "/tmp/project",
    [SAVE_IMAGE]: async () => true,
    [SAVE_FILE]: async () => true,
    [WINDOW_CHROME]: async () => ({
      controlsCentreY: 20,
      controlsInlineEnd: 72,
      measured: true,
    }),
  };
  const routes = { ...defaults, ...answers };
  const listeners = new Map<string, Set<(data: unknown) => void>>();
  return {
    call: vi.fn(async (method: string) => {
      const answer = routes[method];
      if (!answer) throw new Error(`unbound method ${method}`);
      return answer();
    }),
    on: async (event, listener) => {
      const set = listeners.get(event) ?? new Set();
      set.add(listener);
      listeners.set(event, set);
      return () => set.delete(listener);
    },
    emit: (event, data) => listeners.get(event)?.forEach((listener) => listener(data)),
  };
}

describe("DesktopHostClient", () => {
  it("validates and caches the Wails bootstrap result", async () => {
    const binding = hostBinding({
      [BOOTSTRAP]: async () => ({
        localRuntime: { endpoint: "http://127.0.0.1:17171", localToken: "token" },
      }),
    });
    const client = createDesktopHostClient(binding);

    await expect(client.bootstrap()).resolves.toMatchObject({
      runtime: { localToken: "token" },
    });
    await client.bootstrap();
    expect(binding.call).toHaveBeenCalledTimes(1);
    expect(binding.call).toHaveBeenCalledWith(BOOTSTRAP);
  });

  it("rejects a malformed host response", async () => {
    const client = createDesktopHostClient(
      hostBinding({ [BOOTSTRAP]: async () => ({ localRuntime: {} }) }),
    );
    await expect(client.bootstrap()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("preserves bootstrap wire refinements and endpoint adaptation", async () => {
    for (const localRuntime of [
      { endpoint: "not a URL" },
      { endpoint: "http://127.0.0.1:17171", localToken: "" },
      { endpoint: "file:///tmp/runtime" },
    ]) {
      const client = createDesktopHostClient(
        hostBinding({ [BOOTSTRAP]: async () => ({ localRuntime }) }),
      );
      await expect(client.bootstrap()).rejects.toBeInstanceOf(RpcTransportError);
    }
    const client = createDesktopHostClient(
      hostBinding({
        [BOOTSTRAP]: async () => ({ localRuntime: { endpoint: "http://127.0.0.1:17171/" } }),
      }),
    );
    await expect(client.bootstrap()).resolves.toMatchObject({
      runtime: { endpoint: "http://127.0.0.1:17171" },
      localFilesystemEndpoint: "http://127.0.0.1:17171",
    });
  });

  it("keeps empty notification targets legal when sending", async () => {
    for (const message of [
      { id: "run:1", title: "Done" },
      { id: "run:1", title: "Done", target: "" },
    ]) {
      expect(DesktopMethods.SendNotification.args.safeParse([message]).success).toBe(true);
      const binding = hostBinding({ [SEND_NOTIFICATION]: async () => null });
      await expect(createDesktopHostClient(binding).sendNotification(message)).resolves.toBe(true);
    }
    expect(
      DesktopMethods.SendNotification.args.safeParse([{ id: "", title: "Done" }]).success,
    ).toBe(false);
  });

  it("returns the directory selected by the packaged host", async () => {
    const binding = hostBinding();

    await expect(createDesktopHostClient(binding).chooseWorkingDirectory()).resolves.toBe(
      "/tmp/project",
    );
    expect(binding.call).toHaveBeenCalledWith(CHOOSE_WORKING_DIRECTORY);
  });

  it("distinguishes selection cancellation from invalid host responses", async () => {
    const cancelled = hostBinding({ [CHOOSE_WORKING_DIRECTORY]: async () => "" });
    await expect(createDesktopHostClient(cancelled).chooseWorkingDirectory()).resolves.toBeNull();

    const malformed = hostBinding({ [CHOOSE_WORKING_DIRECTORY]: async () => ({ path: "/tmp" }) });
    await expect(
      createDesktopHostClient(malformed).chooseWorkingDirectory(),
    ).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("captures the exact binary range and awaits the native write or cancellation", async () => {
    const choice = Promise.withResolvers<boolean>();
    const binding = hostBinding({ [SAVE_FILE]: () => choice.promise });
    const client = createDesktopHostClient(binding);
    const backing = new Uint8Array([1, 0x50, 0x4b, 0, 0xff, 0x80, 2]);
    const bytes = backing.subarray(1, 6);
    const saving = client.saveFile({
      filename: "财务报告.xlsx",
      mime: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      bytes,
    });
    backing.fill(0);
    expect(binding.call).toHaveBeenCalledExactlyOnceWith(SAVE_FILE, "财务报告.xlsx", "UEsA/4A=");
    choice.resolve(false);
    await expect(saving).resolves.toBe("cancelled");

    const completed = hostBinding();
    await expect(
      createDesktopHostClient(completed).saveFile({
        filename: "flame-session.json",
        mime: "application/json",
        bytes: new TextEncoder().encode("{}"),
      }),
    ).resolves.toBe("saved");
  });

  it("encodes large binary payloads without argument expansion or UTF-8 replacement", async () => {
    const binding = hostBinding();
    const bytes = Uint8Array.from({ length: 200_000 }, (_, index) => index % 256);
    await createDesktopHostClient(binding).saveFile({
      filename: "bundle.zip",
      mime: "application/zip",
      bytes,
    });
    const encoded = binding.call.mock.calls[0]![2] as string;
    expect(Uint8Array.from(atob(encoded), (character) => character.charCodeAt(0))).toEqual(bytes);
  });

  it("rejects malformed file-save responses and native failures", async () => {
    const file = {
      filename: "flame.pdf",
      mime: "application/pdf",
      bytes: new Uint8Array([0, 255]),
    };
    for (const value of [undefined, null, "saved", {}, 1]) {
      const binding = hostBinding({ [SAVE_FILE]: async () => value });
      await expect(createDesktopHostClient(binding).saveFile(file)).rejects.toBeInstanceOf(
        RpcTransportError,
      );
    }
    const failed = hostBinding({
      [SAVE_FILE]: async () => {
        throw new Error("selected file could not be written");
      },
    });
    await expect(createDesktopHostClient(failed).saveFile(file)).rejects.toBeInstanceOf(
      RpcTransportError,
    );
  });

  it("saves inline images through the packaged host and preserves cancellation", async () => {
    const binding = hostBinding();
    const source = "data:image/png;base64,aW1hZ2U=";

    await expect(createDesktopHostClient(binding).saveImage(source)).resolves.toBe(true);
    expect(binding.call).toHaveBeenCalledWith(SAVE_IMAGE, source);

    const cancelled = hostBinding({ [SAVE_IMAGE]: async () => false });
    await expect(createDesktopHostClient(cancelled).saveImage(source)).resolves.toBe(false);
  });

  it("rejects malformed image-save responses", async () => {
    const malformed = hostBinding({ [SAVE_IMAGE]: async () => "saved" });
    await expect(
      createDesktopHostClient(malformed).saveImage("data:image/png;base64,aW1hZ2U="),
    ).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("reads the platform's window-control geometry", async () => {
    const binding = hostBinding();
    await expect(createDesktopHostClient(binding).windowChrome()).resolves.toEqual({
      controlsCentreY: 20,
      controlsInlineEnd: 72,
    });
    expect(binding.call).toHaveBeenCalledWith(WINDOW_CHROME);
  });

  it("answers null for every way of having nothing to measure", async () => {
    const unmeasured = hostBinding({
      [WINDOW_CHROME]: async () => ({ controlsCentreY: 0, controlsInlineEnd: 0, measured: false }),
    });
    await expect(createDesktopHostClient(unmeasured).windowChrome()).resolves.toBeNull();

    const negative = hostBinding({
      [WINDOW_CHROME]: async () => ({ controlsCentreY: -1, controlsInlineEnd: 72, measured: true }),
    });
    await expect(createDesktopHostClient(negative).windowChrome()).resolves.toBeNull();

    const malformed = hostBinding({
      [WINDOW_CHROME]: async () => ({ controlsCentreY: 20 }),
    });
    await expect(createDesktopHostClient(malformed).windowChrome()).resolves.toBeNull();

    const broken = hostBinding({
      [WINDOW_CHROME]: async () => {
        throw new Error("no window");
      },
    });
    await expect(createDesktopHostClient(broken).windowChrome()).resolves.toBeNull();
  });

  it("addresses the host by its fully qualified Go method names", async () => {
    const binding = hostBinding();
    const client = createDesktopHostClient(binding);
    await client.bootstrap();
    await client.chooseWorkingDirectory();
    await client.saveImage("data:image/png;base64,aW1hZ2U=");
    await client.windowChrome();
    expect(binding.call.mock.calls.flat()).toEqual([
      BOOTSTRAP,
      CHOOSE_WORKING_DIRECTORY,
      SAVE_IMAGE,
      "data:image/png;base64,aW1hZ2U=",
      WINDOW_CHROME,
    ]);
  });

  it("reads notification authorization and rejects a state the host never names", async () => {
    const client = createDesktopHostClient(
      hostBinding({ [NOTIFICATION_AUTHORIZATION]: async () => "granted" }),
    );
    await expect(client.notificationAuthorization()).resolves.toBe("granted");
    const broken = createDesktopHostClient(
      hostBinding({ [NOTIFICATION_AUTHORIZATION]: async () => "maybe" }),
    );
    await expect(broken.notificationAuthorization()).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("sends notifications and hands back the target a click opened", async () => {
    const binding = hostBinding({ [SEND_NOTIFICATION]: async () => null });
    const client = createDesktopHostClient(binding);
    const message = { id: "run:1", title: "Done", target: "session-1" };
    await expect(client.sendNotification(message)).resolves.toBe(true);
    expect(binding.call).toHaveBeenCalledWith(SEND_NOTIFICATION, message);

    const opened = vi.fn();
    const stop = await client.onNotificationOpened(opened);
    binding.emit(NotificationOpenedEvent, "session-1");
    binding.emit(NotificationOpenedEvent, "");
    stop();
    binding.emit(NotificationOpenedEvent, "session-2");
    expect(opened.mock.calls).toEqual([["session-1"]]);
  });

  describe("inside the packaged webview", () => {
    afterEach(() => vi.unstubAllGlobals());

    it("reaches the host before Wails has injected its runtime object", async () => {
      vi.stubGlobal("webkit", { messageHandlers: { external: { postMessage: () => {} } } });
      expect("_wails" in globalThis).toBe(false);
      wailsCall.mockResolvedValueOnce({
        localRuntime: { endpoint: "http://127.0.0.1:17171", localToken: "token" },
      });
      const bootstrap = await createClientHost(IMAGE_FILENAME_STEM).bootstrap();
      expect(bootstrap.runtime.localToken).toBe("token");
      expect(wailsCall).toHaveBeenCalledWith(BOOTSTRAP);
    });
  });
});
