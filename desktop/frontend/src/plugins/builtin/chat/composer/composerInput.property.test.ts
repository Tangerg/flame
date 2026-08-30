import { describe, expect, it } from "vitest";
import { Arbitrary, forEachSeed } from "@/test/arbitrary";
import { parseFileRefs } from "@/plugins/builtin/agent/public/fileRefs";
import { isLargePaste } from "./domain/largePaste";
import { draftMentions, removeMention } from "./application/draftContext";
import { activeMention } from "./application/fileMentions";
import { fuzzyFile } from "./application/fuzzyFile";

// Everything here reads text a person pasted or an agent produced, so the input
// space is "any string" rather than any shape the contract admits. These assert the
// properties that have to hold across all of it: a parser agrees with the text it
// parsed, a ranking stays inside its own candidates, and nothing splits a character.

function corpus(a: Arbitrary): string {
  return a.bool(0.25) ? `${a.text()} @${a.text()} ${a.text()}` : a.text();
}

describe("composer input, over arbitrary text", () => {
  it("splits file references into a partition that rebuilds the text", () => {
    forEachSeed(600, (a) => {
      const text = corpus(a);
      const rebuilt = parseFileRefs(text)
        .map((part) => {
          if (typeof part === "string") return part;
          const line = part.line ? `:${part.line}` : "";
          const column = part.column ? `:${part.column}` : "";
          return `${part.path}${line}${column}`;
        })
        .join("");
      expect(rebuilt).toBe(text);
    });
  });

  it("answers the paste heuristic for any text", () => {
    forEachSeed(600, (a) => {
      expect(typeof isLargePaste(corpus(a))).toBe("boolean");
    });
  });

  it("ranks only its own candidates, within the requested bound", () => {
    forEachSeed(400, (a) => {
      const paths = Array.from({ length: a.int(12) }, () => corpus(a));
      const limit = 1 + a.int(8);
      const ranked = fuzzyFile(corpus(a), paths, limit);
      expect(ranked.length).toBeLessThanOrEqual(limit);
      for (const path of ranked) expect(paths).toContain(path);
    });
  });

  it("points every chip at text the draft really contains", () => {
    forEachSeed(600, (a) => {
      const text = corpus(a);
      for (const mention of draftMentions(text)) {
        expect(text.slice(mention.start, mention.end)).toBe(`@${mention.path}`);
      }
    });
  });

  it("removes a chip without leaving its path behind or growing the draft", () => {
    forEachSeed(600, (a) => {
      const text = corpus(a);
      const mentions = draftMentions(text);
      if (mentions.length === 0) return;
      const removed = removeMention(text, mentions[0]!);
      expect(removed.length).toBeLessThanOrEqual(text.length);
      expect(draftMentions(removed).length).toBeLessThan(mentions.length);
    });
  });

  it("only reports an active mention that starts at an @ in the draft", () => {
    forEachSeed(600, (a) => {
      const text = corpus(a);
      const caret = a.int(text.length + 1);
      const mention = activeMention(text, caret);
      if (!mention) return;
      expect(text[mention.start]).toBe("@");
      expect(mention.end).toBe(caret);
      expect(text.slice(mention.start + 1, mention.end)).toBe(mention.query);
    });
  });
});
