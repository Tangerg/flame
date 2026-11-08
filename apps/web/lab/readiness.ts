import type { Locator } from "@playwright/test";

interface ReadinessSurface {
  locator(selector: string): Locator;
}

export async function expectLabReady(
  surface: ReadinessSurface,
  options: { readonly timeout?: number } = {},
): Promise<void> {
  await surface.locator('html[data-lab-ready="true"], html[data-lab-error]').waitFor({
    state: "visible",
    ...options,
  });
  const failure = await surface.locator("html").evaluate((root) => ({
    url: root.ownerDocument.URL,
    ready: root.getAttribute("data-lab-ready") === "true",
    reason: root.getAttribute("data-lab-error"),
  }));
  if (failure.reason !== null) {
    throw new Error(`lab document ${failure.url} failed: ${failure.reason}`);
  }
  if (!failure.ready) {
    throw new Error(`lab document ${failure.url} is no longer ready`);
  }
}
