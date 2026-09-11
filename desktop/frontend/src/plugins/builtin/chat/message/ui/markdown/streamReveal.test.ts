// Two halves, and the split is deliberate.
//
// `pickRate` is a LADDER — 40/80/160, drained at 8 per char between 80 and 280 — and those
// numbers are a perf decision that is allowed to move. What is pinned about it is its shape:
// monotone, and never slower once the stream has stopped.
//
// The hook around it was left untested on the grounds that it is "a moving target tied to perf
// characteristics". That is true of the rate and of nothing else. Reveal ONE character wrong and
// the transcript shows a replacement glyph; reveal past the end and it throws; hand back
// something that is not a prefix and the markdown parser sees a document nobody wrote. None of
// those depend on how fast it goes, so they are asserted here at whatever rate the ladder
// happens to be running.

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { publishMotionScale } from "@/lib/appearance";
import { segmentWords } from "@/lib/i18n/segmentWords";
import { pickRate, useCommitThrottle, useStreamReveal } from "./streamReveal";

describe("pickRate — streaming mode (3-tier ladder)", () => {
  it("returns RATE_CRUISE (40 c/s) for small backlogs", () => {
    expect(pickRate(0, true)).toBe(40);
    expect(pickRate(5, true)).toBe(40);
    expect(pickRate(19, true)).toBe(40);
  });

  it("returns RATE_MODERATE (80 c/s) for mid-sized backlogs", () => {
    expect(pickRate(20, true)).toBe(80);
    expect(pickRate(40, true)).toBe(80);
    expect(pickRate(59, true)).toBe(80);
  });

  it("returns RATE_CATCHUP (160 c/s) for large backlogs", () => {
    expect(pickRate(60, true)).toBe(160);
    expect(pickRate(100, true)).toBe(160);
    expect(pickRate(10_000, true)).toBe(160);
  });

  it("rate strictly increases (or stays equal) with backlog", () => {
    // Property: monotone non-decreasing. Catches any future tier-shuffle
    // regression that would let a bigger backlog drain slower.
    let prev = 0;
    for (let backlog = 0; backlog < 200; backlog += 5) {
      const rate = pickRate(backlog, true);
      expect(rate).toBeGreaterThanOrEqual(prev);
      prev = rate;
    }
  });
});

describe("pickRate — drain mode (streaming=false)", () => {
  it("scales with backlog at DRAIN_RATE_PER_CHAR (=8)", () => {
    // 30 chars × 8 = 240 — within [MIN=80, MAX=280] so no clamp.
    expect(pickRate(30, false)).toBe(240);
  });

  it("clamps to DRAIN_RATE_MIN (=80) for tiny backlogs", () => {
    // 0 chars × 8 = 0 → clamped up to 80.
    expect(pickRate(0, false)).toBe(80);
    // 5 chars × 8 = 40 → also below MIN.
    expect(pickRate(5, false)).toBe(80);
  });

  it("clamps to DRAIN_RATE_MAX (=280) for huge backlogs", () => {
    // 100 chars × 8 = 800 → clamped down to 280.
    expect(pickRate(100, false)).toBe(280);
    expect(pickRate(10_000, false)).toBe(280);
  });

  it("drain rate stays ≥ streaming cruise rate at any backlog", () => {
    // Property: drain mode is never SLOWER than the streaming cruise
    // tier — once the stream stops we always want to catch up at least
    // as quickly as we were going.
    for (let backlog = 0; backlog < 200; backlog += 5) {
      expect(pickRate(backlog, false)).toBeGreaterThanOrEqual(40);
    }
  });
});

// A reveal that never finishes is indistinguishable from one that never starts, so every
// behavioural case below drives real frames. rAF and `performance` are faked together because
// the loop reads both — with only rAF faked the hook sees every frame as zero-length and never
// accumulates a character.
const FAKE = ["requestAnimationFrame", "cancelAnimationFrame", "performance"] as const;

/** Runs frames until the reveal settles, collecting what was shown at each step. */
function play(text: string, streaming: boolean, mode: "smooth" | "typewriter" | "instant") {
  const seen: string[] = [];
  const { result, rerender, unmount } = renderHook(
    ({ value }: { value: string }) => useStreamReveal(value, streaming, mode),
    { initialProps: { value: text } },
  );
  seen.push(result.current);
  for (let frame = 0; frame < 400 && result.current !== text; frame += 1) {
    act(() => void vi.advanceTimersByTime(16));
    seen.push(result.current);
  }
  return { seen, final: result.current, rerender, unmount };
}

describe("useStreamReveal — what holds at any rate", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: [...FAKE] });
    publishMotionScale(1);
  });
  afterEach(() => {
    publishMotionScale(1);
    vi.useRealTimers();
  });

  const PROSE = "The boundary is clean. Runtime owns the state machine, and the shell renders it.";

  it("hands back a PREFIX of the source at every frame, and eventually all of it", () => {
    const { seen, final } = play(PROSE, true, "smooth");
    for (const shown of seen) expect(PROSE.startsWith(shown)).toBe(true);
    expect(final).toBe(PROSE);
    // Floor: a reveal that arrived whole on frame one would satisfy the line above and prove
    // nothing about the loop.
    expect(new Set(seen).size).toBeGreaterThan(3);
  });

  it("never stops on half of a surrogate pair", () => {
    // Every emoji here is two UTF-16 units. Cutting between them renders U+FFFD for a frame —
    // the defect the loop's `isHighSurrogate` step exists to prevent, and one that only shows
    // up on a frame nobody screenshots.
    const EMOJI = "ship 🚀 then 🎉 and 🛠️ done 😀 finally";
    const { seen, final } = play(EMOJI, true, "typewriter");
    const stranded = seen.filter((shown) => {
      if (shown.length === 0) return false;
      const last = shown.charCodeAt(shown.length - 1);
      return last >= 0xd800 && last <= 0xdbff;
    });
    expect(stranded, "frames ending on a lone high surrogate").toEqual([]);
    expect(final).toBe(EMOJI);
  });

  it("cuts smooth mode at word boundaries, and typewriter mode not only there", () => {
    const boundaries = new Set<number>([0]);
    let at = 0;
    for (const word of segmentWords(PROSE)) {
      at += word.length;
      boundaries.add(at);
    }

    const smooth = play(PROSE, true, "smooth");
    const offBoundary = smooth.seen.filter((shown) => !boundaries.has(shown.length));
    expect(offBoundary, "smooth mode cut inside a word").toEqual([]);

    // The counterpart, so "smooth is word-wise" is a measured difference rather than a property
    // both modes happen to share on this string.
    const typed = play(PROSE, true, "typewriter");
    expect(typed.seen.some((shown) => !boundaries.has(shown.length))).toBe(true);
  });

  it("reveals the whole text at once when the reader asked for no animation", () => {
    const { result } = renderHook(() => useStreamReveal(PROSE, true, "instant"));
    expect(result.current).toBe(PROSE);
  });

  it("reveals the whole text at once under reduced motion, whatever the mode says", () => {
    publishMotionScale(0);
    const { result } = renderHook(() => useStreamReveal(PROSE, true, "smooth"));
    expect(result.current).toBe(PROSE);
  });

  it("starts over from the shorter text when the source is replaced, not from where it was", () => {
    // Shrink, then grow again — a turn replaced by its edited self, or a compaction that
    // rewrites the tail and streams a new one.
    //
    // The shrink ALONE proves nothing, and this test asserted only that for one commit:
    // `slice` clamps, so a reveal position left pointing past the end still hands back exactly
    // the short string and looks perfect. Verified by deleting the clamp — 15 tests, all green.
    //
    // It costs something on the way back UP. A stale position means the next text opens with
    // that many characters already on screen, none of which were ever revealed, and the reader
    // sees a paragraph appear whole and then continue letter by letter.
    const { result, rerender } = renderHook(
      ({ value }: { value: string }) => useStreamReveal(value, true, "smooth"),
      { initialProps: { value: PROSE } },
    );
    for (let frame = 0; frame < 15; frame += 1) act(() => void vi.advanceTimersByTime(16));
    const reached = result.current.length;
    // Mid-reveal, not finished: the position has to be somewhere the clamp can matter.
    expect(reached, "the reveal has to be under way").toBeGreaterThan(10);
    expect(reached, "and not already finished").toBeLessThan(PROSE.length);

    const SHORT = "The boundary";
    expect(SHORT.length).toBeLessThan(reached);
    act(() => rerender({ value: SHORT }));
    expect(result.current).toBe(SHORT);

    act(() => rerender({ value: PROSE }));
    act(() => void vi.advanceTimersByTime(16));
    // One frame's worth past the short text, not a leap back to where the old one had got to.
    expect(
      result.current.length,
      "the replacement opened with text that was never revealed",
    ).toBeLessThan(reached);

    for (let frame = 0; frame < 200 && result.current !== PROSE; frame += 1) {
      act(() => void vi.advanceTimersByTime(16));
    }
    expect(result.current).toBe(PROSE);
  });

  it("still finishes once the stream has stopped", () => {
    // `streaming: false` is the drain — the same loop at the faster rate. A reveal that only
    // advanced while more was arriving would leave the last words of every answer off screen.
    const { final } = play(PROSE, false, "smooth");
    expect(final).toBe(PROSE);
  });
});

// The zero case is a short circuit with a consumer: `MarkdownMessage` reads `source === text` to
// decide what material is on screen, and that read sits downstream of this hook. What the short
// circuit buys is a render, not a reference — a string committed through state compares equal by
// value once the trailing timeout fires, so the only thing at stake is WHEN it becomes equal.
// Both halves are pinned here because a refactor that routes every value through state looks
// harmless from the identity side and silently costs a window on every mount.
describe("useCommitThrottle", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("hands back settled text on the same render", () => {
    const { result, rerender } = renderHook(
      ({ value }: { value: string }) => useCommitThrottle(value, 0),
      { initialProps: { value: "one" } },
    );

    expect(result.current).toBe("one");
    act(() => rerender({ value: "two" }));
    expect(result.current, "the zero case waited for a timeout").toBe("two");
  });

  it("holds a streaming value back, then catches up to it exactly", () => {
    const { result, rerender } = renderHook(
      ({ value }: { value: string }) => useCommitThrottle(value, 120),
      { initialProps: { value: "one" } },
    );

    act(() => rerender({ value: "two" }));
    // The whole point of the throttle: the parser is not handed every token.
    expect(result.current, "the throttle let a change straight through").toBe("one");

    act(() => void vi.advanceTimersByTime(200));
    expect(result.current).toBe("two");
  });
});
