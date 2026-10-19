import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { activateComposerStorage, useComposerStore } from "./composerStore";
import * as imageInput from "../public/input";

const composer = () => useComposerStore.getState();
const STORAGE_KEY = useComposerStore.persist.getOptions().name!;

afterEach(() => vi.restoreAllMocks());

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

describe("submission binding", () => {
  it("consumes the accepted snapshot without clearing its replacement", () => {
    composer().loadSession("pending");
    composer().setValue("original");
    const submission = composer().captureSubmission();
    composer().addPaste("a new attachment");
    expect(submission.consume("original")).toBe(false);
    expect(composer().composer.draft.pastes).toHaveLength(1);
    expect(composer().composer.draft.value).toBe("original");
  });
});

describe("captured authoring lifetime", () => {
  it("retires a draft callback after a storage target changes away and back", () => {
    activateComposerStorage("https://draft-a.example");
    composer().loadSession("same-session");
    composer().setValue("original target");
    const submission = composer().captureSubmission();
    activateComposerStorage("https://draft-b.example");
    composer().loadSession("same-session");
    composer().setValue("successor target");
    activateComposerStorage("https://draft-a.example");
    expect(submission.consume("original target")).toBe(false);
    expect(composer().composer.draft.value).toBe("original target");
  });

  it("keeps a successor Session's pending image when an older Session accepts", async () => {
    composer().loadSession("older");
    composer().setValue("old submission");
    const submission = composer().captureSubmission();
    composer().loadSession("newer");
    composer().setValue("keep editing");
    let resolve!: (image: Awaited<ReturnType<typeof imageInput.fileToInputImage>>) => void;
    const reading = new Promise<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>((done) => {
      resolve = done;
    });
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading);
    composer().addImageFiles([new File([""], "pending.png")]);
    expect(submission.consume("old submission")).toBe(false);
    resolve({ mime: "image/png", data: "successor-image", name: "pending.png" });
    await vi.waitFor(() => expect(composer().composer.draft.images).toHaveLength(1));
    expect(composer().composer.draft.value).toBe("keep editing");
    expect(composer().composer.draft.images[0]?.data).toBe("successor-image");
  });
});
