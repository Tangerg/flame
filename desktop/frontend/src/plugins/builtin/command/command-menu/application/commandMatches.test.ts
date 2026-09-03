import { describe, expect, it } from "vitest";
import { matchCommands } from "./commandMatches";

const commands = [
  { id: "b", label: "New chat" },
  { id: "a", label: "Close panel or chat" },
  { id: "c", label: "Toggle sidebar rail" },
];

describe("matchCommands", () => {
  it("shows everything for an empty query, in reading order", () => {
    expect(matchCommands(commands, "  ").map((c) => c.id)).toEqual(["a", "b", "c"]);
  });

  it("matches anywhere in the label, ignoring case", () => {
    expect(matchCommands(commands, "CHAT").map((c) => c.id)).toEqual(["a", "b"]);
  });

  it("answers nothing when nothing matches", () => {
    expect(matchCommands(commands, "zzz")).toEqual([]);
  });
});
