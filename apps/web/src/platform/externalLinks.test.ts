import { afterEach, beforeEach, describe, expect, it, onTestFinished, vi } from "vitest";
import type { Window as HappyDOMWindow } from "happy-dom";
import { Browser } from "@wailsio/runtime";
import { installExternalLinks } from "@/platform/externalLinks";
import { preventBrowserNavigation } from "@/test/browserTasks";
const openURL = vi.mocked(Browser.OpenURL);

vi.mock("@wailsio/runtime", () => ({
  Browser: { OpenURL: vi.fn() },
}));

const LONDON = "https://www.openstreetmap.org/?mlat=51.5074&mlon=-0.1278";

const CONTROL = "https://example.com/control";

const disposers: (() => void)[] = [];
const network = vi.fn(async () => {
  throw new Error("Unexpected test network access");
});

function install(
  kind: "web" | "desktop" = "desktop",
  reportFailure: (error: unknown, url: string) => void = vi.fn(),
): () => void {
  const dispose = installExternalLinks(document, kind, reportFailure);
  disposers.push(dispose);
  return dispose;
}

function anchor(attributes: Record<string, string>, text = "London"): HTMLAnchorElement {
  const element = document.createElement("a");
  for (const [name, value] of Object.entries(attributes)) element.setAttribute(name, value);
  element.textContent = text;
  document.body.append(element);
  return element;
}

// Observe cancellation after document handlers. happy-dom starts anchor defaults
// during bubbling, so native navigation is disabled separately below.
function activate(
  target: Element,
  { type = "click", ...init }: MouseEventInit & { type?: "click" | "auxclick" } = {},
): boolean {
  let cancelled = false;
  const probe = (event: Event) => {
    cancelled = event.defaultPrevented;
    event.preventDefault();
  };
  window.addEventListener(type, probe);
  target.dispatchEvent(
    new MouseEvent(type, { bubbles: true, cancelable: true, button: 0, ...init }),
  );
  window.removeEventListener(type, probe);
  return cancelled;
}

// A later admitted link proves earlier activations were refused rather than pending.
async function expectOnlyControlOpened(): Promise<void> {
  expect(activate(anchor({ href: CONTROL, target: "_blank" }))).toBe(true);
  await vi.waitFor(() => expect(openURL).toHaveBeenCalledWith(CONTROL));
  expect(openURL.mock.calls).toEqual([[CONTROL]]);
}

function settle(): Promise<void> {
  // Concurrent mocked imports can evaluate the real SDK in Vitest.
  // Complete each simulated gesture before starting another.
  return vi.dynamicImportSettled();
}

beforeEach(() => {
  const dom = (window as unknown as HappyDOMWindow).happyDOM;
  const originalURL = document.URL;
  const restoreNavigation = preventBrowserNavigation();
  dom.setURL("https://flame.test/");
  network.mockClear();
  vi.stubGlobal("fetch", network);
  onTestFinished(() => {
    dom.setURL(originalURL);
    restoreNavigation();
    vi.unstubAllGlobals();
  });
});

afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose();
  document.body.replaceChildren();
  openURL.mockReset();
  vi.restoreAllMocks();
  if (network.mock.calls.length) throw new Error("Unexpected test network access");
});

describe("installExternalLinks", () => {
  it("sends the exact public destination and its query to the OS", async () => {
    install();
    const link = anchor({ href: LONDON, target: "_blank", rel: "noreferrer noopener" });

    expect(activate(link)).toBe(true);
    await vi.waitFor(() => expect(openURL).toHaveBeenCalledExactlyOnceWith(LONDON));
  });

  it("captures the destination before its original anchor changes or retires", async () => {
    install();
    const link = anchor({ href: LONDON });
    expect(activate(link)).toBe(true);
    link.href = CONTROL;
    link.remove();
    await vi.waitFor(() => expect(openURL).toHaveBeenCalledExactlyOnceWith(LONDON));
  });

  it("recognizes activation from nested text and SVG content", async () => {
    install();
    const link = anchor({ href: LONDON });
    link.innerHTML = '<span>London</span><svg><path d="M0 0h1"></path></svg>';

    expect(activate(link.querySelector("span")!)).toBe(true);
    await settle();
    expect(activate(link.querySelector("path")!)).toBe(true);
    await vi.waitFor(() => expect(openURL).toHaveBeenCalledTimes(2));
    expect(openURL.mock.calls).toEqual([[LONDON], [LONDON]]);
  });

  it("opens keyboard, modified and middle activations once each", async () => {
    install();
    const link = anchor({ href: LONDON });

    expect(activate(link, { detail: 0 })).toBe(true);
    await settle();
    expect(activate(link, { metaKey: true })).toBe(true);
    await settle();
    expect(activate(link, { ctrlKey: true })).toBe(true);
    await settle();
    expect(activate(link, { shiftKey: true })).toBe(true);
    await settle();
    expect(activate(link, { type: "auxclick", button: 1 })).toBe(true);
    await vi.waitFor(() => expect(openURL).toHaveBeenCalledTimes(5));
    await settle();
    expect(openURL).toHaveBeenCalledTimes(5);
  });

  it("opens mail links and same-origin links that explicitly target a new window", async () => {
    install();
    const report = new URL("/report?id=1", document.baseURI).href;

    expect(activate(anchor({ href: "mailto:team@example.com?subject=Map" }))).toBe(true);
    await settle();
    expect(activate(anchor({ href: "/report?id=1", target: "_BLANK" }))).toBe(true);
    await vi.waitFor(() => expect(openURL).toHaveBeenCalledTimes(2));
    expect(openURL.mock.calls).toEqual([["mailto:team@example.com?subject=Map"], [report]]);
  });

  it("leaves gestures and destinations it does not own to their native owners", async () => {
    install();
    const external = anchor({ href: LONDON });
    const cancelled = anchor({ href: LONDON });
    cancelled.addEventListener("click", (event) => event.preventDefault());
    const plain = document.createElement("div");
    plain.textContent = "London";
    document.body.append(plain);

    activate(cancelled);
    for (const [target, init] of [
      [external, { type: "auxclick", button: 2 }],
      [external, { altKey: true }],
      [anchor({ href: LONDON, download: "" }), {}],
      [anchor({ href: "/settings" }), {}],
      [anchor({ href: "#section" }), {}],
      [anchor({ href: "wails://wails.localhost/settings", target: "_blank" }), {}],
      [anchor({ href: "file:///etc/hosts", target: "_blank" }), {}],
      [anchor({ href: "tel:+441234567890" }), {}],
      [anchor({ href: "ftp://example.com/file", target: "_blank" }), {}],
      [anchor({ href: "https://[", target: "_blank" }), {}],
      [anchor({ href: "https://user:secret@example.com/", target: "_blank" }), {}],
      [anchor({}), {}],
      [plain, {}],
    ] as const) {
      expect(activate(target, init)).toBe(false);
    }
    await settle();
    await expectOnlyControlOpened();
  });

  it("leaves the browser's native link behavior untouched", async () => {
    const dispose = install("web");

    expect(activate(anchor({ href: LONDON, target: "_blank" }))).toBe(false);
    dispose();
    install();
    await expectOnlyControlOpened();
  });

  it("reports a refused OS request exactly once without retrying", async () => {
    const failure = new Error("open refused");
    openURL.mockRejectedValueOnce(failure);
    const reportFailure = vi.fn();
    install("desktop", reportFailure);

    expect(activate(anchor({ href: LONDON }))).toBe(true);
    await vi.waitFor(() => expect(reportFailure).toHaveBeenCalledExactlyOnceWith(failure, LONDON));
    await settle();
    expect(reportFailure).toHaveBeenCalledOnce();
    expect(openURL).toHaveBeenCalledOnce();
  });

  it("stops recognizing links and reporting pending failures after disposal", async () => {
    const pending = Promise.withResolvers<void>();
    openURL.mockReturnValueOnce(pending.promise);
    const reportFailure = vi.fn();
    const dispose = install("desktop", reportFailure);
    const link = anchor({ href: LONDON });

    expect(activate(link)).toBe(true);
    await vi.waitFor(() => expect(openURL).toHaveBeenCalledOnce());
    dispose();
    pending.reject(new Error("open refused"));
    expect(activate(link)).toBe(false);
    await settle();
    expect(reportFailure).not.toHaveBeenCalled();
    expect(openURL).toHaveBeenCalledOnce();
  });
});
