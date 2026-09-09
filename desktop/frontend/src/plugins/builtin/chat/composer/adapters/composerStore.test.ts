import { beforeEach, describe, expect, it } from "vitest";
import { useComposerStore } from "./composerStore";

const composer = () => useComposerStore.getState();
const STORAGE_KEY = useComposerStore.persist.getOptions().name!;

beforeEach(() => {
  localStorage.removeItem(STORAGE_KEY);
  composer().clear();
});

/**
 * The unsent text is the one thing in this store a user would notice losing.
 *
 * It persists and had no test at all — not the round trip, not the pairing behind it.
 * `partialize` decides what reaches storage and `parsePersistedComposer` decides what comes
 * back, two functions in two files, and nothing made them agree. Text typed and not sent is
 * exactly what a desktop app is expected to still have after a restart.
 *
 * Written without naming a persisted field, so it holds when the shape changes and cannot be
 * satisfied by reading back a subset.
 */
describe("the composer's drafts", () => {
  it("survive a restart", async () => {
    composer().loadSession("s1");
    composer().setValue("half a thought");

    const payload = localStorage.getItem(STORAGE_KEY) ?? "null";
    const written = (JSON.parse(payload) as { state: unknown } | null)?.state;
    expect(written, "nothing reached storage").toBeTruthy();

    // Clearing persists the cleared state, so the payload goes back before the rehydrate —
    // otherwise this reads back what the clearing wrote and passes on an empty draft.
    composer().clear();
    localStorage.setItem(STORAGE_KEY, payload);
    await useComposerStore.persist.rehydrate();

    const partialize = useComposerStore.persist.getOptions().partialize!;
    expect(JSON.parse(JSON.stringify(partialize(composer() as never)))).toEqual(written);
    composer().loadSession("s1");
    expect(composer().composer.draft.value).toBe("half a thought");
  });

  // Images are deliberately NOT persisted — a data URL in localStorage is a quota failure
  // waiting for a screenshot. The comment on `partialize` says so; this holds it to it.
  it("do not carry images into storage", () => {
    composer().loadSession("s1");
    composer().setValue("look at this");
    composer().addImages([{ mime: "image/png", data: "AAAAbase64payload" }]);

    expect(localStorage.getItem(STORAGE_KEY) ?? "").not.toContain("base64payload");
  });
});
