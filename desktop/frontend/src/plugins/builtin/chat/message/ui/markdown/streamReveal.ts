import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { segmentWords } from "@/lib/i18n/segmentWords";

const RATE_CRUISE = 40;
const RATE_MODERATE = 80;
const RATE_CATCHUP = 160;

const SENTENCE_PAUSE_MS = 80;
const MAX_DEBT = 12;
const MAX_FRAME_STEP_MS = 64;

const DRAIN_RATE_MIN = 80;
const DRAIN_RATE_MAX = 280;
const DRAIN_RATE_PER_CHAR = 8;

const SENTENCE_END_RE = /[。！？…!?.]$/;

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

export function pickRate(backlog: number, streaming: boolean): number {
  if (!streaming) {
    return Math.min(DRAIN_RATE_MAX, Math.max(DRAIN_RATE_MIN, backlog * DRAIN_RATE_PER_CHAR));
  }
  if (backlog >= 60) return RATE_CATCHUP;
  if (backlog >= 20) return RATE_MODERATE;
  return RATE_CRUISE;
}

export function useStreamReveal(rawText: string, enabled: boolean, typewriter = false): string {
  const reduce = prefersReducedMotion();
  const active = enabled && !reduce;

  const initialLen = active ? 0 : rawText.length;
  const [displayLen, setDisplayLen] = useState(initialLen);

  const stateRef = useRef({
    rawText: "",
    words: [] as string[],
    displayLen: initialLen,
    lastTickAt: -1,
    charDebt: 0,
    pauseUntil: 0,
  });

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

  const rafRef = useRef(0);
  const armRef = useRef(() => {});

  useEffect(() => {
    const tick = () => {
      rafRef.current = 0;
      const st = stateRef.current;
      const backlog = st.rawText.length - st.displayLen;

      if (backlog <= 0) {
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
        const code = st.rawText.charCodeAt(newLen - 1);
        if (code >= 0xd800 && code <= 0xdbff) newLen += 1;
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
    if (reduce) return;
    armRef.current();
  }, [rawText, reduce]);

  return reduce ? rawText : rawText.slice(0, displayLen);
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
