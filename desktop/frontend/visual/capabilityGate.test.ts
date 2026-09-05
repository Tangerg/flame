import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { VISUAL_RUNTIME_FEATURES } from "./agentFixtureFacts";

const SRC = join(import.meta.dirname, "..", "src");

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sources(path);
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

/**
 * A capability the fixture leaves out is not neutral. The surface gated on it renders its
 * off-ramp instead, and the goldens then hold a photograph of the app refusing to work —
 * which looks exactly like a photograph of the app, so nothing catches it.
 *
 * Reading the required list off the app's own gate sites rather than restating it means a
 * new gate arrives here as a failure, not as a surface that silently never renders.
 */
describe("the visual fixtures' Runtime", () => {
  it("advertises every capability the app gates a surface on", () => {
    const gated = new Set<string>();
    for (const file of sources(SRC)) {
      for (const match of readFileSync(file, "utf8").matchAll(
        /useRuntimeCapability\(\s*"([a-zA-Z]+)"\s*\)/g,
      )) {
        gated.add(match[1]!);
      }
    }

    // Not vacuous: the scan finding nothing would pass this test while proving nothing.
    expect(gated.size).toBeGreaterThanOrEqual(3);
    expect([...gated].filter((name) => !VISUAL_RUNTIME_FEATURES.includes(name as never))).toEqual(
      [],
    );
  });
});
