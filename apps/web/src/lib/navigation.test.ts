import { beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
let navigation: ReturnType<typeof createMemoryNavigator>;
beforeEach(() => {
  navigation = createMemoryNavigator();
});

describe("a location patch", () => {
  it("leaves out means leave alone, null means clear", () => {
    navigation.go({ session: "sess_1", dock: "review", settings: "providers" });
    navigation.go({ settings: null });

    expect(navigation.get()).toEqual({
      session: "sess_1",
      view: null,
      dock: "review",
      settings: null,
      subagent: null,
    });
  });

  it("notifies subscribers with both sides of the move", () => {
    const seen = vi.fn();
    navigation.subscribe(seen);

    navigation.go({ session: "sess_1" });

    expect(seen).toHaveBeenCalledTimes(1);
    expect(seen.mock.calls[0]?.[0]).toMatchObject({ session: "sess_1" });
    expect(seen.mock.calls[0]?.[1]).toMatchObject({ session: "" });
  });

  it("is not a move when nothing changes", () => {
    navigation.go({ view: "diff" });
    const seen = vi.fn();
    navigation.subscribe(seen);

    navigation.go({ view: "diff" });

    expect(seen).not.toHaveBeenCalled();
  });
});

describe("history", () => {
  it("goes back to where it came from, and forward again", () => {
    navigation.go({ session: "sess_1" });
    navigation.go({ session: "sess_2" });

    navigation.back();
    expect(navigation.get().session).toBe("sess_1");

    navigation.forward();
    expect(navigation.get().session).toBe("sess_2");
  });

  it("stops at the ends instead of falling off them", () => {
    navigation.go({ session: "sess_1" });

    navigation.back();
    navigation.back();
    expect(navigation.get().session).toBe("");

    navigation.forward();
    navigation.forward();
    expect(navigation.get().session).toBe("sess_1");
  });

  it("replace corrects the current entry without adding one to go back to", () => {
    navigation.go({ session: "sess_restored" }, { replace: true });

    navigation.back();

    expect(navigation.get().session).toBe("sess_restored");
  });

  it("forward is dropped once a new move is made from a back entry", () => {
    navigation.go({ view: "a" });
    navigation.go({ view: "b" });
    navigation.back();

    navigation.go({ view: "c" });
    navigation.forward();

    expect(navigation.get().view).toBe("c");
  });
});
