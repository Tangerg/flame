// One rAF loop plus time-based char-debt accounting, so the reveal keeps a steady rate
// however irregularly the backend chunks arrive. `smooth` advances by whole words and
// breathes at sentence ends, giving the caller's per-word fade time to land; `typewriter`
// spends raw characters with no pauses.

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { segmentWords } from "@/lib/i18n/segmentWords";

// chars/sec, selected by backlog.
const RATE_CRUISE = 40;
const RATE_MODERATE = 80;
const RATE_CATCHUP = 160;

const SENTENCE_PAUSE_MS = 80;
const MAX_DEBT = 12;
const MAX_FRAME_STEP_MS = 64;

// Drain scales with backlog so a short tail does not feel slow while a missed paragraph
// still does not blast onto the screen at once.
const DRAIN_RATE_MIN = 80;
const DRAIN_RATE_MAX = 280;
const DRAIN_RATE_PER_CHAR = 8;

const SENTENCE_END_RE = /[。！？…!?.]$/;

// Progressive reveal is JS-driven, so the blanket `prefers-reduced-motion` rule in
// globals.css cannot reach it — it only tones down CSS durations. Checks BOTH the OS media
// query and the in-app "Motion: Off" setting so the two cannot disagree.
function prefersReducedMotion(): boolean {
  if (typeof document !== "undefined") {
    if (document.documentElement.getAttribute("data-motion") === "off") return true;
  }
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

// Exported only so tests can pin the rate selection.
export function pickRate(backlog: number, streaming: boolean): number {
  if (!streaming) {
    return Math.min(DRAIN_RATE_MAX, Math.max(DRAIN_RATE_MIN, backlog * DRAIN_RATE_PER_CHAR));
  }
  if (backlog >= 60) return RATE_CATCHUP;
  if (backlog >= 20) return RATE_MODERATE;
  return RATE_CRUISE;
}

/** `enabled` controls both the initial state (false → start fully revealed) and the live
 *  rate (false → drain mode). The rAF loop stays mounted so stream-end drains gracefully. */
export function useStreamReveal(rawText: string, enabled: boolean, typewriter = false): string {
  const reduce = prefersReducedMotion();
  const active = enabled && !reduce;

  const initialLen = active ? 0 : rawText.length;
  const [displayLen, setDisplayLen] = useState(initialLen);

  // In refs so the rAF tick reads the freshest values without re-subscribing the effect.
  const stateRef = useRef({
    rawText: "",
    words: [] as string[],
    displayLen: initialLen,
    lastTickAt: -1,
    charDebt: 0,
    pauseUntil: 0,
  });

  // Layout-phase so the new render publishes before the next browser frame can advance the
  // old material.
  const enabledRef = useRef(active);
  const typewriterRef = useRef(typewriter);
  useLayoutEffect(() => {
    enabledRef.current = active;
    typewriterRef.current = typewriter;
    const state = stateRef.current;
    if (state.rawText !== rawText) {
      state.rawText = rawText;
      state.words = segmentWords(rawText);
      if (state.displayLen > rawText.length) {
        state.displayLen = rawText.length;
      }
    }
  }, [active, rawText, typewriter]);

  // 0 = parked. The loop PARKS at zero backlog instead of self-rescheduling forever: every
  // mounted message owns one of these hooks, so a long session would otherwise keep
  // hundreds of 60fps callbacks spinning while idle.
  const rafRef = useRef(0);
  const armRef = useRef(() => {});

  useEffect(() => {
    const tick = () => {
      rafRef.current = 0;
      const st = stateRef.current;
      const backlog = st.rawText.length - st.displayLen;

      if (backlog <= 0) {
        // Reset rate state on park, or a stale `lastTickAt` gives the next growth event a
        // giant elapsed and dumps the whole debt in one frame.
        st.lastTickAt = -1;
        st.charDebt = 0;
        return;
      }

      const now = performance.now();
      if (now < st.pauseUntil) {
        rafRef.current = requestAnimationFrame(tick);
        return;
      }

      if (st.lastTickAt < 0) {
        // Cold start: seed one unit so the first frame is not empty — there is no elapsed
        // time to integrate yet.
        st.lastTickAt = now;
        st.charDebt = 1;
      } else {
        const elapsed = Math.min(now - st.lastTickAt, MAX_FRAME_STEP_MS);
        st.lastTickAt = now;
        st.charDebt += pickRate(backlog, enabledRef.current) * (elapsed / 1000);
      }

      let newLen: number;
      let lastWord = "";
      if (typewriterRef.current) {
        const reveal = Math.floor(st.charDebt);
        st.charDebt -= reveal;
        newLen = st.displayLen + reveal;
      } else {
        // Walk to the current boundary, then reveal whole words while the debt (paid in
        // characters) covers them.
        const words = st.words;
        let charCount = 0;
        let wordIdx = 0;
        while (wordIdx < words.length && charCount < st.displayLen) {
          charCount += words[wordIdx]!.length;
          wordIdx++;
        }
        while (st.charDebt >= 1 && wordIdx < words.length) {
          const w = words[wordIdx]!;
          charCount += w.length;
          st.charDebt -= w.length;
          wordIdx++;
        }
        lastWord = wordIdx > 0 ? words[wordIdx - 1]! : "";
        newLen = charCount;
      }
      st.charDebt = Math.max(0, Math.min(st.charDebt, MAX_DEBT));

      newLen = Math.min(newLen, st.rawText.length);
      // Typewriter mode spends raw UTF-16 units, so a mid-emoji boundary would leave a lone
      // high surrogate and render "�" for a frame.
      if (newLen > st.displayLen && newLen < st.rawText.length) {
        const code = st.rawText.charCodeAt(newLen - 1);
        if (code >= 0xd800 && code <= 0xdbff) newLen += 1;
      }
      if (newLen !== st.displayLen) {
        st.displayLen = newLen;
        setDisplayLen(newLen);
      }

      // Smooth mode only (`lastWord` is "" in typewriter), and only while still streaming.
      if (lastWord && enabledRef.current && SENTENCE_END_RE.test(lastWord.trimEnd())) {
        st.pauseUntil = now + SENTENCE_PAUSE_MS;
      }

      rafRef.current = requestAnimationFrame(tick);
    };

    armRef.current = () => {
      if (rafRef.current === 0) rafRef.current = requestAnimationFrame(tick);
    };
    armRef.current();

    return () => {
      if (rafRef.current !== 0) cancelAnimationFrame(rafRef.current);
      rafRef.current = 0;
    };
  }, []);

  useEffect(() => {
    if (reduce) return;
    armRef.current();
  }, [rawText, reduce]);

  // Bypasses `displayLen`, which is pinned at mount and never advances because the loop
  // never arms under reduced motion.
  return reduce ? rawText : rawText.slice(0, displayLen);
}

/**
 * Coalesces `useStreamReveal`'s ~60×/s output to one commit per `minMs`, so a burst of tiny
 * tokens cannot re-parse the whole block tree every frame. The trailing edge ALWAYS flushes
 * the latest value, so settled text is never left as a stale slice.
 */
export function useCommitThrottle(value: string, minMs: number): string {
  const [committed, setCommitted] = useState(value);
  const lastCommitRef = useRef(0);

  useEffect(() => {
    if (minMs <= 0) return;
    const elapsed = performance.now() - lastCommitRef.current;
    const delay = Math.max(0, minMs - elapsed);
    const id = setTimeout(() => {
      lastCommitRef.current = performance.now();
      setCommitted(value);
    }, delay);
    return () => clearTimeout(id);
  }, [value, minMs]);

  return minMs <= 0 ? value : committed;
}
