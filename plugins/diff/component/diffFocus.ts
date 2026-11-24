import { useLayoutEffect, type RefObject } from "react";
import { diffFileRow } from "@flame/ui";
import { type Query, type Read } from "./diffRead";

export type Selection =
  | { view: "files"; path: string | null }
  | { view: "file"; path: string; returnTo: "files" | "source" };

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

// Switching between the overview and the reader replaces the element that held
// focus; this returns focus to the control that matches the requested move.
export function useDiffFocusRestoration(
  targets: DiffFocusTargets,
  state: {
    file: unknown;
    showingReader: boolean;
    accepted: Read["accepted"];
    hasFiles: boolean;
    selection: Selection;
    error: unknown;
  },
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
    const navigation = pending.target === "navigation" ? pending.active : null;
    // Disabling the activated navigation button can blur it to the body.
    const lostNavigationFocus =
      state.showingReader && navigation?.disabled && active === document.body;
    if (
      !lostNavigationFocus &&
      focusMovedFromOrigin(active, pending.active, document.body)
    ) {
      targets.pending.current = null;
      return;
    }
    if (
      pending.target === "list" &&
      pending.query &&
      state.accepted?.query !== pending.query
    ) {
      // Refresh keeps focus while the requested overview is pending, even
      // when an earlier scoped result still provides visible rows.
      if (
        active === document.body &&
        pending.active?.isConnected === false &&
        targets.refreshButton.current
      ) {
        targets.refreshButton.current.focus();
        targets.pending.current = {
          ...pending,
          active: targets.refreshButton.current,
        };
      }
      return;
    }
    targets.pending.current = null;
    if (pending.target !== "list" && state.showingReader) {
      if (!navigation?.isConnected || navigation.disabled) {
        targets.heading.current?.focus();
      }
    } else {
      if (targets.bodyViewport.current)
        targets.bodyViewport.current.scrollTop = targets.overviewScroll.current;
      restoreFileOverview(targets.list.current, fallback, state.selection.path);
    }
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
