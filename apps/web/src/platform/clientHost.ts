import { createBrowserHost } from "@/platform/browserHost";
import { createDesktopHostClient } from "@/platform/desktopHost";
import { type ClientHost } from "@/platform/host";
export function createClientHost(imageFilenameStem: string): ClientHost {
  // Native channels exist before Wails injects its JavaScript runtime object.
  const scope = globalThis as {
    webkit?: { messageHandlers?: { external?: unknown } };
    chrome?: { webview?: unknown };
  };
  if (
    scope.webkit?.messageHandlers?.external === undefined &&
    scope.chrome?.webview === undefined
  ) {
    return createBrowserHost(imageFilenameStem);
  }
  return createDesktopHostClient({
    async call(method, ...args) {
      const { Call } = await import("@wailsio/runtime");
      return Call.ByName(method, ...args);
    },
    async on(event, listener) {
      const { Events } = await import("@wailsio/runtime");
      return Events.On(event, (value) => listener(value.data));
    },
  });
}
