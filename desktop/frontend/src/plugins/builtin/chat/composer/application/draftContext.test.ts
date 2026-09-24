import { describe, expect, it } from "vitest";
import { draftMentions, formatFileReference, removeMention } from "./draftContext";

const known = (...paths: string[]) => new Set(paths);

describe("draftMentions", () => {
  it("finds a file the draft attached", () => {
    expect(draftMentions("look at @src/app.ts please", known("src/app.ts"))).toEqual([
      { path: "src/app.ts", start: 8, end: 19 },
    ]);
  });

  it("leaves an @word alone when no such file exists", () => {
    expect(draftMentions("thanks @alice", known("src/app.ts"))).toEqual([]);
  });

  it("ignores an address, which is the reason the token must start a word", () => {
    expect(draftMentions("mail user@host.com", known("host.com"))).toEqual([]);
  });

  it("keeps the same file twice apart, so closing one chip keeps the other", () => {
    const found = draftMentions("@a.ts and @a.ts", known("a.ts"));
    expect(found.map((m) => m.start)).toEqual([0, 10]);
  });

  it("finds one at the very start and the very end", () => {
    expect(draftMentions("@a.ts", known("a.ts"))).toHaveLength(1);
    expect(draftMentions("see @b.ts", known("b.ts"))).toHaveLength(1);
  });
});

describe("formatFileReference", () => {
  it.each(["docs/my notes.md", "src/(group)/页面.tsx", 'odd "name".txt', "back\\slash.txt"])(
    "round-trips %s through the draft",
    (path) => {
      const value = `see ${formatFileReference(path)} now`;
      const [found] = draftMentions(value, known(path));
      expect(found?.path).toBe(path);
      expect(value.slice(found!.start, found!.end)).toBe(formatFileReference(path));
    },
  );

  it("keeps a plain path unquoted", () => {
    expect(formatFileReference("src/app.ts")).toBe("@src/app.ts");
  });
});

describe("removeMention", () => {
  const only = (value: string, path: string) => draftMentions(value, known(path))[0]!;

  it("closes the gap it leaves rather than doubling the space", () => {
    const value = "look at @src/app.ts please";
    expect(removeMention(value, only(value, "src/app.ts"))).toBe("look at please");
  });

  it("takes the whole thing when the draft was only a mention", () => {
    expect(removeMention("@a.ts", only("@a.ts", "a.ts"))).toBe("");
  });

  it("removes a quoted reference whole", () => {
    const value = 'open @"my file.md" next';
    expect(removeMention(value, only(value, "my file.md"))).toBe("open next");
  });

  it("removes the occurrence it was given, not the first match of the same path", () => {
    const value = "@a.ts and @a.ts";
    const second = draftMentions(value, known("a.ts"))[1]!;
    expect(removeMention(value, second)).toBe("@a.ts and");
  });
});
