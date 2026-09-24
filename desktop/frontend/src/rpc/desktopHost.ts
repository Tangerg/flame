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

const HOST_METHOD = {
  bootstrap: "main.DesktopHost.Bootstrap",
  chooseWorkingDirectory: "main.DesktopHost.ChooseWorkingDirectory",
  revealWindow: "main.DesktopHost.RevealWindow",
  saveImage: "main.DesktopHost.SaveImage",
  windowChrome: "main.DesktopHost.WindowChrome",
} as const;

export interface DesktopHostBinding {
  call(method: string, ...args: unknown[]): Promise<unknown>;
}

export interface DesktopHostClient {
  bootstrap(): Promise<DesktopBootstrap | null>;
  chooseWorkingDirectory(): Promise<string | null>;
  saveImage(source: string): Promise<boolean>;
  windowChrome(): Promise<WindowChrome | null>;
  revealWindow(): Promise<void>;
}

const WindowChromeSchema = z.object({
  controlsCentreY: z.number().nonnegative(),
  controlsInlineEnd: z.number().nonnegative(),
  measured: z.boolean(),
});

const WorkingDirectorySchema = z.string();
const SaveImageSchema = z.boolean();

const DesktopBootstrapSchema = z.object({
  localRuntime: z.object({
    endpoint: z.url(),
    localToken: z.string().min(1).optional(),
  }),
});

async function wailsDesktopHostBinding(): Promise<DesktopHostBinding | undefined> {
  if (!("_wails" in globalThis)) return undefined;
  const { Call } = await import("@wailsio/runtime");
  return { call: (method, ...args) => Call.ByName(method, ...args) };
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
