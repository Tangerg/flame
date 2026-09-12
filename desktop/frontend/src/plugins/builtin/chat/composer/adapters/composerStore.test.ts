import { beforeEach, describe, expect, it } from "vitest";
import { useComposerStore } from "./composerStore";

const composer = () => useComposerStore.getState();
const STORAGE_KEY = useComposerStore.persist.getOptions().name!;

beforeEach(() => {
  localStorage.removeItem(STORAGE_KEY);
  composer().clear();
});

describe("the composer's drafts", () => {
  it("survive a restart", async () => {
    composer().loadSession("s1");
    composer().setValue("half a thought");

    const payload = localStorage.getItem(STORAGE_KEY) ?? "null";
    const written = (JSON.parse(payload) as { state: unknown } | null)?.state;
    expect(written, "nothing reached storage").toBeTruthy();

    composer().clear();
    localStorage.setItem(STORAGE_KEY, payload);
    await useComposerStore.persist.rehydrate();

    const partialize = useComposerStore.persist.getOptions().partialize!;
    expect(JSON.parse(JSON.stringify(partialize(composer() as never)))).toEqual(written);
    composer().loadSession("s1");
    expect(composer().composer.draft.value).toBe("half a thought");
  });

  it("do not carry images into storage", () => {
    composer().loadSession("s1");
    composer().setValue("look at this");
    composer().addImages([{ mime: "image/png", data: "AAAAbase64payload" }]);

    expect(localStorage.getItem(STORAGE_KEY) ?? "").not.toContain("base64payload");
  });
});
