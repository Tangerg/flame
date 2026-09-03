import { describe, expect, it } from "vitest";
import { matchCommands } from "./commandMatches";

const choice = (key: string, label: string) => ({ key, label, run: () => undefined });
const commands = [
  choice("b", "New chat"),
  choice("a", "Close panel or chat"),
  choice("c", "View: Terminal"),
];

describe("matchCommands", () => {
  it("shows everything for an empty query, in reading order", () => {
    expect(matchCommands(commands, "  ").map((c) => c.key)).toEqual(["a", "b", "c"]);
  });

  it("matches anywhere in the label, ignoring case", () => {
    expect(matchCommands(commands, "CHAT").map((c) => c.key)).toEqual(["a", "b"]);
  });

  it("answers nothing when nothing matches", () => {
    expect(matchCommands(commands, "zzz")).toEqual([]);
  });
});
