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
import { createComposerStore } from "@/features/composer/adapters/composerStore";
import { submitComposer } from "@/features/composer/application/submitComposer";
import { mockLocalStorage } from "@/test/localStorage";
const composer = () => testClient.composer.store.getState();

let STORAGE_KEY: string;

afterEach(() => vi.restoreAllMocks());

beforeEach(() => {
  STORAGE_KEY = `flame.composer:${encodeURIComponent(testClient.runtime.endpoint.currentRuntimeEndpoint())}`;
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

  it("survive a restart", () => {
    composer().loadSession("s1");
    composer().setValue("half a thought");

    const payload = localStorage.getItem(STORAGE_KEY) ?? "null";
    const written = (JSON.parse(payload) as { state: unknown } | null)?.state;
    expect(written, "nothing reached storage").toBeTruthy();

    const restored = createComposerStore();
    finishTestClient(() => restored.dispose());
    restored.activate(testClient.runtime.endpoint.currentRuntimeEndpoint());
    restored.store.getState().loadSession("s1");
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY)!)).toEqual({ state: written, version: 0 });
    expect(restored.store.getState().composer.draft.value).toBe("half a thought");
  });

  it("do not carry images into storage", () => {
    composer().loadSession("s1");
    composer().setValue("look at this");
    composer().addImages([{ mime: "image/png", data: "AAAAbase64payload" }]);

    expect(localStorage.getItem(STORAGE_KEY) ?? "").not.toContain("base64payload");
  });
});

function coldComposer() {
  const owner = createComposerStore();
  finishTestClient(() => owner.dispose());
  return owner;
}

function refuseWrites(cause: unknown, target = STORAGE_KEY) {
  const replacement = mockLocalStorage();
  replacement.storage.setItem.mockImplementation((key, value) => {
    if (key === target) throw cause;
    replacement.backing.setItem(key, value);
  });
  return replacement;
}

function refuseReads(cause: unknown) {
  const replacement = mockLocalStorage();
  replacement.storage.getItem.mockImplementation((key) => {
    if (key === STORAGE_KEY) throw cause;
    return replacement.backing.getItem(key);
  });
  return replacement;
}

describe("draft saving failures", () => {
  it("keeps confirmation through non-text changes without retrying a refused write", () => {
    composer().loadSession("confirmed");
    composer().setValue("confirmed text");
    const cause = new DOMException("write refused", "QuotaExceededError");
    const write = refuseWrites(cause);
    composer().setModel({ kind: "explicit", provider: "recorded", model: "selected" });
    composer().setValue("confirmed text");
    composer().addImages([{ mime: "image/png", data: "memory-only" }]);
    composer().removeImage(composer().composer.draft.imageAttachments[0]!.id);
    composer().loadSession("other");
    composer().loadSession("confirmed");
    expect(write.storage.setItem).not.toHaveBeenCalled();
    expect(composer().persistenceState).toEqual({ status: "saved" });
    expect(testClient.composer.hasUnconfirmedDrafts()).toBe(false);

    composer().setValue("new text");
    expect(write.storage.setItem).toHaveBeenCalledOnce();
    composer().setValue("new text");
    composer().setModel({ kind: "session" });
    composer().loadSession("other");
    composer().loadSession("confirmed");
    expect(write.storage.setItem).toHaveBeenCalledOnce();
    expect(composer().persistenceState).toEqual({ status: "failed", operation: "save", cause });
  });

  it.each(["QuotaExceededError", "SecurityError"])(
    "retains and retries the latest draft after %s",
    (name) => {
      composer().loadSession("unsaved");
      composer().setValue("saved draft");
      composer().addPaste("  /literal evidence 保留\r\n".repeat(128));
      const confirmed = localStorage.getItem(STORAGE_KEY);
      const cause = new DOMException("storage refused the write", name);
      const write = refuseWrites(cause);

      expect(() => composer().setValue("  newer draft 保留  ")).not.toThrow();
      composer().setValue("  latest draft 保留\n");
      const draft = composer().composer.draft;
      expect(composer().persistenceState).toEqual({ status: "failed", operation: "save", cause });
      expect(testClient.composer.hasUnconfirmedDrafts()).toBe(true);
      expect(localStorage.getItem(STORAGE_KEY)).toBe(confirmed);
      expect(() => testClient.composer.access.retryPersistence()).not.toThrow();
      expect(composer().composer.draft).toBe(draft);

      write.restore();
      testClient.composer.access.retryPersistence();
      expect(composer().persistenceState).toEqual({ status: "saved" });
      expect(testClient.composer.hasUnconfirmedDrafts()).toBe(false);
      expect(composer().composer.draft).toBe(draft);
      const restored = coldComposer();
      restored.activate(testClient.runtime.endpoint.currentRuntimeEndpoint());
      restored.store.getState().loadSession("unsaved");
      expect(restored.store.getState().composer.draft).toEqual(draft);
    },
  );

  it("keeps the original encoding cause without escaping authoring", () => {
    composer().setValue("confirmed");
    const confirmed = localStorage.getItem(STORAGE_KEY);
    const cause = { detail: "  encoding refused 保留\noriginal cause" };
    vi.spyOn(JSON, "stringify").mockImplementationOnce(() => {
      throw cause;
    });

    expect(() => composer().setValue("unconfirmed")).not.toThrow();
    expect(composer().composer.draft.value).toBe("unconfirmed");
    expect(composer().persistenceState).toEqual({ status: "failed", operation: "save", cause });
    expect(localStorage.getItem(STORAGE_KEY)).toBe(confirmed);
    testClient.composer.access.retryPersistence();
    expect(composer().persistenceState).toEqual({ status: "saved" });
    expect(localStorage.getItem(STORAGE_KEY)).toContain("unconfirmed");
  });

  it("still reads an image and consumes an accepted input once while saving fails", async () => {
    const reading =
      Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    const read = vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    composer().setValue("inspect this image");
    const write = refuseWrites(new DOMException("full", "QuotaExceededError"));
    try {
      expect(() => composer().addImageFiles([new File([""], "proof.png")])).not.toThrow();
      expect(read).toHaveBeenCalledOnce();
      expect(composer().composer.draft.imageAttachments).toMatchObject([{ status: "pending" }]);
      reading.resolve({ mime: "image/png", data: "recorded-image", name: "proof.png" });
      await vi.waitFor(() => expect(composer().composer.draft.attachmentsReady).toBe(true));
      const sendInput = vi.fn(() => true);
      const restoreFocus = vi.fn();
      expect(() =>
        submitComposer({
          draft: composer().captureSubmission(),
          sendInput,
          canSend: () => true,
          restoreFocus,
        }),
      ).not.toThrow();
      expect(sendInput).toHaveBeenCalledExactlyOnceWith({
        parts: [
          { kind: "text", text: "inspect this image" },
          { kind: "image", mime: "image/png", data: "recorded-image" },
        ],
      });
      expect(restoreFocus).toHaveBeenCalledOnce();
      expect(composer().composer.draft.value).toBe("");
      expect(composer().composer.draft.imageAttachments).toEqual([]);
      expect(composer().persistenceState).toMatchObject({ status: "failed", operation: "save" });
      write.restore();
      testClient.composer.access.retryPersistence();
      expect(composer().persistenceState).toEqual({ status: "saved" });
      expect(sendInput).toHaveBeenCalledOnce();
    } finally {
      reading.resolve({ mime: "image/png", data: "recorded-image", name: "proof.png" });
      await reading.promise;
    }
  });

  it.each([
    { name: "clearing", change: () => composer().clear() },
    { name: "discarding", change: () => composer().discardSession("removed") },
  ])("retains an unconfirmed $name removal until retry", ({ change }) => {
    composer().loadSession("kept");
    composer().setValue("other Session stays saved");
    composer().loadSession("removed");
    composer().setValue("remove this saved draft");
    const confirmed = localStorage.getItem(STORAGE_KEY);
    const write = refuseWrites(new DOMException("full", "QuotaExceededError"));
    expect(change).not.toThrow();
    expect(composer().composer.draft.value).toBe("");
    expect(localStorage.getItem(STORAGE_KEY)).toBe(confirmed);
    expect(testClient.composer.hasUnconfirmedDrafts()).toBe(true);
    write.restore();
    testClient.composer.access.retryPersistence();
    const restored = coldComposer();
    restored.activate(testClient.runtime.endpoint.currentRuntimeEndpoint());
    restored.store.getState().loadSession("removed");
    expect(restored.store.getState().composer.draft.value).toBe("");
    restored.store.getState().loadSession("kept");
    expect(restored.store.getState().composer.draft.value).toBe("other Session stays saved");
  });

  it("retains inactive endpoint failures for exit observation and recovery copying", () => {
    const targetA = "https://unconfirmed-a.test";
    const targetB = "https://unconfirmed-b.test";
    testClient.composer.activate(targetA);
    composer().loadSession("shared");
    const write = refuseWrites(
      new Error("write refused"),
      `flame.composer:${encodeURIComponent(targetA)}`,
    );
    const observed: boolean[] = [];
    const stop = testClient.composer.subscribePersistence(() =>
      observed.push(testClient.composer.hasUnconfirmedDrafts()),
    );
    try {
      composer().setValue("  draft on A 保留  ");
      composer().addPaste("/not a command\r\noriginal evidence");
      const draft = composer().composer.draft;
      testClient.composer.activate(targetB);
      composer().loadSession("shared");
      composer().setValue("confirmed on B");
      expect(composer().persistenceState).toEqual({ status: "saved" });
      expect(testClient.composer.hasUnconfirmedDrafts()).toBe(true);
      expect(JSON.parse(testClient.composer.access.getRecoveryDrafts())).toEqual({
        targets: [
          {
            active: false,
            drafts: {
              shared: {
                value: draft.value,
                pastes: draft.pastes.map(({ id, text }) => ({ id, text })),
              },
            },
          },
        ],
      });
      testClient.composer.activate(targetA);
      expect(composer().composer.draft).toBe(draft);
      write.restore();
      testClient.composer.access.retryPersistence();
      expect(testClient.composer.hasUnconfirmedDrafts()).toBe(false);
      expect(observed[0]).toBe(true);
      expect(observed.at(-1)).toBe(false);
    } finally {
      stop();
    }
  });
});

describe("unread draft protection", () => {
  it("retries a refused read without overwriting the confirmed Session collection", () => {
    composer().loadSession("alpha");
    composer().setValue("saved alpha");
    composer().loadSession("beta");
    composer().setValue("saved beta");
    const confirmed = localStorage.getItem(STORAGE_KEY);
    const cause = new DOMException("read denied", "SecurityError");
    const read = refuseReads(cause);
    const write = read.storage.setItem;
    const restored = coldComposer();
    restored.activate(testClient.runtime.endpoint.currentRuntimeEndpoint());
    restored.store.getState().loadSession("beta");
    restored.store.getState().setValue("");
    restored.store.getState().removeImage("missing");
    restored.store.getState().removePaste("missing");
    restored.store
      .getState()
      .setModel({ kind: "explicit", provider: "recorded", model: "selected" });
    expect(restored.store.getState().persistenceState).toEqual({
      status: "failed",
      operation: "read",
      cause,
    });
    expect(write).not.toHaveBeenCalled();
    read.restore();
    restored.retryPersistence();
    expect(restored.store.getState().composer.draft.value).toBe("saved beta");
    expect(restored.store.getState().modelPreference).toEqual({
      kind: "explicit",
      provider: "recorded",
      model: "selected",
    });
    expect(restored.store.getState().persistenceState).toEqual({ status: "saved" });
    expect(localStorage.getItem(STORAGE_KEY)).toBe(confirmed);
    expect(write).not.toHaveBeenCalled();
    restored.store.getState().loadSession("alpha");
    expect(restored.store.getState().composer.draft.value).toBe("saved alpha");
  });

  it("keeps authored recovery and the unread confirmed payload separate", () => {
    composer().loadSession("alpha");
    composer().setValue("unread alpha");
    composer().loadSession("beta");
    composer().setValue("unread beta");
    const confirmed = localStorage.getItem(STORAGE_KEY);
    const cause = new DOMException("private-token must not be exported", "SecurityError");
    const read = refuseReads(cause);
    const write = read.storage.setItem;
    const restored = coldComposer();
    restored.activate(testClient.runtime.endpoint.currentRuntimeEndpoint());
    restored.store.getState().loadSession("alpha");
    restored.store.getState().setValue("  authored recovery 保留\n");
    restored.store.getState().addPaste("/literal paste\r\n  original spacing");
    const draft = restored.store.getState().composer.draft;
    read.restore();
    restored.retryPersistence();
    expect(restored.store.getState().composer.draft).toBe(draft);
    expect(restored.store.getState().persistenceState).toEqual({
      status: "recovery-required",
      cause,
    });
    expect(write).not.toHaveBeenCalled();
    expect(localStorage.getItem(STORAGE_KEY)).toBe(confirmed);
    const recovery = restored.getRecoveryDrafts();
    expect(JSON.parse(recovery)).toEqual({
      targets: [
        {
          active: true,
          drafts: {
            alpha: {
              value: draft.value,
              pastes: draft.pastes.map(({ id, text }) => ({ id, text })),
            },
          },
        },
      ],
    });
    expect(recovery).not.toContain("private-token");
    expect(recovery).not.toContain(testClient.runtime.endpoint.currentRuntimeEndpoint());
  });

  it("does not replace malformed confirmed bytes with an initialized empty draft", () => {
    const raw = "{unfinished saved draft";
    localStorage.setItem(STORAGE_KEY, raw);
    const replacement = mockLocalStorage();
    const write = replacement.storage.setItem;
    const restored = coldComposer();
    restored.activate(testClient.runtime.endpoint.currentRuntimeEndpoint());
    restored.store.getState().loadSession("original");
    restored.retryPersistence();
    expect(restored.store.getState().persistenceState).toMatchObject({
      status: "failed",
      operation: "read",
      cause: expect.any(SyntaxError),
    });
    expect(write).not.toHaveBeenCalled();
    expect(localStorage.getItem(STORAGE_KEY)).toBe(raw);
  });

  it("does not replace an image selected while an original draft read was unavailable", async () => {
    composer().setValue("confirmed before the read refusal");
    const confirmed = localStorage.getItem(STORAGE_KEY);
    const cause = new DOMException("read refused", "SecurityError");
    const read = refuseReads(cause);
    const reading =
      Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    const restored = coldComposer();
    restored.activate(testClient.runtime.endpoint.currentRuntimeEndpoint());
    try {
      restored.store.getState().addImageFiles([new File([""], "retained.png")]);
      const selection = restored.store.getState().composer.draft.imageAttachments[0]!;
      read.restore();
      restored.retryPersistence();
      expect(restored.store.getState().composer.draft.imageAttachments).toEqual([selection]);
      expect(restored.store.getState().persistenceState).toEqual({
        status: "recovery-required",
        cause,
      });
      reading.resolve({ mime: "image/png", data: "retained", name: "retained.png" });
      await vi.waitFor(() =>
        expect(restored.store.getState().composer.draft.images).toMatchObject([
          { id: selection.id, data: "retained" },
        ]),
      );
      expect(localStorage.getItem(STORAGE_KEY)).toBe(confirmed);
    } finally {
      reading.resolve({ mime: "image/png", data: "retained", name: "retained.png" });
      await reading.promise;
    }
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
