import { type NotificationAuthorization } from "@flame/component-sdk/notifications";
import { normalizeRuntimeEndpoint } from "@flame/runtime-client";
import { errorMessage, RpcTransportError } from "@flame/runtime-client";
import { type ClientBootstrap, type ClientHost } from "@/platform/host";
import {
  callDesktop,
  DesktopMethods,
  MAXIMUM_EXPORT_CHUNK_BYTES,
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

function exportChunk(bytes: Uint8Array): string {
  let binary = "";
  const batchSize = 0x8000;
  for (let offset = 0; offset < bytes.byteLength; offset += batchSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + batchSize));
  }
  return btoa(binary);
}

async function saveExport(
  binding: DesktopHostBinding,
  filename: string,
  bytes: Uint8Array,
): Promise<"saved" | "cancelled"> {
  if (bytes.byteLength <= MAXIMUM_EXPORT_CHUNK_BYTES) {
    const result = await callDesktop(binding, "SaveFile", filename, exportChunk(bytes));
    return DesktopMethods.SaveFile.result.parse(result) ? "saved" : "cancelled";
  }
  // The picker yields. A plugin may reuse its buffer as soon as saveFile returns the promise.
  const snapshot = new Uint8Array(bytes);
  const transfer = DesktopMethods.BeginFileSave.result.parse(
    await callDesktop(binding, "BeginFileSave", filename, snapshot.byteLength),
  );
  if (!transfer) return "cancelled";
  try {
    for (let offset = 0; offset < snapshot.byteLength; offset += MAXIMUM_EXPORT_CHUNK_BYTES) {
      const encoded = exportChunk(snapshot.subarray(offset, offset + MAXIMUM_EXPORT_CHUNK_BYTES));
      await callDesktop(binding, "AppendFileSave", transfer, offset, encoded);
    }
    await callDesktop(binding, "FinishFileSave", transfer);
    return "saved";
  } catch (error) {
    try {
      await callDesktop(binding, "CancelFileSave", transfer);
    } catch (cleanup) {
      throw new AggregateError(
        [error, cleanup],
        `file export failed: ${errorMessage(error)}; cleanup failed: ${errorMessage(cleanup)}`,
      );
    }
    throw error;
  }
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
      try {
        return await saveExport(binding, filename, bytes);
      } catch (error) {
        throw new RpcTransportError(`desktop host file save failed: ${errorMessage(error)}`);
      }
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
