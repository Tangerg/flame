import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { motionScale } from "@/lib/appearance";
import { segmentWords } from "@/lib/i18n/segmentWords";
import type { StreamReveal } from "../../streamReveal";

/**
 * How text arrives on screen: the reader's stored preference, plus the one case the reader
 * never chose. `instant` is a RENDER decision — replayed history and anything already
 * complete has nothing to reveal — so it extends the preference rather than joining it.
 */
export type MarkdownReveal = StreamReveal | "instant";

// Characters per second, chosen by how far the reveal has fallen behind the stream.
const RATE_CRUISE = 40;
const RATE_MODERATE = 80;
const RATE_CATCHUP = 160;
const BACKLOG_MODERATE_CHARS = 20;
const BACKLOG_CATCHUP_CHARS = 60;

const SENTENCE_PAUSE_MS = 80;
const MAX_DEBT = 12;
const MAX_FRAME_STEP_MS = 64;

const DRAIN_RATE_MIN = 80;
const DRAIN_RATE_MAX = 280;
const DRAIN_RATE_PER_CHAR = 8;

const SENTENCE_END_RE = /[。！？…!?.]$/;

const HIGH_SURROGATE_FIRST = 0xd800;
const HIGH_SURROGATE_LAST = 0xdbff;

function isHighSurrogate(code: number): boolean {
  return code >= HIGH_SURROGATE_FIRST && code <= HIGH_SURROGATE_LAST;
}

// The painter publishes the scale AND writes `data-motion` for the stylesheet. Reading the
// published value keeps this on the one path; scraping the attribute made the same fact
// reachable two ways, and only one of them survives a rename.
function prefersReducedMotion(): boolean {
  if (motionScale() === 0) return true;
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

export function pickRate(backlog: number, streaming: boolean): number {
  if (!streaming) {
    return Math.min(DRAIN_RATE_MAX, Math.max(DRAIN_RATE_MIN, backlog * DRAIN_RATE_PER_CHAR));
  }
  if (backlog >= BACKLOG_CATCHUP_CHARS) return RATE_CATCHUP;
  if (backlog >= BACKLOG_MODERATE_CHARS) return RATE_MODERATE;
  return RATE_CRUISE;
}

export function useStreamReveal(
  rawText: string,
  streaming: boolean,
  reveal: MarkdownReveal,
): string {
  // The closed value travels the whole way rather than arriving as two booleans: the hook
  // owns what each mode means, and a caller cannot pick a combination that means nothing.
  const whole = reveal === "instant" || prefersReducedMotion();
  const active = streaming && !whole;

  const initialLen = active ? 0 : rawText.length;
  const [displayLen, setDisplayLen] = useState(initialLen);

  const stateRef = useRef({
    rawText: "",
    words: [] as string[],
    displayLen: initialLen,
    lastTickAt: null as number | null,
    charDebt: 0,
    pauseUntil: 0,
  });

  const enabledRef = useRef(active);
  const typewriterRef = useRef(reveal === "typewriter");
  useLayoutEffect(() => {
    enabledRef.current = active;
    typewriterRef.current = reveal === "typewriter";
    const state = stateRef.current;
    if (state.rawText !== rawText) {
      state.rawText = rawText;
      state.words = segmentWords(rawText);
      if (state.displayLen > rawText.length) {
        state.displayLen = rawText.length;
      }
    }
  }, [active, rawText, reveal]);

  const rafRef = useRef(0);
  const armRef = useRef(() => {});

  useEffect(() => {
    const tick = () => {
      rafRef.current = 0;
      const st = stateRef.current;
      const backlog = st.rawText.length - st.displayLen;

      if (backlog <= 0) {
        st.lastTickAt = null;
        st.charDebt = 0;
        return;
      }

      const now = performance.now();
      if (now < st.pauseUntil) {
        rafRef.current = requestAnimationFrame(tick);
        return;
      }

      if (st.lastTickAt === null) {
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
      if (newLen > st.displayLen && newLen < st.rawText.length) {
        // Cutting between a surrogate pair renders a replacement character for a frame.
        if (isHighSurrogate(st.rawText.charCodeAt(newLen - 1))) newLen += 1;
      }
      if (newLen !== st.displayLen) {
        st.displayLen = newLen;
        setDisplayLen(newLen);
      }

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
    if (whole) return;
    armRef.current();
  }, [rawText, whole]);

  return whole ? rawText : rawText.slice(0, displayLen);
}

/**
 * Trailing-throttles a streaming value, and at `minMs <= 0` returns the SAME value it was
 * given rather than a committed copy. That identity is load-bearing: `MarkdownMessage`
 * compares `source === text` to decide what material is visible, and a committed copy is
 * never `===`. It is also why the throttle here is not `useThrottledValue` from react-pacer,
 * which routes every value through state.
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
