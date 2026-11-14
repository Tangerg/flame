import { afterEach, describe, expect, it, vi } from "vitest";
import { bindClientPageLifecycle, bindDraftExitWarning } from "@/app/pageLifecycle";
import { ClientRenderer } from "@/app/renderer";
import { createComposerStore } from "@/features/composer/adapters/composerStore";
import { refuseDraftStorage } from "@/test/draftStorage";

const disposals: Array<() => Promise<void>> = [];
afterEach(async () => {
  for (const dispose of disposals.splice(0)) await dispose();
});

async function mountedPage() {
  const input = document.createElement("input");
  input.value = "  preserved draft  ";
  const unmount = vi.fn(() => input.remove());
  const mount = vi.fn(() => {
    document.body.append(input);
    return { unmount };
  });
  const closed = Promise.withResolvers<void>();
  const closeConnection = vi.fn(() => closed.promise);
  const stopWatching = vi.fn();
  const renderer = new ClientRenderer({
    initializeClientHost: async () => undefined,
    prepareWindowChrome: async () => undefined,
    watchWindowChrome: () => stopWatching,
    mount,
    mountStartupFailure: () => {
      throw new Error("unexpected startup failure");
    },
    closeConnection,
    reportFailure: vi.fn(),
  });
  const unbind = bindClientPageLifecycle(window, () => {
    void renderer.dispose();
  });
  disposals.push(async () => {
    unbind();
    closed.resolve();
    await renderer.dispose();
  });
  await renderer.start();
  input.focus();
  input.setSelectionRange(7, 7);
  return { input, mount, unmount, closeConnection, stopWatching, renderer, unbind, closed };
}

function transition(type: "pagehide" | "pageshow", persisted: boolean) {
  const event = new Event(type);
  Object.defineProperty(event, "persisted", { value: persisted });
  window.dispatchEvent(event);
}

describe("client page lifecycle", () => {
  it("warns only while the draft owner has an unconfirmed write and preserves a cancelled page", async () => {
    const page = await mountedPage();
    const drafts = createComposerStore();
    drafts.activate("https://draft-exit.test");
    const unbind = bindDraftExitWarning(window, drafts);
    const leaving = () => window.dispatchEvent(new Event("beforeunload", { cancelable: true }));
    await vi.waitFor(() => expect(drafts.store.getState().persistenceState.status).toBe("saved"));
    const refusal = refuseDraftStorage(
      "save",
      new DOMException("storage quota exceeded", "QuotaExceededError"),
    );
    try {
      expect(leaving()).toBe(true);
      drafts.store.getState().setValue("unconfirmed draft");
      expect(leaving()).toBe(false);
      expect(page.unmount).not.toHaveBeenCalled();
      expect(page.closeConnection).not.toHaveBeenCalled();
      expect(page.input.isConnected).toBe(true);
      await vi.waitFor(() =>
        expect(drafts.store.getState().persistenceState.status).toBe("failed"),
      );
      refusal.restore();
      drafts.retryPersistence();
      await vi.waitFor(() => expect(drafts.store.getState().persistenceState.status).toBe("saved"));
      expect(leaving()).toBe(true);
      expect(page.mount).toHaveBeenCalledOnce();
    } finally {
      refusal.restore();
      unbind();
      drafts.dispose();
    }
  });

  it("retains the exit warning for an inactive target and removes it on unbinding", async () => {
    const drafts = createComposerStore();
    drafts.activate("https://draft-exit-original.test");
    const unbind = bindDraftExitWarning(window, drafts);
    const leaving = () => window.dispatchEvent(new Event("beforeunload", { cancelable: true }));
    await vi.waitFor(() => expect(drafts.store.getState().persistenceState.status).toBe("saved"));
    const refusal = refuseDraftStorage(
      "save",
      new DOMException("storage quota exceeded", "QuotaExceededError"),
    );
    try {
      drafts.store.getState().setValue("original target draft");
      await vi.waitFor(() =>
        expect(drafts.store.getState().persistenceState.status).toBe("failed"),
      );
      refusal.restore();
      drafts.activate("https://draft-exit-successor.test");
      drafts.store.getState().setValue("saved successor draft");
      await vi.waitFor(() => expect(drafts.store.getState().persistenceState.status).toBe("saved"));
      expect(leaving()).toBe(false);
      unbind();
      expect(leaving()).toBe(true);
    } finally {
      refusal.restore();
      unbind();
      drafts.dispose();
    }
  });

  it("keeps the mounted owner and editing position when a later guard cancels navigation", async () => {
    const page = await mountedPage();
    const guard = (event: Event) => event.preventDefault();
    window.addEventListener("beforeunload", guard);
    try {
      const leaving = new Event("beforeunload", { cancelable: true });
      expect(window.dispatchEvent(leaving)).toBe(false);
      expect(leaving.defaultPrevented).toBe(true);
      expect(page.unmount).not.toHaveBeenCalled();
      expect(page.closeConnection).not.toHaveBeenCalled();
      expect(page.mount).toHaveBeenCalledOnce();
      expect(page.input.isConnected).toBe(true);
      expect(document.activeElement).toBe(page.input);
      expect(page.input.selectionStart).toBe(7);
      page.input.setRangeText("continued ", 7, 7, "end");
      expect(page.input.value).toBe("  presecontinued rved draft  ");
    } finally {
      window.removeEventListener("beforeunload", guard);
    }
  });

  it("does not retire a hidden document that becomes visible again", async () => {
    const page = await mountedPage();
    const visibility = Object.getOwnPropertyDescriptor(document, "visibilityState");
    try {
      for (const value of ["hidden", "visible"]) {
        Object.defineProperty(document, "visibilityState", { configurable: true, value });
        document.dispatchEvent(new Event("visibilitychange"));
      }
      expect(page.unmount).not.toHaveBeenCalled();
      expect(page.closeConnection).not.toHaveBeenCalled();
      expect(page.stopWatching).not.toHaveBeenCalled();
      expect(page.input.isConnected).toBe(true);
      expect(page.input.value).toBe("  preserved draft  ");
    } finally {
      if (visibility) Object.defineProperty(document, "visibilityState", visibility);
      else Reflect.deleteProperty(document, "visibilityState");
    }
  });

  it("preserves the same owner through a persisted pagehide and pageshow", async () => {
    const page = await mountedPage();
    transition("pagehide", true);
    transition("pageshow", true);
    expect(page.mount).toHaveBeenCalledOnce();
    expect(page.unmount).not.toHaveBeenCalled();
    expect(page.closeConnection).not.toHaveBeenCalled();
    expect(page.stopWatching).not.toHaveBeenCalled();
    expect(page.input.isConnected).toBe(true);
    expect(document.activeElement).toBe(page.input);
    expect(page.input.value).toBe("  preserved draft  ");
    expect(page.input.selectionStart).toBe(7);
  });

  it("retires once when a nonpersisted departure competes with explicit teardown", async () => {
    const page = await mountedPage();
    transition("pagehide", false);
    const closing = page.renderer.dispose();
    transition("pagehide", false);
    expect(page.unmount).toHaveBeenCalledOnce();
    expect(page.stopWatching).toHaveBeenCalledOnce();
    expect(page.closeConnection).toHaveBeenCalledOnce();
    expect(page.renderer.dispose()).toBe(closing);
    expect(page.input.isConnected).toBe(false);
    page.closed.resolve();
    await closing;
  });

  it("unbinds page events without retiring the owner before explicit HMR teardown", async () => {
    const page = await mountedPage();
    page.unbind();
    transition("pagehide", false);
    expect(page.unmount).not.toHaveBeenCalled();
    expect(page.closeConnection).not.toHaveBeenCalled();
    const closing = page.renderer.dispose();
    expect(page.unmount).toHaveBeenCalledOnce();
    expect(page.stopWatching).toHaveBeenCalledOnce();
    expect(page.closeConnection).toHaveBeenCalledOnce();
    page.closed.resolve();
    await closing;
  });
});
