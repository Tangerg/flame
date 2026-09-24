import { describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "./errors";
import { createDesktopHostClient, type DesktopHostBinding } from "./desktopHost";

const BOOTSTRAP = "main.DesktopHost.Bootstrap";
const CHOOSE_WORKING_DIRECTORY = "main.DesktopHost.ChooseWorkingDirectory";
const SAVE_IMAGE = "main.DesktopHost.SaveImage";
const WINDOW_CHROME = "main.DesktopHost.WindowChrome";
const NOTIFICATION_AUTHORIZATION = "main.DesktopHost.NotificationAuthorization";
const SEND_NOTIFICATION = "main.DesktopHost.SendNotification";

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
    on: (event, listener) => {
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
      localRuntime: { localToken: "token" },
    });
    await client.bootstrap();
    expect(binding.call).toHaveBeenCalledTimes(1);
    expect(binding.call).toHaveBeenCalledWith(BOOTSTRAP);
  });

  it("returns null outside Wails", async () => {
    await expect(createDesktopHostClient(undefined).bootstrap()).resolves.toBeNull();
  });

  it("rejects a malformed host response", async () => {
    const client = createDesktopHostClient(
      hostBinding({ [BOOTSTRAP]: async () => ({ localRuntime: {} }) }),
    );
    await expect(client.bootstrap()).rejects.toBeInstanceOf(RpcTransportError);
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

  it("returns null outside Wails instead of inventing a working directory", async () => {
    await expect(createDesktopHostClient(undefined).chooseWorkingDirectory()).resolves.toBeNull();
  });

  it("saves inline images through the packaged host and preserves cancellation", async () => {
    const binding = hostBinding();
    const source = "data:image/png;base64,aW1hZ2U=";

    await expect(createDesktopHostClient(binding).saveImage(source)).resolves.toBe(true);
    expect(binding.call).toHaveBeenCalledWith(SAVE_IMAGE, source);

    const cancelled = hostBinding({ [SAVE_IMAGE]: async () => false });
    await expect(createDesktopHostClient(cancelled).saveImage(source)).resolves.toBe(false);
  });

  it("rejects malformed image-save responses and stays inert outside Wails", async () => {
    const malformed = hostBinding({ [SAVE_IMAGE]: async () => "saved" });
    await expect(
      createDesktopHostClient(malformed).saveImage("data:image/png;base64,aW1hZ2U="),
    ).rejects.toBeInstanceOf(RpcTransportError);
    await expect(
      createDesktopHostClient(undefined).saveImage("data:image/png;base64,aW1hZ2U="),
    ).resolves.toBe(false);
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
    await expect(createDesktopHostClient(undefined).windowChrome()).resolves.toBeNull();

    const unmeasured = hostBinding({
      [WINDOW_CHROME]: async () => ({ controlsCentreY: 0, controlsInlineEnd: 0, measured: false }),
    });
    await expect(createDesktopHostClient(unmeasured).windowChrome()).resolves.toBeNull();

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
    binding.emit("desktop:notification-opened", "session-1");
    binding.emit("desktop:notification-opened", "");
    stop();
    binding.emit("desktop:notification-opened", "session-2");
    expect(opened.mock.calls).toEqual([["session-1"]]);
  });
});
