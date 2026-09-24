import { z } from "zod";
import { errorMessage, RpcTransportError } from "./errors";

interface LocalRuntimeConnection {
  endpoint: string;
  localToken?: string;
}

export interface DesktopBootstrap {
  localRuntime: LocalRuntimeConnection;
}

interface WindowChrome {
  controlsCentreY: number;
  controlsInlineEnd: number;
}

type NotificationAuthorization = "unsupported" | "default" | "granted" | "denied";

interface DesktopNotification {
  id: string;
  title: string;
  body?: string;
  target?: string;
}

const NOTIFICATION_OPENED_EVENT = "desktop:notification-opened";

const HOST_METHOD = {
  bootstrap: "main.DesktopHost.Bootstrap",
  notificationAuthorization: "main.DesktopHost.NotificationAuthorization",
  requestNotificationAuthorization: "main.DesktopHost.RequestNotificationAuthorization",
  sendNotification: "main.DesktopHost.SendNotification",
  chooseWorkingDirectory: "main.DesktopHost.ChooseWorkingDirectory",
  openPath: "main.DesktopHost.OpenPath",
  revealPath: "main.DesktopHost.RevealPath",
  revealWindow: "main.DesktopHost.RevealWindow",
  saveImage: "main.DesktopHost.SaveImage",
  windowChrome: "main.DesktopHost.WindowChrome",
} as const;

export interface DesktopHostBinding {
  call(method: string, ...args: unknown[]): Promise<unknown>;
  on(event: string, listener: (data: unknown) => void): () => void;
}

export interface DesktopHostClient {
  bootstrap(): Promise<DesktopBootstrap | null>;
  chooseWorkingDirectory(): Promise<string | null>;
  saveImage(source: string): Promise<boolean>;
  windowChrome(): Promise<WindowChrome | null>;
  revealWindow(): Promise<void>;
  revealPath(path: string): Promise<boolean>;
  openPath(path: string): Promise<boolean>;
  notificationAuthorization(): Promise<NotificationAuthorization | null>;
  requestNotificationAuthorization(): Promise<NotificationAuthorization | null>;
  sendNotification(notification: DesktopNotification): Promise<boolean | null>;
  onNotificationOpened(listener: (target: string) => void): Promise<() => void>;
}

const WindowChromeSchema = z.object({
  controlsCentreY: z.number().nonnegative(),
  controlsInlineEnd: z.number().nonnegative(),
  measured: z.boolean(),
});

const WorkingDirectorySchema = z.string();
const NotificationAuthorizationSchema = z.enum(["unsupported", "default", "granted", "denied"]);
const NotificationTargetSchema = z.string().min(1);
const SaveImageSchema = z.boolean();

const DesktopBootstrapSchema = z.object({
  localRuntime: z.object({
    endpoint: z.url(),
    localToken: z.string().min(1).optional(),
  }),
});

// Wails injects `window._wails` only once navigation has finished, which can be
// after this module runs, so its absence says nothing. The native message
// channel Wails calls through exists from the first script on every platform:
// WKWebView and WebKitGTK expose `webkit.messageHandlers.external`, WebView2
// exposes `chrome.webview`. A browser has neither.
function insideWailsWebview(): boolean {
  const scope = globalThis as {
    webkit?: { messageHandlers?: { external?: unknown } };
    chrome?: { webview?: unknown };
  };
  return (
    scope.webkit?.messageHandlers?.external !== undefined || scope.chrome?.webview !== undefined
  );
}

async function wailsDesktopHostBinding(): Promise<DesktopHostBinding | undefined> {
  if (!insideWailsWebview()) return undefined;
  const { Call, Events } = await import("@wailsio/runtime");
  return {
    call: (method, ...args) => Call.ByName(method, ...args),
    on: (event, listener) => Events.On(event, (ev) => listener(ev.data)),
  };
}

async function readAuthorization(
  host: DesktopHostBinding,
  method: string,
): Promise<NotificationAuthorization> {
  let value: unknown;
  try {
    value = await host.call(method);
  } catch (error) {
    throw new RpcTransportError(`desktop host notification check failed: ${errorMessage(error)}`);
  }
  const parsed = NotificationAuthorizationSchema.safeParse(value);
  if (!parsed.success) {
    throw new RpcTransportError(
      `desktop host notification check returned an invalid shape: ${parsed.error.message}`,
    );
  }
  return parsed.data;
}

export function createDesktopHostClient(binding?: DesktopHostBinding): DesktopHostClient {
  let pending: Promise<DesktopBootstrap | null> | undefined;
  return {
    bootstrap() {
      pending ??= (async () => {
        const host = binding ?? (await wailsDesktopHostBinding());
        if (!host) return null;
        let value: unknown;
        try {
          value = await host.call(HOST_METHOD.bootstrap);
        } catch (error) {
          throw new RpcTransportError(`desktop host bootstrap failed: ${errorMessage(error)}`);
        }
        const parsed = DesktopBootstrapSchema.safeParse(value);
        if (!parsed.success) {
          throw new RpcTransportError(
            `desktop host bootstrap returned an invalid shape: ${parsed.error.message}`,
          );
        }
        return parsed.data;
      })();
      return pending;
    },
    async chooseWorkingDirectory() {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return null;
      let value: unknown;
      try {
        value = await host.call(HOST_METHOD.chooseWorkingDirectory);
      } catch (error) {
        throw new RpcTransportError(
          `desktop host directory selection failed: ${errorMessage(error)}`,
        );
      }
      const parsed = WorkingDirectorySchema.safeParse(value);
      if (!parsed.success) {
        throw new RpcTransportError(
          `desktop host directory selection returned an invalid shape: ${parsed.error.message}`,
        );
      }
      return parsed.data.length > 0 ? parsed.data : null;
    },
    async saveImage(source) {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return false;
      let value: unknown;
      try {
        value = await host.call(HOST_METHOD.saveImage, source);
      } catch (error) {
        throw new RpcTransportError(`desktop host image save failed: ${errorMessage(error)}`);
      }
      const parsed = SaveImageSchema.safeParse(value);
      if (!parsed.success) {
        throw new RpcTransportError(
          `desktop host image save returned an invalid shape: ${parsed.error.message}`,
        );
      }
      return parsed.data;
    },
    async revealPath(path) {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return false;
      try {
        await host.call(HOST_METHOD.revealPath, path);
        return true;
      } catch (error) {
        throw new RpcTransportError(`desktop host path reveal failed: ${errorMessage(error)}`);
      }
    },
    async openPath(path) {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return false;
      try {
        await host.call(HOST_METHOD.openPath, path);
        return true;
      } catch (error) {
        throw new RpcTransportError(`desktop host path open failed: ${errorMessage(error)}`);
      }
    },
    async notificationAuthorization() {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return null;
      return readAuthorization(host, HOST_METHOD.notificationAuthorization);
    },
    async requestNotificationAuthorization() {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return null;
      return readAuthorization(host, HOST_METHOD.requestNotificationAuthorization);
    },
    async sendNotification(notification) {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return null;
      try {
        await host.call(HOST_METHOD.sendNotification, notification);
        return true;
      } catch (error) {
        throw new RpcTransportError(`desktop host notification failed: ${errorMessage(error)}`);
      }
    },
    async onNotificationOpened(listener) {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return () => {};
      return host.on(NOTIFICATION_OPENED_EVENT, (data) => {
        const target = NotificationTargetSchema.safeParse(data);
        if (target.success) listener(target.data);
        else console.error("[desktop] notification opened without a target:", data);
      });
    },
    async revealWindow() {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) {
        window.focus();
        return;
      }
      try {
        await host.call(HOST_METHOD.revealWindow);
      } catch (error) {
        throw new RpcTransportError(`desktop host window reveal failed: ${errorMessage(error)}`);
      }
    },
    async windowChrome() {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return null;
      try {
        const parsed = WindowChromeSchema.safeParse(await host.call(HOST_METHOD.windowChrome));
        if (!parsed.success || !parsed.data.measured) return null;
        return {
          controlsCentreY: parsed.data.controlsCentreY,
          controlsInlineEnd: parsed.data.controlsInlineEnd,
        };
      } catch {
        return null;
      }
    },
  };
}
