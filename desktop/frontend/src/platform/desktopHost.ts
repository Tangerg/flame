import { z } from "zod";
import type { NotificationAuthorization } from "@/foundation/notificationAuthorization";
import { normalizeRuntimeEndpoint } from "@flame/runtime-contract/client/endpoint";
import { errorMessage, RpcTransportError } from "@flame/runtime-contract/client/errors";

import type { ClientBootstrap, ClientHost } from "./host";

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
  on(event: string, listener: (data: unknown) => void): Promise<() => void>;
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
    endpoint: z.url().transform((value, ctx) => {
      const endpoint = normalizeRuntimeEndpoint(value);
      if (endpoint) return endpoint;
      ctx.addIssue({ code: "custom", message: "invalid Runtime endpoint" });
      return z.NEVER;
    }),
    localToken: z.string().min(1).optional(),
  }),
});

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

export function createDesktopHostClient(binding: DesktopHostBinding): ClientHost {
  let pending: Promise<ClientBootstrap> | undefined;
  return {
    kind: "desktop",
    bootstrap() {
      pending ??= (async () => {
        const host = binding;
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
        return {
          runtime: parsed.data.localRuntime,
          localFilesystemEndpoint: parsed.data.localRuntime.endpoint,
        };
      })();
      return pending;
    },
    async chooseWorkingDirectory() {
      const host = binding;
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
      const host = binding;
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
      const host = binding;
      try {
        await host.call(HOST_METHOD.revealPath, path);
        return true;
      } catch (error) {
        throw new RpcTransportError(`desktop host path reveal failed: ${errorMessage(error)}`);
      }
    },
    async openPath(path) {
      const host = binding;
      try {
        await host.call(HOST_METHOD.openPath, path);
        return true;
      } catch (error) {
        throw new RpcTransportError(`desktop host path open failed: ${errorMessage(error)}`);
      }
    },
    async notificationAuthorization() {
      const host = binding;
      return readAuthorization(host, HOST_METHOD.notificationAuthorization);
    },
    async requestNotificationAuthorization() {
      const host = binding;
      return readAuthorization(host, HOST_METHOD.requestNotificationAuthorization);
    },
    async sendNotification(notification) {
      const host = binding;
      try {
        await host.call(HOST_METHOD.sendNotification, notification);
        return true;
      } catch (error) {
        throw new RpcTransportError(`desktop host notification failed: ${errorMessage(error)}`);
      }
    },
    async onNotificationOpened(listener) {
      const host = binding;
      return host.on(NOTIFICATION_OPENED_EVENT, (data) => {
        const target = NotificationTargetSchema.safeParse(data);
        if (target.success) listener(target.data);
        else console.error("[desktop] notification opened without a target:", data);
      });
    },
    async revealWindow() {
      const host = binding;
      try {
        await host.call(HOST_METHOD.revealWindow);
      } catch (error) {
        throw new RpcTransportError(`desktop host window reveal failed: ${errorMessage(error)}`);
      }
    },
    async windowChrome() {
      const host = binding;
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
