const EXTERNAL_PROTOCOLS = new Set(["http:", "https:", "mailto:"]);

// The Wails macOS webview has no new-window handler, so external web and mail
// links do nothing unless the OS receives them. Browser keeps its native behavior.
export function installExternalLinks(
  document: Document,
  kind: "web" | "desktop",
  reportFailure: (error: unknown, url: string) => void,
): () => void {
  if (kind === "web") return () => {};
  let installed = true;
  const activate = (event: MouseEvent) => {
    const url = externalDestination(document, event);
    if (url === null) return;
    event.preventDefault();
    void openExternal(url).catch((error: unknown) => {
      if (installed) reportFailure(error, url);
    });
  };
  document.addEventListener("click", activate);
  document.addEventListener("auxclick", activate);
  return () => {
    installed = false;
    document.removeEventListener("click", activate);
    document.removeEventListener("auxclick", activate);
  };
}

async function openExternal(url: string): Promise<void> {
  const { Browser } = await import("@wailsio/runtime");
  await Browser.OpenURL(url);
}

function externalDestination(document: Document, event: MouseEvent): string | null {
  if (event.defaultPrevented || event.altKey) return null;
  if (event.button !== (event.type === "auxclick" ? 1 : 0)) return null;
  if (!(event.target instanceof Element)) return null;
  const anchor = event.target.closest("a[href]");
  if (!anchor || anchor.hasAttribute("download")) return null;
  let url: URL;
  try {
    url = new URL(anchor.getAttribute("href")!, document.baseURI);
  } catch (error) {
    if (error instanceof TypeError) return null;
    throw error;
  }
  if (!EXTERNAL_PROTOCOLS.has(url.protocol) || url.username || url.password) return null;
  if (
    url.protocol !== "mailto:" &&
    url.origin === new URL(document.URL).origin &&
    anchor.getAttribute("target")?.toLowerCase() !== "_blank"
  ) {
    return null;
  }
  return url.href;
}
