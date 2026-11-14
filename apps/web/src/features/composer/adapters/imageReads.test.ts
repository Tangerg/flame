import { afterEach, expect, it, onTestFinished, vi } from "vitest";
import { createComposerStore } from "@/features/composer/adapters/composerStore";

afterEach(() => vi.restoreAllMocks());
function controlledReads() {
  const started: FileReader[] = [];
  const read = vi.spyOn(FileReader.prototype, "readAsDataURL").mockImplementation(function (
    this: FileReader,
  ) {
    started.push(this);
  });
  const abort = vi.spyOn(FileReader.prototype, "abort").mockImplementation(function (
    this: FileReader,
  ) {
    this.dispatchEvent(new ProgressEvent("abort"));
  });
  return { started, read, abort };
}
function owner() {
  const owner = createComposerStore();
  onTestFinished(() => owner.dispose());
  owner.activate("endpoint");
  return owner;
}
const file = (name: string) => new File(["image"], name, { type: "image/png" });

it("runs one FileReader at a time and never starts a removed queued file", async () => {
  const reads = controlledReads();
  const drafts = owner();
  drafts.store.getState().addImageFiles([file("first"), file("second"), file("third")]);
  expect(reads.read).toHaveBeenCalledTimes(1);
  const [first, second] = drafts.store.getState().composer.draft.imageAttachments;
  drafts.store.getState().removeImage(second!.id);
  drafts.store.getState().removeImage(first!.id);
  expect(reads.abort).toHaveBeenCalledOnce();
  await vi.waitFor(() => expect(reads.read).toHaveBeenCalledTimes(2));
  expect(reads.read.mock.calls.map(([file]) => (file as File).name)).toEqual(["first", "third"]);
  expect(drafts.store.getState().composer.draft.imageAttachments).toMatchObject([
    { name: "third", status: "pending" },
  ]);
});

it.each(["clear", "replace", "discard", "dispose"])(
  "aborts active and queued reads on %s",
  async (action) => {
    const reads = controlledReads();
    const drafts = owner();
    drafts.store.getState().loadSession("session");
    drafts.store.getState().addImageFiles([file("first"), file("queued")]);
    if (action === "clear") drafts.store.getState().clear();
    else if (action === "replace") drafts.store.getState().replaceDraft({ text: "replacement" });
    else if (action === "discard") drafts.store.getState().discardSession("session");
    else drafts.dispose();
    await Promise.resolve();
    await Promise.resolve();
    expect(reads.abort).toHaveBeenCalledOnce();
    expect(reads.read).toHaveBeenCalledOnce();
    expect(
      drafts.store
        .getState()
        .composer.draft.imageAttachments.some((image) => image.status === "pending"),
    ).toBe(false);
  },
);

it("retires pending endpoint images visibly while keeping ready images and authored text on return", async () => {
  const reads = controlledReads();
  const drafts = owner();
  drafts.store.getState().loadSession("same");
  drafts.store.getState().setValue("keep original text");
  drafts.store.getState().addImages([{ mime: "image/png", data: "ready" }]);
  drafts.store.getState().addImageFiles([file("active"), file("queued")]);
  drafts.activate("successor");
  drafts.store.getState().loadSession("same");
  drafts.store.getState().setValue("successor text");
  expect(reads.abort).toHaveBeenCalledOnce();
  drafts.activate("endpoint");
  drafts.store.getState().loadSession("same");
  expect(drafts.store.getState().composer.draft.value).toBe("keep original text");
  expect(
    drafts.store.getState().composer.draft.imageAttachments.map((image) => image.status),
  ).toEqual(["ready", "failed", "failed"]);
  expect(drafts.store.getState().composer.draft.images[0]?.data).toBe("ready");
  expect(drafts.store.getState().composer.draft.attachmentsReady).toBe(false);
});

it("keeps an original Session's active read when another Session becomes current", () => {
  const reads = controlledReads();
  const drafts = owner();
  drafts.store.getState().loadSession("first");
  drafts.store.getState().addImageFiles([file("original")]);
  drafts.store.getState().loadSession("second");
  drafts.store.getState().addImageFiles([file("later")]);
  expect(reads.abort).not.toHaveBeenCalled();
  expect(reads.read).toHaveBeenCalledOnce();
});
