import { beforeEach, describe, expect, it } from "vitest";
import { useAgentSessionStore } from "./agentSessionStore";

const store = () => useAgentSessionStore.getState();

beforeEach(() => {
  useAgentSessionStore.setState({
    openSessionIds: [],
    lastSessionId: "",
  });
});

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

describe("a stored payload of another shape", () => {
  it("boots on defaults", async () => {
    localStorage.setItem(
      useAgentSessionStore.persist.getOptions().name!,
      JSON.stringify({ state: { openSessionIds: "stale", lastSessionId: "stale" } }),
    );

    await useAgentSessionStore.persist.rehydrate();

    expect(store().openSessionIds).toEqual([]);
    expect(store().lastSessionId).toBe("");
  });
});

describe("the round trip", () => {
  it("gets back everything it chose to persist", async () => {
    store().holdOpen("s1");
    store().holdOpen("s2");
    store().rememberSession("s2");

    const key = useAgentSessionStore.persist.getOptions().name!;
    const payload = localStorage.getItem(key) ?? "null";
    const written = (JSON.parse(payload) as { state: unknown }).state;
    expect(written).toBeTruthy();

    useAgentSessionStore.setState({
      openSessionIds: [],
      lastSessionId: "",
    });
    localStorage.setItem(key, payload);
    await useAgentSessionStore.persist.rehydrate();

    const partialize = useAgentSessionStore.persist.getOptions().partialize!;
    const readBack = JSON.parse(JSON.stringify(partialize(store() as never)));
    expect(readBack).toEqual(written);
  });
});
