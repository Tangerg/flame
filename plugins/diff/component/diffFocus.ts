import { useLayoutEffect, type RefObject } from "react";
import { diffFileRow } from "@flame/ui";
import { type Query, type Read, type Selection } from "./diffRead";

export type PendingFocus =
  | { target: "list"; active: Element | null; query?: Query }
  | { target: "file"; active: Element | null }
  | { target: "navigation"; active: HTMLButtonElement };

function restoreFileOverview(
  list: HTMLElement | null,
  fallback: HTMLElement | null,
  selectedPath: string | null,
) {
  const row =
    list && selectedPath !== null ? diffFileRow(list, selectedPath) : undefined;
  (row ?? fallback)?.focus();
}

// A connected origin distinguishes deliberate blur from its removal.
function focusMovedFromOrigin(
  active: Element | null,
  origin: Element | null,
  body: HTMLElement,
) {
  return active !== origin && (active !== body || origin?.isConnected === true);
}

interface DiffFocusTargets {
  pending: RefObject<PendingFocus | null>;
  last: RefObject<HTMLElement | null>;
  refreshButton: RefObject<HTMLButtonElement | null>;
  search: RefObject<HTMLInputElement | null>;
  heading: RefObject<HTMLHeadingElement | null>;
  list: RefObject<HTMLElement | null>;
  bodyViewport: RefObject<HTMLDivElement | null>;
  overviewScroll: RefObject<number>;
}

interface DiffFocusState {
  file: unknown;
  showingReader: boolean;
  accepted: Read["accepted"];
  hasFiles: boolean;
  selection: Selection;
  error: unknown;
}

// A deliberate focus move supersedes the pending restoration; disabling the
// activated navigation button only blurs it to the body.
function userMovedFocus(
  pending: PendingFocus,
  showingReader: boolean,
  active: Element | null,
): boolean {
  const navigation = pending.target === "navigation" ? pending.active : null;
  const lostNavigationFocus =
    showingReader && navigation?.disabled && active === document.body;
  return (
    !lostNavigationFocus &&
    focusMovedFromOrigin(active, pending.active, document.body)
  );
}

// Refresh keeps focus while the requested overview is pending, even when an
// earlier scoped result still provides visible rows.
function holdRefreshFocus(
  targets: DiffFocusTargets,
  pending: Extract<PendingFocus, { target: "list" }>,
  active: Element | null,
) {
  const refresh = targets.refreshButton.current;
  if (
    active === document.body &&
    pending.active?.isConnected === false &&
    refresh
  ) {
    refresh.focus();
    targets.pending.current = { ...pending, active: refresh };
  }
}

function completeMove(
  targets: DiffFocusTargets,
  state: DiffFocusState,
  pending: PendingFocus,
  fallback: HTMLElement | null,
) {
  targets.pending.current = null;
  if (pending.target !== "list" && state.showingReader) {
    const navigation = pending.target === "navigation" ? pending.active : null;
    if (!navigation?.isConnected || navigation.disabled)
      targets.heading.current?.focus();
    return;
  }
  if (targets.bodyViewport.current)
    targets.bodyViewport.current.scrollTop = targets.overviewScroll.current;
  restoreFileOverview(targets.list.current, fallback, state.selection.path);
}

// Switching between the overview and the reader replaces the element that held
// focus; this returns focus to the control that matches the requested move.
export function useDiffFocusRestoration(
  targets: DiffFocusTargets,
  state: DiffFocusState,
) {
  useLayoutEffect(() => {
    const pending = targets.pending.current;
    const active = document.activeElement;
    const fallback =
      state.accepted && !state.hasFiles
        ? targets.refreshButton.current
        : targets.search.current;
    const lostFocus = targets.last.current?.isConnected === false;
    if (lostFocus) targets.last.current = null;
    if (!pending) {
      if (!state.showingReader && active === document.body && lostFocus)
        fallback?.focus();
      return;
    }
    if (userMovedFocus(pending, state.showingReader, active)) {
      targets.pending.current = null;
      return;
    }
    if (
      pending.target === "list" &&
      pending.query &&
      state.accepted?.query !== pending.query
    ) {
      holdRefreshFocus(targets, pending, active);
      return;
    }
    completeMove(targets, state, pending, fallback);
    // The targets are stable refs; only the presented state reruns the restoration.
  }, [
    state.file,
    state.showingReader,
    state.accepted,
    state.hasFiles,
    state.selection,
    state.error,
  ]);
}
