import { describe, expect, it } from "vitest";
import { forEachSeed } from "@/test/arbitrary";
import {
  projectAskUserAnswer,
  projectConversationHits,
  projectFetchedPage,
  projectGlobPreview,
  projectHttpPreview,
  projectRecalledMemories,
  projectSkillPreview,
  projectToolSearchGroups,
  projectWebSearchPreview,
} from "./specialisedPreviewProjections";

// These read a tool's own answer, which for half of them is prose a model wrote and
// for the rest is JSON a tool emitted. Each is anchored on the one piece of
// structure the runtime actually produces and is meant to degrade to "no structure
// found" otherwise — so the property is that every one of them is total: a wording
// change on the backend costs a plain preview, never a thrown projection, which the
// reducer would swallow into a card that never appears.

const PROJECTIONS = [
  ["skill", projectSkillPreview],
  ["askUser", projectAskUserAnswer],
  ["glob", projectGlobPreview],
  ["webSearch", projectWebSearchPreview],
  ["recalledMemories", projectRecalledMemories],
  ["conversationHits", projectConversationHits],
  ["toolSearchGroups", projectToolSearchGroups],
  ["httpPreview", projectHttpPreview],
  ["fetchedPage", projectFetchedPage],
] as const;

function payloads(): string[] {
  return [
    "",
    " ",
    "null",
    "[]",
    "{}",
    "0",
    "false",
    '"a string"',
    "{unclosed",
    "[1, 2,",
    '{"schedules": null}',
    '{"schedules": {}}',
    '{"schedule": []}',
    '{"hits": "not an array"}',
    '{"status": "not a number"}',
    '{"content": 5}',
    '{"results": [null, 1, "x"]}',
    "1. \n2. \n",
    "Not loaded:\n  [] \n",
    "🙂",
    "\ud83d",
    "a".repeat(5000),
    "\n".repeat(500),
  ];
}

describe("every specialised tool preview projection", () => {
  it.each(PROJECTIONS.map(([name]) => name))("%s is total over an undefined result", (name) => {
    const project = PROJECTIONS.find(([candidate]) => candidate === name)![1];
    expect(() => project(undefined)).not.toThrow();
  });

  it.each(PROJECTIONS.map(([name]) => name))("%s is total over a malformed result", (name) => {
    const project = PROJECTIONS.find(([candidate]) => candidate === name)![1];
    const thrown: string[] = [];
    for (const payload of payloads()) {
      try {
        project(payload);
      } catch (error) {
        thrown.push(`${JSON.stringify(payload).slice(0, 40)}: ${String(error)}`);
      }
    }
    expect(thrown).toEqual([]);
  });

  it.each(PROJECTIONS.map(([name]) => name))("%s is total over arbitrary prose", (name) => {
    const project = PROJECTIONS.find(([candidate]) => candidate === name)![1];
    const thrown: string[] = [];
    forEachSeed(300, (a) => {
      try {
        project(a.text());
      } catch (error) {
        thrown.push(String(error));
      }
    });
    expect(thrown).toEqual([]);
  });

  it("answers a list-shaped projection with an array, whatever it was handed", () => {
    const listy = [
      projectSkillPreview,
      projectWebSearchPreview,
      projectRecalledMemories,
      projectConversationHits,
      projectToolSearchGroups,
    ];
    forEachSeed(200, (a) => {
      for (const project of listy) {
        expect(Array.isArray(project(a.text()))).toBe(true);
        expect(Array.isArray(project(undefined))).toBe(true);
      }
    });
  });

  it("answers a string-shaped projection with a string", () => {
    forEachSeed(200, (a) => {
      for (const project of [projectAskUserAnswer]) {
        expect(typeof project(a.text())).toBe("string");
        expect(typeof project(undefined)).toBe("string");
      }
    });
  });
});
