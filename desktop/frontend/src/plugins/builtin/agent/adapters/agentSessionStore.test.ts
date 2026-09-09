import { beforeEach, describe, expect, it } from "vitest";
import { useAgentSessionStore } from "./agentSessionStore";

const store = () => useAgentSessionStore.getState();

beforeEach(() => {
  useAgentSessionStore.setState({
    openSessionIds: [],
    lastSessionId: "",
    draftSessionIds: new Set(),
    freshDraftSessionIds: new Set(),
  });
});

// This store is memory, not location: which session is active lives in the
// URL (see lib/navigation). What is asserted here is the tab set, the cold-start
// seed, and draft ownership — plus the pruning that keeps it from growing.
describe("the open set", () => {
  it("holds a session open once", () => {
    store().holdOpen("s1");
    store().holdOpen("s1");
    expect(store().openSessionIds).toEqual(["s1"]);
  });

  it("releases one without touching the others", () => {
    store().holdOpen("s1");
    store().holdOpen("s2");
    store().release("s1");
    expect(store().openSessionIds).toEqual(["s2"]);
  });

  it("retains only the ids boot reconciliation kept", () => {
    store().holdOpen("s1");
    store().holdOpen("s2");
    store().retainOnly(["s2"]);
    expect(store().openSessionIds).toEqual(["s2"]);
  });
});

describe("the cold-start seed", () => {
  it("remembers where the user was", () => {
    store().rememberSession("s1");
    expect(store().lastSessionId).toBe("s1");
  });
});

// The version is the discard switch: bumping it must leave no trace of the old
// payload — including in storage, or the next boot reads it again.
describe("storage written by an older version", () => {
  it("boots on defaults and restamps storage at the current version", async () => {
    localStorage.setItem(
      "flame.agent-session",
      JSON.stringify({ state: { openSessionIds: ["stale"], lastSessionId: "stale" }, version: 1 }),
    );

    await useAgentSessionStore.persist.rehydrate();

    expect(store().openSessionIds).toEqual([]);
    expect(store().lastSessionId).toBe("");

    const stored = JSON.parse(localStorage.getItem("flame.agent-session") ?? "null") as {
      version: number;
    };
    expect(stored.version).toBe(useAgentSessionStore.persist.getOptions().version);
  });
});

/**
 * What the store writes, it must read back — stated without naming a field.
 *
 * `partialize` decides what is written and a Zod schema decides what is accepted, and they
 * are two hand-maintained lists: the comment above the schema says "Mirrors `partialize`
 * below", which is a fact with two owners. A Zod object STRIPS unknown keys, so a field added
 * to one and not the other is written to storage and silently dropped on the next boot — the
 * state simply does not survive a restart, with no error anywhere.
 *
 * Every other test here writes the payload BY HAND, so all of them would pass through that.
 * This one lets the store write its own, and compares the store's choice against itself, so
 * it cannot be satisfied by a subset and does not have to be edited when the shape changes.
 */
describe("the round trip", () => {
  it("gets back everything it chose to persist", async () => {
    store().holdOpen("s1");
    store().holdOpen("s2");
    store().rememberSession("s2");
    store().markDraft("s2");

    const key = useAgentSessionStore.persist.getOptions().name!;
    const payload = localStorage.getItem(key) ?? "null";
    const written = (JSON.parse(payload) as { state: unknown }).state;
    expect(written).toBeTruthy();

    // Emptying the store PERSISTS the empty state, so the payload has to go back before the
    // rehydrate — otherwise this reads back what the clearing wrote and passes on nothing.
    useAgentSessionStore.setState({
      openSessionIds: [],
      lastSessionId: "",
      draftSessionIds: new Set(),
      freshDraftSessionIds: new Set(),
    });
    localStorage.setItem(key, payload);
    await useAgentSessionStore.persist.rehydrate();

    const partialize = useAgentSessionStore.persist.getOptions().partialize!;
    const readBack = JSON.parse(JSON.stringify(partialize(store() as never)));
    expect(readBack).toEqual(written);
  });
});

describe("drafts", () => {
  it("marks and graduates a draft", () => {
    store().markDraft("s1");
    expect(store().draftSessionIds.has("s1")).toBe(true);
    expect(store().freshDraftSessionIds.has("s1")).toBe(true);

    store().graduateDraft("s1");
    expect(store().draftSessionIds.has("s1")).toBe(false);
    expect(store().freshDraftSessionIds.has("s1")).toBe(false);
  });

  it("restores draft ownership without restoring the in-process freshness proof", async () => {
    localStorage.setItem(
      "flame.agent-session",
      JSON.stringify({
        state: {
          openSessionIds: ["s1"],
          lastSessionId: "s1",
          draftSessionIds: ["s1"],
        },
        version: useAgentSessionStore.persist.getOptions().version,
      }),
    );

    await useAgentSessionStore.persist.rehydrate();

    expect(store().draftSessionIds).toEqual(new Set(["s1"]));
    expect(store().freshDraftSessionIds).toEqual(new Set());
  });

  it("graduating a session that isn't a draft changes nothing", () => {
    const before = store().draftSessionIds;
    store().graduateDraft("s1");
    expect(store().draftSessionIds).toBe(before);
  });

  it("prunes draft refs when a session stops being open", () => {
    store().holdOpen("s1");
    store().markDraft("s1");

    store().release("s1");

    expect(store().draftSessionIds.has("s1")).toBe(false);
    expect(store().freshDraftSessionIds.has("s1")).toBe(false);
  });
});
