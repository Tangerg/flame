import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as imageInput from "@/features/composer/public/input";
const composer = () => testClient.composer.store.getState();

let STORAGE_KEY: string;

afterEach(() => vi.restoreAllMocks());

beforeEach(() => {
  STORAGE_KEY = testClient.composer.store.persist.getOptions().name!;
  localStorage.removeItem(STORAGE_KEY);
  composer().clear();
});

describe("the composer's drafts", () => {
  it("keeps partial failures attached until the user removes them", async () => {
    const first = Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    const second = Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage")
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    composer().setValue("keep both selections");
    composer().addImageFiles([new File([""], "first.png"), new File([""], "second.png")]);
    expect(composer().composer.draft.imageAttachments.map((image) => image.status)).toEqual([
      "pending",
      "pending",
    ]);
    first.resolve({ mime: "image/png", data: "first-image", name: "first.png" });
    second.reject(new Error("file read failed"));
    await vi.waitFor(() =>
      expect(composer().composer.draft.imageAttachments.map((image) => image.status)).toEqual([
        "ready",
        "failed",
      ]),
    );
    expect(composer().captureSubmission().attachmentsReady).toBe(false);
    expect(composer().captureSubmission().consume()).toBe(false);
    const failed = composer().composer.draft.imageAttachments[1]!;
    expect(failed.name).toBe("second.png");
    composer().removeImage(failed.id);
    expect(composer().captureSubmission().attachmentsReady).toBe(true);
    expect(composer().composer.draft.images).toMatchObject([{ data: "first-image" }]);
    expect(composer().composer.draft.value).toBe("keep both selections");
  });

  it("does not restore a pending image after the user removes it", async () => {
    const reading =
      Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    composer().addImageFiles([new File([""], "cancelled.png")]);
    composer().removeImage(composer().composer.draft.imageAttachments[0]!.id);
    reading.resolve({ mime: "image/png", data: "cancelled", name: "cancelled.png" });
    await reading.promise;
    await Promise.resolve();
    expect(composer().composer.draft.imageAttachments).toEqual([]);
  });

  it.each(["clear", "replace", "discard"])("retires pending image input on %s", async (action) => {
    const reading =
      Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    composer().loadSession("retired");
    composer().addImageFiles([new File([""], "retired.png")]);
    if (action === "clear") composer().clear();
    else if (action === "replace") composer().replaceDraft({ text: "replacement" });
    else composer().discardSession("retired");
    reading.resolve({ mime: "image/png", data: "retired", name: "retired.png" });
    await reading.promise;
    await Promise.resolve();
    expect(composer().composer.draft.imageAttachments).toEqual([]);
    expect(composer().composer.draft.value).toBe(action === "replace" ? "replacement" : "");
  });

  it("finishes image reading in its original Session while another draft is active", async () => {
    const reading =
      Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    composer().loadSession("original");
    composer().setValue("original text");
    composer().addImageFiles([new File([""], "original.png")]);
    composer().loadSession("other");
    composer().setValue("other text");
    reading.resolve({ mime: "image/png", data: "original-image", name: "original.png" });
    await reading.promise;
    await Promise.resolve();
    await Promise.resolve();

    expect(composer().composer.draft.value).toBe("other text");
    expect(composer().composer.draft.images).toEqual([]);
    composer().loadSession("original");
    expect(composer().composer.draft.value).toBe("original text");
    expect(composer().composer.draft.images).toMatchObject([{ data: "original-image" }]);
  });

  it("survive a restart", async () => {
    composer().loadSession("s1");
    composer().setValue("half a thought");

    const payload = localStorage.getItem(STORAGE_KEY) ?? "null";
    const written = (JSON.parse(payload) as { state: unknown } | null)?.state;
    expect(written, "nothing reached storage").toBeTruthy();

    composer().clear();
    localStorage.setItem(STORAGE_KEY, payload);
    await testClient.composer.store.persist.rehydrate();

    const partialize = testClient.composer.store.persist.getOptions().partialize!;
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

describe("pasted text", () => {
  it("reports whether an edited paste is still attached", () => {
    composer().addPaste("first");
    composer().addPaste("second");
    const [first, second] = composer().composer.draft.pastes;

    expect(composer().editPaste(first!.id, "revised")).toBe(true);
    expect(composer().editPaste(second!.id, " \n ")).toBe(false);
    expect(composer().composer.draft.pastes).toMatchObject([{ id: first!.id, text: "revised" }]);
    expect(composer().editPaste("missing", "anything")).toBe(false);
    expect(composer().composer.draft.pastes).toMatchObject([{ id: first!.id, text: "revised" }]);
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
    testClient.composer.activate("https://draft-a.example");
    composer().loadSession("same-session");
    composer().setValue("original target");
    const submission = composer().captureSubmission();
    testClient.composer.activate("https://draft-b.example");
    composer().loadSession("same-session");
    composer().setValue("successor target");
    testClient.composer.activate("https://draft-a.example");
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

describe("captured draft replacement", () => {
  beforeEach(() => {
    composer().loadSession("replace-draft");
    composer().replaceDraft({ text: "original draft" });
  });

  it("replaces the unchanged draft and its attachments in one publication", () => {
    composer().addPaste("old paste");
    composer().addImages([{ mime: "image/png", data: "old-image" }]);
    const replace = composer().captureDraftReplacement();
    const observed = vi.fn();
    const unsubscribe = testClient.composer.store.subscribe((state) =>
      observed(state.composer.draft),
    );

    try {
      expect(
        replace({ text: "canonical input", images: [{ mime: "image/jpeg", data: "new-image" }] }),
      ).toBe(true);
      expect(observed).toHaveBeenCalledOnce();
      expect(composer().composer.draft).toMatchObject({
        value: "canonical input",
        images: [{ mime: "image/jpeg", data: "new-image" }],
        pastes: [],
      });
    } finally {
      unsubscribe();
    }
  });

  it("retains a later text revision even if it returns to the captured text", () => {
    const replace = composer().captureDraftReplacement();
    composer().setValue("later draft");
    composer().setValue("original draft");
    const later = composer().composer.draft;

    expect(replace({ text: "history input" })).toBe(false);
    expect(composer().composer.draft).toBe(later);
  });

  it("retires an empty draft replacement after later authoring is cleared", () => {
    composer().loadSession("empty-replacement-clear");
    const replace = composer().captureDraftReplacement();
    composer().setValue("authored later");
    composer().clear();

    expect(replace({ text: "history input" })).toBe(false);
    expect(composer().composer.draft.value).toBe("");
  });

  it("retires an empty draft replacement after later authoring is consumed", () => {
    composer().loadSession("empty-replacement-consume");
    const replace = composer().captureDraftReplacement();
    composer().setValue("submitted later");
    const submission = composer().captureSubmission();

    expect(submission.consume("submitted later")).toBe(true);
    expect(replace({ text: "history input" })).toBe(false);
    expect(composer().composer.draft.value).toBe("");
  });

  it.each(["paste", "image"])("retains a later %s attachment", (kind) => {
    const replace = composer().captureDraftReplacement();
    if (kind === "paste") composer().addPaste("later paste");
    else composer().addImages([{ mime: "image/png", data: "later-image" }]);
    const later = composer().composer.draft;

    expect(replace({ text: "history input" })).toBe(false);
    expect(composer().composer.draft).toBe(later);
  });

  it("leaves both Session drafts unchanged after switching away", () => {
    const original = composer().composer.draft;
    const replace = composer().captureDraftReplacement();
    composer().loadSession("another-session");
    composer().replaceDraft({ text: "another draft" });
    const current = composer().composer.draft;

    expect(replace({ text: "history input" })).toBe(false);
    expect(composer().composer.draft).toBe(current);
    composer().loadSession("replace-draft");
    expect(composer().composer.draft).toBe(original);
  });

  it("retires replacement after the storage target changes away and back", () => {
    testClient.composer.activate("https://replace-draft-a.test");
    composer().loadSession("same-session");
    composer().setValue("original target");
    const replace = composer().captureDraftReplacement();
    testClient.composer.activate("https://replace-draft-b.test");
    composer().loadSession("same-session");
    composer().setValue("successor target");
    testClient.composer.activate("https://replace-draft-a.test");
    composer().loadSession("same-session");

    expect(replace({ text: "history input" })).toBe(false);
    expect(composer().composer.draft.value).toBe("original target");
  });

  it("retains an image selected after capture while its bytes are still pending", async () => {
    const reading =
      Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    const replace = composer().captureDraftReplacement();
    composer().addImageFiles([new File([""], "later.png")]);

    expect(replace({ text: "history input" })).toBe(false);
    expect(composer().composer.draft.value).toBe("original draft");
    reading.resolve({ mime: "image/png", data: "later-image", name: "later.png" });
    await vi.waitFor(() => expect(composer().composer.draft.images).toHaveLength(1));
    expect(composer().composer.draft.images[0]?.data).toBe("later-image");
  });

  it("detects a newer intake while an earlier image is already pending", async () => {
    const first = Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    const second = Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage")
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    composer().addImageFiles([new File([""], "first.png")]);
    const replace = composer().captureDraftReplacement();
    composer().addImageFiles([new File([""], "second.png")]);

    expect(replace({ text: "history input" })).toBe(false);
    second.resolve({ mime: "image/png", data: "second-image", name: "second.png" });
    first.resolve({ mime: "image/png", data: "first-image", name: "first.png" });
    await vi.waitFor(() => expect(composer().composer.draft.images).toHaveLength(2));
    expect(composer().composer.draft.value).toBe("original draft");
    expect(composer().composer.draft.images.map((image) => image.data)).toEqual([
      "first-image",
      "second-image",
    ]);
  });

  it("keeps selection order when ordinary image imports settle in reverse order", async () => {
    const first = Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    const second = Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage")
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    composer().addImageFiles([new File([""], "first.png")]);
    composer().addImageFiles([new File([""], "second.png")]);

    second.resolve({ mime: "image/png", data: "second-image", name: "second.png" });
    await vi.waitFor(() => expect(composer().composer.draft.images).toHaveLength(1));
    first.resolve({ mime: "image/png", data: "first-image", name: "first.png" });
    await vi.waitFor(() => expect(composer().composer.draft.images).toHaveLength(2));
    expect(composer().composer.draft.images.map((image) => image.data)).toEqual([
      "first-image",
      "second-image",
    ]);
  });
});
