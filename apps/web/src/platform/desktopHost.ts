import { type NotificationAuthorization } from "@/foundation/notificationAuthorization";
import { normalizeRuntimeEndpoint } from "@flame/runtime-client";
import { errorMessage, RpcTransportError } from "@flame/runtime-client";
import { type ClientBootstrap, type ClientHost } from "@/platform/host";
import {
  callDesktop,
  DesktopMethods,
  NotificationOpenedEvent,
  NotificationTargetSchema,
} from "@/platform/desktop.generated";

export interface DesktopHostBinding {
  call(method: string, ...args: unknown[]): Promise<unknown>;
  on(event: string, listener: (data: unknown) => void): Promise<() => void>;
}

async function readAuthorization(
  host: DesktopHostBinding,
  method: "NotificationAuthorization" | "RequestNotificationAuthorization",
): Promise<NotificationAuthorization> {
  let value: unknown;
  try {
    value = await callDesktop(host, method);
  } catch (error) {
    throw new RpcTransportError(`desktop host notification check failed: ${errorMessage(error)}`);
  }
  const parsed = DesktopMethods[method].result.safeParse(value);
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
          value = await callDesktop(host, "Bootstrap");
        } catch (error) {
          throw new RpcTransportError(`desktop host bootstrap failed: ${errorMessage(error)}`);
        }
        const parsed = DesktopMethods.Bootstrap.result.safeParse(value);
        if (!parsed.success) {
          throw new RpcTransportError(
            `desktop host bootstrap returned an invalid shape: ${parsed.error.message}`,
          );
        }
        const endpoint = normalizeRuntimeEndpoint(parsed.data.localRuntime.endpoint);
        if (!endpoint) {
          throw new RpcTransportError(
            "desktop host bootstrap returned an invalid Runtime endpoint",
          );
        }
        return {
          runtime: { ...parsed.data.localRuntime, endpoint },
          localFilesystemEndpoint: endpoint,
        };
      })().finally(() => {
        pending = undefined;
      });
      return pending;
    },
    async chooseWorkingDirectory() {
      const host = binding;
      let value: unknown;
      try {
        value = await callDesktop(host, "ChooseWorkingDirectory");
      } catch (error) {
        throw new RpcTransportError(
          `desktop host directory selection failed: ${errorMessage(error)}`,
        );
      }
      const parsed = DesktopMethods.ChooseWorkingDirectory.result.safeParse(value);
      if (!parsed.success) {
        throw new RpcTransportError(
          `desktop host directory selection returned an invalid shape: ${parsed.error.message}`,
        );
      }
      return parsed.data.length > 0 ? parsed.data : null;
    },
    // Encoding captures the selected byte range before yielding. The native panel
    // owns the destination; neither the plugin nor IPC supplies a path.
    async saveFile({ filename, bytes }) {
      const captured = new Uint8Array(bytes);
      let binary = "";
      for (let offset = 0; offset < captured.byteLength; offset += 0x8000) {
        binary += String.fromCharCode(...captured.subarray(offset, offset + 0x8000));
      }
      const contentBase64 = btoa(binary);
      const host = binding;
      let value: unknown;
      try {
        value = await callDesktop(host, "SaveFile", filename, contentBase64);
      } catch (error) {
        throw new RpcTransportError(`desktop host file save failed: ${errorMessage(error)}`);
      }
      const parsed = DesktopMethods.SaveFile.result.safeParse(value);
      if (!parsed.success) {
        throw new RpcTransportError(
          `desktop host file save returned an invalid shape: ${parsed.error.message}`,
        );
      }
      return parsed.data ? "saved" : "cancelled";
    },
    async revealPath(path) {
      const host = binding;
      try {
        await callDesktop(host, "RevealPath", path);
        return true;
      } catch (error) {
        throw new RpcTransportError(`desktop host path reveal failed: ${errorMessage(error)}`);
      }
    },
    async openPath(path) {
      const host = binding;
      try {
        await callDesktop(host, "OpenPath", path);
        return true;
      } catch (error) {
        throw new RpcTransportError(`desktop host path open failed: ${errorMessage(error)}`);
      }
    },
    async notificationAuthorization() {
      const host = binding;
      return readAuthorization(host, "NotificationAuthorization");
    },
    async requestNotificationAuthorization() {
      const host = binding;
      return readAuthorization(host, "RequestNotificationAuthorization");
    },
    async sendNotification(notification) {
      const host = binding;
      try {
        await callDesktop(host, "SendNotification", notification);
        return true;
      } catch (error) {
        throw new RpcTransportError(`desktop host notification failed: ${errorMessage(error)}`);
      }
    },
    async onNotificationOpened(listener) {
      const host = binding;
      return host.on(NotificationOpenedEvent, (data) => {
        const target = NotificationTargetSchema.safeParse(data);
        if (target.success) listener(target.data);
        else console.error("[desktop] notification opened without a target:", data);
      });
    },
    async revealWindow() {
      const host = binding;
      try {
        await callDesktop(host, "RevealWindow");
      } catch (error) {
        throw new RpcTransportError(`desktop host window reveal failed: ${errorMessage(error)}`);
      }
    },
    async windowChrome() {
      const host = binding;
      try {
        const parsed = DesktopMethods.WindowChrome.result.safeParse(
          await callDesktop(host, "WindowChrome"),
        );
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
