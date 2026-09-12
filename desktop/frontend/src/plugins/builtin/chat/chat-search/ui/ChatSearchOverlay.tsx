import * as stylex from "@stylexjs/stylex";
import { useEffect, useRef, useState } from "react";
import { IconButton, TextField, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import {
  clearChatSearchHighlights,
  installChatSearchHighlightStyles,
  paintChatSearchHighlights,
} from "../adapters/searchHighlights";
import { setChatSearchOpener } from "../application/openChatSearch";
import { findMessageRanges } from "../adapters/messageRanges";
import { face, radius, space, surface, type as typeStep } from "@/styles/tokens.stylex";

const cs = stylex.create({
  field: { height: space.s7, width: "calc(var(--spacing) * 56)", paddingInline: space.s2 },
  count: { paddingInline: space.s1_5 },
  pill: {
    position: "fixed",
    top: space.s3,
    right: space.s4,
    zIndex: "var(--layer-floating)",
    display: "inline-flex",
    alignItems: "center",
    gap: space.s1,
    borderRadius: radius.lg,
    backgroundColor: surface.card,
    paddingInline: space.s2,
    paddingBlock: space.s1_5,
    boxShadow: "var(--shadow-overlay)",
  },
  // It sits in the window's drag region, so it has to opt out or the pill cannot be clicked —
  // dragging the window would win. Two names for one thing: Wails reads its own property and
  // the platform reads the standard one.
  undraggable: { "-webkit-app-region": "no-drag", "--wails-draggable": "no-drag" },
});

export function ChatSearchOverlay() {
  const activeSessionId = useActiveSessionId();

  return <SessionChatSearchOverlay key={activeSessionId || "no-session"} />;
}

type SearchState = {
  query: string;
  matches: Range[];
  activeIndex: number;
};

const EMPTY_SEARCH: SearchState = {
  query: "",
  matches: [],
  activeIndex: 0,
};

function SessionChatSearchOverlay() {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState<SearchState>(EMPTY_SEARCH);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setChatSearchOpener(() => setOpen(true));
    return () => setChatSearchOpener(null);
  }, []);

  useEffect(() => {
    const uninstallStyles = installChatSearchHighlightStyles();
    return () => {
      clearChatSearchHighlights();
      uninstallStyles();
    };
  }, []);

  useEffect(() => {
    if (!open) return;
    inputRef.current?.focus();
    inputRef.current?.select();
  }, [open]);

  const close = () => {
    clearChatSearchHighlights();
    setSearch(EMPTY_SEARCH);
    setOpen(false);
  };

  const changeQuery = (query: string) => {
    const found = findMessageRanges(query);
    setSearch({ query, matches: found, activeIndex: 0 });
    paintChatSearchHighlights(found, 0);
    scrollRangeIntoView(found[0]);
  };

  const move = (delta: number) => {
    const { matches, activeIndex } = search;
    if (matches.length === 0) return;
    const nextIndex = (activeIndex + delta + matches.length) % matches.length;
    setSearch({ ...search, activeIndex: nextIndex });
    paintChatSearchHighlights(matches, nextIndex);
    scrollRangeIntoView(matches[nextIndex]);
  };

  const t = useT();
  if (!open) return null;

  const { query, matches, activeIndex } = search;
  const total = matches.length;

  return (
    <div role="search" {...stylex.props(cs.pill, cs.undraggable)}>
      <TextField
        ref={inputRef}
        variant="bare"
        font="sans"
        size="lg"
        aria-label={t("chatSearch.label")}
        value={query}
        onChange={(event) => changeQuery(event.target.value)}
        placeholder={t("chatSearch.placeholder")}
        className={stylex.props(cs.field).className}
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing) return;
          if (event.key === "Escape") {
            event.preventDefault();
            close();
          } else if (event.key === "Enter") {
            event.preventDefault();
            move(event.shiftKey ? -1 : 1);
          }
        }}
      />
      <span {...stylex.props(cs.count, vocab.faint, typeStep.uiSm, face.mono)}>
        {total > 0 ? `${activeIndex + 1} / ${total}` : query ? "0 / 0" : ""}
      </span>
      <IconButton
        icon="chevron-up"
        size="xs"
        title={`${t("chatSearch.prev")} (⇧⏎)`}
        aria-label={t("chatSearch.prev")}
        disabled={total === 0}
        onClick={() => move(-1)}
      />
      <IconButton
        icon="chevron-down"
        size="xs"
        title={`${t("chatSearch.next")} (⏎)`}
        aria-label={t("chatSearch.next")}
        disabled={total === 0}
        onClick={() => move(1)}
      />
      <IconButton
        icon="x"
        size="xs"
        title={`${t("common.close")} (Esc)`}
        aria-label={t("common.close")}
        onClick={close}
      />
    </div>
  );
}

function scrollRangeIntoView(range: Range | undefined): void {
  range?.startContainer.parentElement?.scrollIntoView({
    block: "center",
    behavior: "smooth",
  });
}
