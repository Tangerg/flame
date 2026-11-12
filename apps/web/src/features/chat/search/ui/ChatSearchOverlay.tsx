import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useEffect, useEffectEvent, useRef, useState, type KeyboardEvent } from "react";
import { IconButton, isImeKey, TextField, useContentVisible, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { useAppearanceProjection } from "@/lib/appearance";
import { useStreamFollow, type StreamFollow } from "@/ui/agent/stream-follow";
import { useActiveSessionId } from "@/features/agent/public/session";
import { readingColumn as rc } from "@/features/chat/public/message/rendering";
import {
  clearChatSearchHighlights,
  paintChatSearchHighlights,
} from "@/features/chat/search/adapters/searchHighlights";
import { setChatSearchOpener } from "@/features/chat/search/application/openChatSearch";
import {
  findMessageMatches,
  observeMessageChanges,
  type MessageMatch,
} from "@/features/chat/search/adapters/messageRanges";
import { face, space, type as typeStep } from "@flame/ui/tokens.stylex";
const cs = stylex.create({
  bar: {
    display: "flex",
    alignItems: "center",
    gap: space.s1,
    minWidth: 0,
    flexShrink: 0,
    paddingBlock: space.s1_5,
  },
  field: { flex: 1, minWidth: 0 },
  count: { flexShrink: 0, paddingInline: space.s1_5, whiteSpace: "nowrap" },
});

export function ChatSearchOverlay() {
  const activeSessionId = useActiveSessionId();
  const visible = useContentVisible();

  return visible ? <SessionChatSearchOverlay key={activeSessionId || "no-session"} /> : null;
}

type SearchState = {
  query: string;
  matches: MessageMatch[];
  activeIndex: number;
};

const EMPTY_SEARCH: SearchState = {
  query: "",
  matches: [],
  activeIndex: 0,
};

function SessionChatSearchOverlay() {
  const appearance = useAppearanceProjection();
  const follow = useStreamFollow();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState<SearchState>(EMPTY_SEARCH);
  const barRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const focusReturn = useRef<HTMLElement | null>(null);
  const readingWitness = useRef({ visible: false, frame: [] as number[] });

  const requestOpen = useEffectEvent(() => {
    // Repeated Find from inside the bar keeps the original opener.
    if (!open || !barRef.current?.contains(document.activeElement)) {
      focusReturn.current = captureFocusReturn();
    }
    if (!open) {
      setOpen(true);
      return;
    }
    inputRef.current?.focus();
    inputRef.current?.select();
  });

  useEffect(() => {
    setChatSearchOpener(() => requestOpen());
    return () => setChatSearchOpener(null);
  }, []);

  useEffect(() => () => clearChatSearchHighlights(), []);

  useEffect(() => {
    if (!open) return;
    inputRef.current?.focus();
    inputRef.current?.select();
  }, [open]);

  const refreshMatches = useEffectEvent(() => {
    const root = owningRegion(barRef.current);
    if (!open || !search.query || !root) return;
    const matches = findMessageMatches(search.query, root);
    // Live ranges collapse when rendered text is replaced; retain the occurrence in its message.
    const current = search.matches[search.activeIndex];
    const retained = current
      ? matches.findIndex(
          (match) => match.message === current.message && match.occurrence === current.occurrence,
        )
      : -1;
    const activeIndex =
      retained >= 0 ? retained : Math.max(0, Math.min(search.activeIndex, matches.length - 1));
    setSearch({ ...search, matches, activeIndex });
    paintChatSearchHighlights(
      matches.map((match) => match.range),
      activeIndex,
    );
    const range = matches[activeIndex]?.range;
    const frame = range ? readingFrame(range, root) : [];
    // Host geometry publication also refreshes ranges; keep a pending reflow's visible witness.
    if (retained < 0 || sameFrame(readingWitness.current.frame, frame)) {
      readingWitness.current = {
        visible:
          !!range && !!barRef.current?.contains(document.activeElement) && isReadable(range, root),
        frame,
      };
    }
  });

  useEffect(() => {
    const root = owningRegion(barRef.current);
    if (!open || !search.query || !root) return;
    return observeMessageChanges(root, refreshMatches);
  }, [open, search.query]);

  const currentRange = search.matches[search.activeIndex]?.range;
  const protectReading = useEffectEvent((range: Range) => stopFollowHidingMatch(range, follow));

  useEffect(() => {
    const bar = barRef.current;
    const region = owningRegion(bar);
    const viewport = currentRange?.startContainer.parentElement?.closest(".msg-scroll-viewport");
    if (!open || !currentRange || !bar || !region || !viewport || !region.contains(viewport))
      return;
    const document = bar.ownerDocument;
    let live = true;
    const onResize = () => {
      if (!live) return;
      const frame = readingFrame(currentRange, region);
      if (!bar.contains(document.activeElement)) readingWitness.current.visible = false;
      // Content shrink can rearm follow without changing the reading frame.
      if (readingWitness.current.visible) protectReading(currentRange);
      if (sameFrame(readingWitness.current.frame, frame)) return;
      if (readingWitness.current.visible) {
        retainInFrames(currentRange, region);
        protectReading(currentRange);
      }
      readingWitness.current.frame = readingFrame(currentRange, region);
    };
    const onScroll = (event: Event) => {
      // Native reflow can deliver a clamped scroll before ResizeObserver; that is not reading.
      if (!live || !sameFrame(readingWitness.current.frame, readingFrame(currentRange, region)))
        return;
      if (event.target instanceof Node && event.target.contains(currentRange.startContainer)) {
        readingWitness.current.visible =
          bar.contains(document.activeElement) && isReadable(currentRange, region);
      }
    };
    const onFocus = () => {
      if (live)
        readingWitness.current.visible =
          bar.contains(document.activeElement) && isReadable(currentRange, region);
    };
    const observer = new ResizeObserver(onResize);
    for (const element of new Set([
      viewport,
      viewport.firstElementChild,
      ...clippingAncestors(currentRange, region),
    ])) {
      if (element) observer.observe(element);
    }
    region.addEventListener("scroll", onScroll, { capture: true, passive: true });
    document.addEventListener("focusin", onFocus);
    return () => {
      live = false;
      observer.disconnect();
      region.removeEventListener("scroll", onScroll, true);
      document.removeEventListener("focusin", onFocus);
    };
  }, [open, currentRange]);

  const reset = () => {
    clearChatSearchHighlights();
    focusReturn.current = null;
    readingWitness.current = { visible: false, frame: [] };
    setSearch(EMPTY_SEARCH);
    setOpen(false);
  };

  const close = () => {
    const bar = barRef.current;
    const active = document.activeElement;
    const ownsFocus = !active || active === document.body || !!bar?.contains(active);
    const target = focusReturn.current;
    const region = owningRegion(bar);
    reset();
    if (ownsFocus) restoreFocus(target, region);
  };

  const changeQuery = (query: string) => {
    const root = owningRegion(barRef.current);
    const found = root ? findMessageMatches(query, root) : [];
    setSearch({ query, matches: found, activeIndex: 0 });
    paintChatSearchHighlights(
      found.map((match) => match.range),
      0,
    );
    const range = found[0]?.range;
    stopFollowHidingMatch(range, follow);
    scrollRangeIntoView(range, appearance.read().motionScale);
    readingWitness.current = {
      visible: !!range && !!root && isReadable(range, root),
      frame: range && root ? readingFrame(range, root) : [],
    };
  };

  const move = (delta: number) => {
    const { matches, activeIndex } = search;
    if (matches.length === 0) return;
    const nextIndex = (activeIndex + delta + matches.length) % matches.length;
    setSearch({ ...search, activeIndex: nextIndex });
    paintChatSearchHighlights(
      matches.map((match) => match.range),
      nextIndex,
    );
    const range = matches[nextIndex]?.range;
    stopFollowHidingMatch(range, follow);
    scrollRangeIntoView(range, appearance.read().motionScale);
    const region = owningRegion(barRef.current);
    readingWitness.current = {
      visible: !!range && !!region && isReadable(range, region),
      frame: range && region ? readingFrame(range, region) : [],
    };
  };

  const onKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.defaultPrevented || isImeKey(event.nativeEvent)) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close();
    } else if (event.key === "Enter" && event.target === inputRef.current) {
      event.preventDefault();
      move(event.shiftKey ? -1 : 1);
    }
  };

  const t = useT();
  if (!open) return null;

  const { query, matches, activeIndex } = search;
  const total = matches.length;

  return (
    <div
      ref={barRef}
      role="search"
      aria-label={t("chatSearch.label")}
      onKeyDownCapture={onKeyDown}
      {...stylex.props(rc.box, rc.gutter, cs.bar)}
    >
      <TextField
        ref={inputRef}
        size="sm"
        aria-label={t("chatSearch.label")}
        value={query}
        onChange={(event) => changeQuery(event.target.value)}
        placeholder={t("chatSearch.placeholder")}
        styles={cs.field}
      />
      <span role="status" {...stylex.props(cs.count, vocab.faint, typeStep.uiSm, face.mono)}>
        {total > 0 ? `${activeIndex + 1} / ${total}` : query ? "0 / 0" : ""}
      </span>
      <IconButton
        icon={iconGlyph("chevron-up")}
        size="xs"
        title={`${t("chatSearch.prev")} (⇧⏎)`}
        aria-label={t("chatSearch.prev")}
        disabled={total === 0}
        focusableWhenDisabled
        onClick={() => move(-1)}
      />
      <IconButton
        icon={iconGlyph("chevron-down")}
        size="xs"
        title={`${t("chatSearch.next")} (⏎)`}
        aria-label={t("chatSearch.next")}
        disabled={total === 0}
        focusableWhenDisabled
        onClick={() => move(1)}
      />
      <IconButton
        icon={iconGlyph("x")}
        size="xs"
        title={`${t("common.close")} (Esc)`}
        aria-label={t("common.close")}
        onClick={close}
      />
    </div>
  );
}

function owningRegion(bar: HTMLElement | null): HTMLElement | null {
  return bar?.closest<HTMLElement>('[role="region"]') ?? null;
}

function captureFocusReturn(): HTMLElement | null {
  const element = document.activeElement;
  return element instanceof HTMLElement && element !== document.body ? element : null;
}

function restoreFocus(target: HTMLElement | null, region: HTMLElement | null): void {
  if (target?.isConnected && !target.closest("[hidden], [inert]")) {
    target.focus({ preventScroll: true });
    // A focus call on unrendered material is a no-op; only a landed focus counts.
    if (document.activeElement === target) return;
  }
  region?.focus({ preventScroll: true });
}

function scrollRangeIntoView(range: Range | undefined, motionScale: number): void {
  if (!range) return;
  const reduced =
    motionScale === 0 || window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  range.startContainer.parentElement?.scrollIntoView({
    block: "center",
    behavior: reduced ? "instant" : "smooth",
  });
}

type Edges = { left: number; right: number; top: number; bottom: number };

const CLIPPING_OVERFLOW = new Set(["auto", "scroll", "hidden", "clip"]);

function clippingAncestors(range: Range, region: HTMLElement): HTMLElement[] {
  const clips: HTMLElement[] = [];
  for (let at = range.startContainer.parentElement; at && at !== region; at = at.parentElement) {
    const style = getComputedStyle(at);
    if (CLIPPING_OVERFLOW.has(style.overflowX) || CLIPPING_OVERFLOW.has(style.overflowY)) {
      clips.push(at);
    }
  }
  return clips;
}

function clipEdges(element: HTMLElement): Edges {
  const box = element.getBoundingClientRect();
  const style = getComputedStyle(element);
  const left = box.left + element.clientLeft;
  const top = box.top + element.clientTop;
  return {
    left: left + (parseFloat(style.scrollPaddingLeft) || 0),
    right: left + element.clientWidth - (parseFloat(style.scrollPaddingRight) || 0),
    top: top + (parseFloat(style.scrollPaddingTop) || 0),
    bottom: top + element.clientHeight - (parseFloat(style.scrollPaddingBottom) || 0),
  };
}

function rangeEdges(range: Range): Edges | null {
  const boxes = [...range.getClientRects()].filter((box) => box.width > 0 && box.height > 0);
  if (boxes.length === 0) return null;
  return {
    left: Math.min(...boxes.map((box) => box.left)),
    right: Math.max(...boxes.map((box) => box.right)),
    top: Math.min(...boxes.map((box) => box.top)),
    bottom: Math.max(...boxes.map((box) => box.bottom)),
  };
}

function stopFollowHidingMatch(range: Range | undefined, follow: StreamFollow | null): void {
  const viewport = follow?.scrollRef.current;
  if (!range || !follow?.state.isAtBottom || !viewport?.contains(range.startContainer)) return;
  const movement = follow.state.targetScrollTop - viewport.scrollTop;
  if (movement <= 0) return;
  const word = rangeEdges(range);
  if (!word) return;
  const clip = clipEdges(viewport);
  if (word.top - movement < clip.top || word.bottom - movement > clip.bottom) follow.stopScroll();
}

function isReadable(range: Range, region: HTMLElement): boolean {
  const word = rangeEdges(range);
  return (
    !!word &&
    clippingAncestors(range, region).every((element) => {
      const clip = clipEdges(element);
      return (
        word.left >= clip.left - 0.5 &&
        word.right <= clip.right + 0.5 &&
        word.top >= clip.top - 0.5 &&
        word.bottom <= clip.bottom + 0.5
      );
    })
  );
}

function readingFrame(range: Range, region: HTMLElement): number[] {
  return clippingAncestors(range, region).flatMap((element) => {
    const box = element.getBoundingClientRect();
    const clip = clipEdges(element);
    // Relative clip edges stay unchanged when a reader deliberately scrolls its ancestors.
    return [
      box.width,
      box.height,
      clip.left - box.left,
      clip.right - box.left,
      clip.top - box.top,
      clip.bottom - box.top,
    ];
  });
}

function sameFrame(previous: number[], next: number[]): boolean {
  return previous.length === next.length && previous.every((value, index) => value === next[index]);
}

function edgeDelta(start: number, end: number, min: number, max: number): number {
  if (start >= min - 0.5 && end <= max + 0.5) return 0;
  return start < min || end - start > max - min ? Math.floor(start - min) : Math.ceil(end - max);
}

function retainInFrames(range: Range, region: HTMLElement): void {
  // Innermost first, so each outer scroll carries an already-placed nested reader.
  for (const element of clippingAncestors(range, region)) {
    const word = rangeEdges(range);
    if (!word) return;
    const clip = clipEdges(element);
    const left = edgeDelta(word.left, word.right, clip.left, clip.right);
    const top = edgeDelta(word.top, word.bottom, clip.top, clip.bottom);
    if (left === 0 && top === 0) continue;
    element.scrollTo({
      left: element.scrollLeft + left,
      top: element.scrollTop + top,
      behavior: "instant",
    });
  }
}
