import { describe, expect, it } from "vitest";
import { Composer } from "../domain/composer";
import { createComposerSendIntent } from "../domain/sendIntent";
import { parsePersistedComposer, persistedComposerDrafts } from "./persistedDrafts";

function restore(composer: Composer): Composer {
  const encoded = JSON.stringify({ drafts: persistedComposerDrafts(composer) });
  const restored = parsePersistedComposer(JSON.parse(encoded));
  expect(restored).not.toBeNull();
  return restored!.activate(composer.activeSessionId);
}

describe("persisted authored text", () => {
  it.each(["", "explain these logs  "])("retains pastes with value %j", (value) => {
    const before = Composer.empty()
      .activate("session")
      .edit((draft) =>
        draft.withValue(value).withPastes([
          { id: "first", text: "/not-a-command\n".repeat(50), lines: 51 },
          { id: "second", text: "  exact whitespace\r\n", lines: 2 },
        ]),
      );
    const after = restore(before);
    expect(after.draft).toEqual(before.draft);
    expect(createComposerSendIntent(after.draft)).toEqual(createComposerSendIntent(before.draft));
    expect(restore(after).draft).toEqual(after.draft);
  });

  it("retains edits and removals rather than resurrecting the original paste", () => {
    const before = Composer.empty()
      .activate("session")
      .edit((draft) => draft.withPastes([{ id: "paste", text: "original", lines: 1 }]))
      .edit((draft) => draft.editPaste("paste", "edited\ntext"));
    const edited = restore(before);
    expect(edited.draft.pastes).toEqual([{ id: "paste", text: "edited\ntext", lines: 2 }]);
    expect(restore(edited.edit((draft) => draft.editPaste("paste", ""))).draft.pastes).toEqual([]);
  });

  it("rejects obsolete and ambiguous records rather than guessing missing authored data", () => {
    expect(parsePersistedComposer({ drafts: { session: { value: "old" } } })).toBeNull();
    expect(
      parsePersistedComposer({
        drafts: {
          session: {
            value: "",
            pastes: [
              { id: "same", text: "one" },
              { id: "same", text: "two" },
            ],
          },
        },
      }),
    ).toBeNull();
  });
});
