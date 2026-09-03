import { describe, expect, it } from "vitest";
import type { AgentSessionSummary } from "./sessionQueries";
import { stepAgentSession } from "./stepSession";

const sessions = ["a", "b", "c"].map((id) => ({ id }) as AgentSessionSummary);

describe("stepAgentSession", () => {
  it("moves one place in the order the index shows", () => {
    expect(stepAgentSession(sessions, "a", 1)).toBe("b");
    expect(stepAgentSession(sessions, "b", -1)).toBe("a");
  });

  it("wraps at both ends", () => {
    expect(stepAgentSession(sessions, "c", 1)).toBe("a");
    expect(stepAgentSession(sessions, "a", -1)).toBe("c");
  });

  // A deletion leaves the location pointing at an id the list no longer carries, and so does
  // a cold start with nothing selected. Both enter from the end the step came from.
  it("enters the list from the end the step came from", () => {
    expect(stepAgentSession(sessions, "", 1)).toBe("a");
    expect(stepAgentSession(sessions, "", -1)).toBe("c");
    expect(stepAgentSession(sessions, "deleted", 1)).toBe("a");
    expect(stepAgentSession(sessions, "deleted", -1)).toBe("c");
  });

  it("answers nothing when there is nowhere to go", () => {
    expect(stepAgentSession([], "a", 1)).toBeUndefined();
    expect(stepAgentSession(sessions.slice(0, 1), "a", 1)).toBe("a");
  });
});
