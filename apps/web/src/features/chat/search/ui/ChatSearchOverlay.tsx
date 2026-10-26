import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useEffect, useEffectEvent, useRef, useState, type KeyboardEvent } from "react";
import { IconButton, isImeKey, TextField, useContentVisible, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { useAppearanceProjection } from "@/lib/appearance";
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
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState<SearchState>(EMPTY_SEARCH);
  const barRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const focusReturn = useRef<HTMLElement | null>(null);

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
  });

  useEffect(() => {
    const root = owningRegion(barRef.current);
    if (!open || !search.query || !root) return;
    return observeMessageChanges(root, refreshMatches);
  }, [open, search.query]);

  const reset = () => {
    clearChatSearchHighlights();
    focusReturn.current = null;
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
    scrollRangeIntoView(found[0]?.range, appearance.read().motionScale);
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
    scrollRangeIntoView(matches[nextIndex]?.range, appearance.read().motionScale);
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
