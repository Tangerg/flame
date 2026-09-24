import { describe, expect, it } from "vitest";
import { Composer, SCRATCH_SESSION_ID, ComposerDraft } from "./composer";

const typed = (composer: Composer, text: string) => composer.edit((draft) => draft.withValue(text));

describe("Composer", () => {
  it("keeps each session's draft apart", () => {
    const composer = typed(Composer.empty(), "scratch").activate("s1");
    expect(composer.draft.value).toBe("");
    expect(typed(composer, "one").activate(SCRATCH_SESSION_ID).draft.value).toBe("scratch");
  });

  it("derives the active draft rather than storing a second copy", () => {
    const composer = typed(Composer.empty().activate("s1"), "hello");
    expect(composer.activate("s2").activate("s1").draft.value).toBe("hello");
  });

  it("keeps the scratch and active drafts when pruning to the live set", () => {
    const composer = typed(Composer.empty(), "scratch")
      .activate("s1")
      .edit((d) => d.withValue("one"))
      .activate("s2")
      .edit((d) => d.withValue("two"))
      .activate("stale")
      .edit((d) => d.withValue("gone"))
      .activate("s2")
      .prune(new Set(["s1"]));

    expect([...composer.durableDraftTexts().keys()].sort()).toEqual([
      SCRATCH_SESSION_ID,
      "s1",
      "s2",
    ]);
  });

  it("does not record blank text or an immediate repeat", () => {
    const once = typed(Composer.empty().record("hello").record("   ").record("hello"), "draft");
    const oldest = once.recallOlder()!.recallOlder()!;
    expect(oldest.draft.value).toBe("hello");
    expect(oldest.recallNewer()!.draft.value).toBe("draft");
  });

  it("restores what was typed when stepping past the newest entry", () => {
    const composer = typed(Composer.empty().record("hello"), "draft");
    const older = composer.recallOlder()!;
    expect(older.draft.value).toBe("hello");
    expect(older.recallNewer()!.draft.value).toBe("draft");
  });

  it("answers null when there is nothing to recall, so a keymap can fall through", () => {
    expect(Composer.empty().recallOlder()).toBeNull();
    expect(Composer.empty().record("hello").recallNewer()).toBeNull();
  });

  it("leaves recall on any edit that is not recall's own", () => {
    const recalling = typed(Composer.empty().record("hello"), "draft").recallOlder()!;
    expect(recalling.isRecalling).toBe(true);
    expect(typed(recalling, "typing").isRecalling).toBe(false);
    expect(recalling.clear().isRecalling).toBe(false);
    expect(recalling.activate("other").isRecalling).toBe(false);
    expect(recalling.record("sent").isRecalling).toBe(false);
  });

  it("answers one stable instance for a session never typed into", () => {
    const composer = Composer.empty();
    expect(composer.draft).toBe(composer.activate("s1").draft);
    expect(composer.draft.images).toBe(composer.activate("s2").draft.images);

    const active = composer.activate("s1");
    expect(active.activate("s1")).toBe(active);
  });

  it("persists only non-empty draft text", () => {
    const composer = typed(Composer.empty().activate("s1"), "kept").activate("s2");
    expect([...composer.durableDraftTexts()]).toEqual([["s1", "kept"]]);
  });

  it("restores durable text into empty drafts", () => {
    const restored = Composer.restoreDrafts(new Map([["s1", "kept"]])).activate("s1");
    expect(restored.draft.value).toBe("kept");
    expect(restored.draft.images).toEqual([]);
  });
});

describe("ComposerDraft pastes", () => {
  const withPaste = ComposerDraft.empty()
    .withValue("explain this")
    .withPastes([{ id: "p1", text: "line 1\nline 2", lines: 2 }]);

  it("edits a paste in place and recounts its lines", () => {
    const edited = withPaste.editPaste("p1", "a\nb\nc");
    expect(edited.pastes).toEqual([{ id: "p1", text: "a\nb\nc", lines: 3 }]);
    expect(edited.value).toBe("explain this");
  });

  it("drops a paste edited down to nothing", () => {
    expect(withPaste.editPaste("p1", "  \n ").pastes).toEqual([]);
  });

  it("puts a paste back into the message the way it would have been sent", () => {
    const restored = withPaste.restorePaste("p1");
    expect(restored.pastes).toEqual([]);
    expect(restored.value).toBe("explain this\n\nline 1\nline 2");
  });

  it("ignores a paste it does not hold", () => {
    expect(withPaste.restorePaste("missing")).toBe(withPaste);
  });
});
