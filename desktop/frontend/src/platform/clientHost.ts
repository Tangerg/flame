import { createBrowserHost } from "./browserHost";
import { createDesktopHostClient } from "./desktopHost";
import type { ClientHost } from "./host";

export function createClientHost(): ClientHost {
  // Native channels exist before Wails injects its JavaScript runtime object.
  const scope = globalThis as {
    webkit?: { messageHandlers?: { external?: unknown } };
    chrome?: { webview?: unknown };
  };
  if (
    scope.webkit?.messageHandlers?.external === undefined &&
    scope.chrome?.webview === undefined
  ) {
    return createBrowserHost();
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
