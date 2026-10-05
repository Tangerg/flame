import { describe, expect, it } from "vitest";
import { closeOpenSession, openSession, reconcileOpenSessions } from "./sessionSelectionModel";

describe("sessionSelectionModel", () => {
  it("holds a session open, and only once", () => {
    expect(openSession(["s1"], "s2")).toEqual(["s1", "s2"]);
    const open = ["s1"];
    expect(openSession(open, "s1")).toBe(open);
  });

  it("closes the active session by selecting its adjacent survivor", () => {
    expect(
      closeOpenSession({ activeSessionId: "s2", openSessionIds: ["s1", "s2", "s3"] }, "s2"),
    ).toEqual({
      activeSessionId: "s3",
      openSessionIds: ["s1", "s3"],
    });
    expect(
      closeOpenSession({ activeSessionId: "s3", openSessionIds: ["s1", "s2", "s3"] }, "s3"),
    ).toEqual({
      activeSessionId: "s2",
      openSessionIds: ["s1", "s2"],
    });
  });

  it("reconciles persisted open sessions against the Runtime's sessions", () => {
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "s1",
          openSessionIds: ["s1", "s2", "s3"],
        },
        ["s1", "s3"],
      ),
    ).toEqual({ activeSessionId: "s1", openSessionIds: ["s1", "s3"] });
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "s1",
          openSessionIds: ["s1", "s2", "s3"],
        },
        ["s2", "s3"],
      ),
    ).toEqual({ activeSessionId: "s3", openSessionIds: ["s2", "s3"] });
  });

  it("returns null when persisted open sessions are already valid", () => {
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "s1",
          openSessionIds: ["s1", "s2"],
        },
        ["s1", "s2"],
      ),
    ).toBeNull();
  });

  it("holds an authoritative deep-linked active session open during reconciliation", () => {
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "deep-link",
          openSessionIds: ["stale"],
        },
        ["deep-link"],
      ),
    ).toEqual({ activeSessionId: "deep-link", openSessionIds: ["deep-link"] });
  });

  it("closes an open session the Runtime no longer lists", () => {
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "deleted-remotely",
          openSessionIds: ["deleted-remotely"],
        },
        [],
      ),
    ).toEqual({ activeSessionId: "", openSessionIds: [] });
  });
});
