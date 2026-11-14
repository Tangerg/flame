import { afterEach, expect, it, onTestFinished, vi } from "vitest";
import { DraftDatabase } from "@/features/composer/adapters/draftDatabase";
import { createComposerStore } from "@/features/composer/adapters/composerStore";
import { Composer, ComposerDraft } from "@/features/composer/domain/composer";
import { createComposerSendIntent } from "@/features/composer/domain/sendIntent";

afterEach(() => vi.restoreAllMocks());
function owner() {
  const owner = createComposerStore();
  onTestFinished(() => owner.dispose());
  return owner;
}
function database() {
  const database = new DraftDatabase();
  onTestFinished(() => database.close());
  return database;
}
const saved = async (value: ReturnType<typeof owner>) => {
  await vi.waitFor(() => expect(value.store.getState().persistenceState.status).toBe("saved"));
};

it("keeps current draft persistence independent of localStorage access", async () => {
  const read = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new DOMException("localStorage unavailable", "SecurityError");
  });
  const drafts = owner();
  drafts.activate("https://indexed-drafts.test");
  await saved(drafts);
  drafts.store.getState().setValue("  current draft 保留  ");
  await saved(drafts);
  const restored = owner();
  restored.activate("https://indexed-drafts.test");
  await saved(restored);
  expect(restored.store.getState().composer.draft.value).toBe("  current draft 保留  ");
  expect(read).not.toHaveBeenCalled();
});

async function persistActiveDraft(composer: Composer): Promise<Composer> {
  const storage = database();
  await storage.write("endpoint", new Map([[composer.activeSessionId, composer.draft]]));
  return Composer.restoreDrafts(await storage.read("endpoint")).activate(composer.activeSessionId);
}

it.each(["", "explain these logs  "])(
  "preserves pasted-text semantics with value %j",
  async (value) => {
    const before = Composer.empty()
      .activate("session")
      .edit((draft) =>
        draft.withValue(value).withPastes([
          { id: "first", text: "/not-a-command\n".repeat(50), lines: 51 },
          { id: "second", text: "  exact whitespace\r\n", lines: 2 },
        ]),
      );
    const after = await persistActiveDraft(before);
    expect(after.draft).toEqual(before.draft);
    expect(createComposerSendIntent(after.draft)).toEqual(createComposerSendIntent(before.draft));
    expect((await persistActiveDraft(after)).draft).toEqual(after.draft);
  },
);

it("restores edits and removals without resurrecting the original paste", async () => {
  const before = Composer.empty()
    .activate("session")
    .edit((draft) => draft.withPastes([{ id: "paste", text: "original", lines: 1 }]))
    .edit((draft) => draft.editPaste("paste", "edited\ntext"));
  const edited = await persistActiveDraft(before);
  expect(edited.draft.pastes).toEqual([{ id: "paste", text: "edited\ntext", lines: 2 }]);
  expect(
    (await persistActiveDraft(edited.edit((draft) => draft.editPaste("paste", "")))).draft.pastes,
  ).toEqual([]);
});

it("writes only the edited Session and reuses immutable pasted text instead of cloning it on each keystroke", async () => {
  const drafts = owner();
  drafts.activate("endpoint");
  await saved(drafts);
  const state = () => drafts.store.getState();
  for (const session of ["one", "two", "three", "four"]) {
    state().loadSession(session);
    state().addPaste("/large paste 保留\n".repeat(20000));
  }
  await saved(drafts);
  const put = vi.spyOn(IDBObjectStore.prototype, "put");
  const add = vi.spyOn(IDBObjectStore.prototype, "add");
  const localStorageWrite = vi.spyOn(Storage.prototype, "setItem");
  for (let i = 1; i <= 10; i++) {
    state().setValue(`text ${i}`);
    await saved(drafts);
  }
  const writes = put.mock.calls.filter((_, i) => {
    const store = put.mock.contexts[i];
    return store instanceof IDBObjectStore && store.name === "drafts";
  });
  expect(writes).toHaveLength(10);
  expect(writes.every(([value]) => (value as { session: string }).session === "four")).toBe(true);
  expect(
    writes.every(([value]) => !Object.hasOwn((value as { pastes: object[] }).pastes[0]!, "text")),
  ).toBe(true);
  expect(
    add.mock.contexts.filter((store) => store instanceof IDBObjectStore && store.name === "pastes"),
  ).toHaveLength(0);
  expect(localStorageWrite).not.toHaveBeenCalled();
  const restored = await database().read("endpoint");
  expect(restored.size).toBe(4);
  expect(restored.get("one")?.pastes[0]?.text).toBe("/large paste 保留\n".repeat(20000));
});

it("does not confirm request success when the transaction subsequently aborts", async () => {
  const drafts = owner();
  drafts.activate("endpoint");
  await saved(drafts);
  drafts.store.getState().setValue("confirmed");
  await saved(drafts);
  const original = IDBObjectStore.prototype.put;
  let abort = true;
  vi.spyOn(IDBObjectStore.prototype, "put").mockImplementation(function (
    this: IDBObjectStore,
    ...args
  ) {
    const request = original.apply(this, args);
    if (this.name === "drafts" && abort) {
      abort = false;
      request.addEventListener("success", () => this.transaction.abort());
    }
    return request;
  });
  drafts.store.getState().setValue("not committed");
  await vi.waitFor(() =>
    expect(drafts.store.getState().persistenceState).toMatchObject({
      status: "failed",
      operation: "save",
      cause: expect.objectContaining({ name: "AbortError" }),
    }),
  );
  expect(drafts.hasUnconfirmedDrafts()).toBe(true);
  expect((await database().read("endpoint")).get("")?.value).toBe("confirmed");
  drafts.retryPersistence();
  await saved(drafts);
  expect((await database().read("endpoint")).get("")?.value).toBe("not committed");
});

it("keeps a later draft pending when an older transaction completes and coalesces its queued revisions", async () => {
  const drafts = owner();
  drafts.activate("endpoint");
  await saved(drafts);
  const original = DraftDatabase.prototype.write;
  const firstCommitted = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  let first = true;
  const write = vi.spyOn(DraftDatabase.prototype, "write").mockImplementation(async function (
    this: DraftDatabase,
    ...args
  ) {
    await original.apply(this, args);
    if (first) {
      first = false;
      firstCommitted.resolve();
      await release.promise;
    }
  });
  drafts.store.getState().setValue("first");
  await firstCommitted.promise;
  for (let i = 1; i <= 40; i++) drafts.store.getState().setValue(`later ${i}`);
  expect(drafts.hasUnconfirmedDrafts()).toBe(true);
  expect(drafts.store.getState().persistenceState.status).toBe("pending");
  release.resolve();
  await saved(drafts);
  expect(write).toHaveBeenCalledTimes(2);
  expect((await database().read("endpoint")).get("")?.value).toBe("later 40");
});

it("replaces a pasted text record transactionally without changing its attachment identity", async () => {
  const storage = database();
  const first = ComposerDraft.empty().withPastes([{ id: "paste", text: "original", lines: 1 }]);
  await storage.write("endpoint", new Map([["session", first]]));
  await storage.write("endpoint", new Map([["session", first.editPaste("paste", "edited\ntext")]]));
  expect((await storage.read("endpoint")).get("session")).toEqual({
    value: "",
    pastes: [{ id: "paste", text: "edited\ntext" }],
  });
});

it("does not reuse a paste record retired by another owner of the same Session", async () => {
  const first = database();
  const second = database();
  const draft = ComposerDraft.empty().withPastes([{ id: "paste", text: "original", lines: 1 }]);
  await first.write("endpoint", new Map([["session", draft]]));
  await second.read("endpoint");
  await second.write("endpoint", new Map([["session", draft.editPaste("paste", "other window")]]));
  await first.write("endpoint", new Map([["session", draft.withValue("later edit")]]));
  expect((await database().read("endpoint")).get("session")).toEqual({
    value: "later edit",
    pastes: [{ id: "paste", text: "original" }],
  });
});
