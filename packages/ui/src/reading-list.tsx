import {
  useCallback,
  useEffect,
  useEffectEvent,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
} from "react";
import { LegendList, type LegendListRef } from "@legendapp/list/react";
import { useContentVisible } from "./content-visibility";
import { isImeKey } from "./keyboard";

const WEBKIT_LINE_SCROLL = 40;

export type ReadingListHandle = LegendListRef;
// Nested readers reuse the row attribute, so a row counts only inside the list's own item.
export function readingRow(
  list: ReadingListHandle,
  node: Node,
): { index: number; key: string } | undefined {
  const state = list.getState();
  for (
    let row = elementFor(node)?.closest<HTMLElement>("[data-reading-key]");
    row;
    row = row.parentElement?.closest<HTMLElement>("[data-reading-key]")
  ) {
    const key = row.dataset.readingKey!;
    const index = state.indexByKey(key);
    if (index !== undefined && state.elementAtIndex(index)?.contains(row))
      return { index, key };
  }
  return undefined;
}
function elementFor(node: Node | null): Element | null {
  if (!node) return null;
  const element = node instanceof Element ? node : node.parentElement;
  if (element?.closest("[data-reading-key]")) return element;
  const root = node.getRootNode();
  return root instanceof ShadowRoot ? elementFor(root.host) : element;
}
function selectedRows(root: HTMLElement, selector: string): HTMLElement[] {
  const selection = root.ownerDocument.getSelection();
  if (!selection || selection.rangeCount === 0) return [];
  const start = elementFor(selection.anchorNode)?.closest<HTMLElement>(
    selector,
  );
  const end = elementFor(selection.focusNode)?.closest<HTMLElement>(selector);
  if (selection.isCollapsed) {
    const active = root.ownerDocument.activeElement;
    if (!root.contains(active)) return [];
    if (start && !start.contains(active) && !active?.contains(start)) return [];
  }
  if (start && end && root.contains(start) && root.contains(end))
    return [start, end];
  if (!start && !end) return [];
  const range = selection.getRangeAt(0);
  return [...root.querySelectorAll<HTMLElement>(selector)].filter((row) =>
    range.intersectsNode(row),
  );
}

function editingHost(node: Node): HTMLElement | null {
  const element = node instanceof Element ? node : node.parentElement;
  if (
    !(element instanceof HTMLElement) ||
    !element.isConnected ||
    !element.isContentEditable
  )
    return null;
  let host = element;
  while (host.parentElement?.isContentEditable) host = host.parentElement;
  return host;
}

export function ReadingList<T>({
  data,
  role = "region",
  className,
  keyExtractor,
  renderItem,
  label,
  footer,
  header,
  ref,
  style,
  renderAll = false,
  maintainScrollAtEndThreshold,
  initialEndLayoutReady,
  onFollowChange,
}: {
  role?: "region" | "group";
  className?: string;
  data: readonly T[];
  keyExtractor(item: T): string;
  renderItem(item: T, index: number): ReactNode;
  label: string;
  footer?: ReactNode;
  header?: ReactNode;
  ref?: Ref<ReadingListHandle>;
  style?: CSSProperties;
  renderAll?: boolean;
  maintainScrollAtEndThreshold?: number;
  initialEndLayoutReady?: boolean;
  onFollowChange?(list: ReadingListHandle, atEnd: boolean): void;
}) {
  const ownerId = useId();
  const rowSelector = `[data-reading-key][data-reading-owner="${CSS.escape(ownerId)}"]`;
  const root = useRef<HTMLDivElement>(null);
  const [list, setList] = useState<ReadingListHandle | null>(null);
  const currentList = useRef<ReadingListHandle | null>(null);
  const [windowed, setWindowed] = useState<{
    generation: number;
    anchor: { index: number; viewOffset: number } | null;
  }>({ generation: 0, anchor: null });
  const waitForHostLayout = initialEndLayoutReady !== undefined && !ref;
  const [initialEndPending, setInitialEndPending] = useState(waitForHostLayout);
  const initialEnd = useRef(
    waitForHostLayout ? { listReady: false, layoutReady: false } : null,
  );
  const visible = useContentVisible();
  const waitingForInitialEnd =
    initialEndPending &&
    waitForHostLayout &&
    visible &&
    !renderAll &&
    windowed.generation === 0 &&
    windowed.anchor === null;
  const renderedAll = useRef(renderAll);
  const [selected, setSelected] = useState<string[]>([]);
  const [focused, setFocused] = useState<string | null>(null);
  const [composing, setComposing] = useState<string | null>(null);
  const focusOwner = useRef<{ key: string; target: EventTarget | null } | null>(
    null,
  );
  const retainFocusVisibility = useRef(true);
  const publish = useEffectEvent((value: ReadingListHandle, atEnd: boolean) =>
    onFollowChange?.(value, atEnd),
  );
  const bind = useCallback(
    (value: ReadingListHandle | null) => {
      currentList.current = value;
      setList(value);
      if (typeof ref === "function") return ref(value);
      if (ref) ref.current = value;
    },
    [ref],
  );
  const releaseInitialEnd = useCallback(() => {
    if (!initialEnd.current) return;
    initialEnd.current = null;
    setInitialEndPending(false);
  }, []);
  const settleInitialEnd = useCallback(() => {
    const readiness = initialEnd.current;
    if (!readiness?.listReady || !readiness.layoutReady) return;
    const value = currentList.current;
    const viewport = value?.getScrollableNode();
    if (!value || !viewport?.isConnected) return;
    const height = viewport.clientHeight;
    const { isAtEnd, isFollowingEnd, scrollLength } = value.getState();
    // Legend's accepted viewport can lag the host's committed layout.
    if (height === 0 || Math.abs(height - scrollLength) > 1) return;
    releaseInitialEnd();
    // A row can grow after initial scrolling while Legend still intends to follow.
    if (!isAtEnd || !isFollowingEnd)
      void value.scrollToEnd({ animated: false });
  }, [releaseInitialEnd]);
  const onInitialEndReady = () => {
    if (!initialEnd.current) return;
    initialEnd.current.listReady = true;
    if (initialEndLayoutReady) settleInitialEnd();
  };
  useLayoutEffect(() => {
    if (!waitingForInitialEnd) {
      releaseInitialEnd();
      return;
    }
    if (!initialEnd.current) return;
    initialEnd.current.layoutReady = initialEndLayoutReady === true;
    settleInitialEnd();
  }, [
    initialEndLayoutReady,
    list,
    releaseInitialEnd,
    settleInitialEnd,
    waitingForInitialEnd,
  ]);
  useEffect(() => {
    // Keep explicit navigation from competing with the initial placement.
    if (!list || waitingForInitialEnd) return;
    const state = list.getState();
    publish(list, state.isAtEnd);
    return state.listen("isAtEnd", (value) => publish(list, value));
  }, [list, waitingForInitialEnd]);
  useLayoutEffect(() => {
    if (!list || !visible) return;
    return () => {
      // A replaced windowed list is already unmounted; only the live list settles.
      if (currentList.current !== list) return;
      releaseInitialEnd();
      list.cancelScroll();
      list.anchorVisibleContent();
    };
  }, [list, releaseInitialEnd, visible]);
  useLayoutEffect(() => {
    const wasComplete = renderedAll.current;
    renderedAll.current = renderAll;
    if (!wasComplete || renderAll || !list) return;
    // Legend keeps every container it has filled, so ending a complete render
    // starts a fresh windowed list at the same reading position.
    const state = list.getState();
    const anchor = state.isAtEnd
      ? undefined
      : list.anchorVisibleContent({ hold: false });
    const index = anchor && state.indexByKey(anchor.key);
    setWindowed((previous) => ({
      generation: previous.generation + 1,
      anchor:
        anchor && index !== undefined
          ? { index, viewOffset: -anchor.offset }
          : null,
    }));
  }, [renderAll, list]);
  const selectionChanged = useEffectEvent(() => {
    const node = root.current;
    if (!list || !node) return;
    const rows = selectedRows(node, rowSelector);
    const indices = rows
      .map((row) => list.getState().indexByKey(row.dataset.readingKey!))
      .filter((index): index is number => index !== undefined);
    if (!indices.length) {
      if (selected.length) setSelected([]);
      return;
    }
    const from = indices.reduce((left, right) => Math.min(left, right));
    const to = indices.reduce((left, right) => Math.max(left, right));
    const keys = data.slice(from, to + 1).map(keyExtractor);
    if (
      keys.length === selected.length &&
      keys.every((key, index) => key === selected[index])
    )
      return;
    releaseInitialEnd();
    list.cancelScroll();
    list.anchorVisibleContent({ key: rows[0]!.dataset.readingKey });
    setSelected(keys);
  });
  useEffect(() => {
    const owner = root.current?.ownerDocument;
    if (!owner) return;
    const focusChanged = (event?: FocusEvent) => {
      selectionChanged();
      if (focusOwner.current?.target === (event?.target ?? owner.activeElement))
        return;
      focusOwner.current = null;
      setFocused(null);
      setComposing(null);
    };
    selectionChanged();
    focusChanged();
    owner.addEventListener("selectionchange", selectionChanged);
    owner.addEventListener("focusin", focusChanged);
    return () => {
      owner.removeEventListener("selectionchange", selectionChanged);
      owner.removeEventListener("focusin", focusChanged);
    };
  }, [list, rowSelector]);
  useLayoutEffect(() => {
    if (!focused || !list || !visible) return;
    const row = [
      ...(root.current?.querySelectorAll<HTMLElement>(rowSelector) ?? []),
    ].find((element) => element.dataset.readingKey === focused);
    if (!row) return;
    const reveal = () => {
      const active = row.ownerDocument.activeElement;
      if (
        !retainFocusVisibility.current ||
        row.querySelector(":active") ||
        !(active instanceof HTMLElement) ||
        !row.contains(active)
      )
        return;
      const rowBounds = row.getBoundingClientRect();
      const bounds = active.getBoundingClientRect();
      const index = list.getState().indexByKey(focused);
      if (index !== undefined)
        void list.scrollIndexIntoView({
          index,
          animated: false,
          itemOffset: Math.max(0, bounds.top - rowBounds.top),
          itemLength: bounds.height,
          viewportMargin: { start: 8, end: 8 },
        });
    };
    let frame: number | undefined;
    const requestReveal = () => {
      if (frame !== undefined) return;
      frame = requestAnimationFrame(() => {
        frame = undefined;
        reveal();
      });
    };
    const observer = new ResizeObserver(requestReveal);
    observer.observe(row);
    const stop = list.getState().listen("lastPositionUpdate", requestReveal);
    return () => {
      observer.disconnect();
      stop();
      if (frame !== undefined) cancelAnimationFrame(frame);
    };
  }, [focused, list, visible, rowSelector]);
  const onViewportKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (
      event.defaultPrevented ||
      !event.cancelable ||
      event.target !== event.currentTarget ||
      event.altKey ||
      event.ctrlKey ||
      event.metaKey ||
      event.shiftKey ||
      isImeKey(event.nativeEvent) ||
      (event.key !== "ArrowUp" && event.key !== "ArrowDown")
    )
      return;
    const reader = currentList.current;
    const viewport = reader?.getScrollableNode();
    if (
      !reader ||
      viewport !== event.currentTarget ||
      viewport.ownerDocument.activeElement !== viewport
    )
      return;
    const selection = viewport.ownerDocument.getSelection();
    if (selection?.rangeCount !== 1) return;
    const { startContainer, endContainer } = selection.getRangeAt(0);
    const host = editingHost(startContainer);
    if (
      !host ||
      viewport.contains(host) ||
      host.contains(viewport) ||
      editingHost(endContainer) !== host
    )
      return;

    // WebKit can send the default arrow action to the editor that retains the selection.
    event.preventDefault();
    // A relative scroll preserves the step as Legend measures rows.
    viewport.scrollBy({
      top: event.key === "ArrowUp" ? -WEBKIT_LINE_SCROLL : WEBKIT_LINE_SCROLL,
      behavior: "instant",
    });
  };
  const keys = new Set(data.map(keyExtractor));
  const pinned = [
    ...new Set(
      [...selected, focused, composing].filter(
        (key): key is string => key !== null && keys.has(key),
      ),
    ),
  ];
  return (
    <div
      ref={root}
      style={{ display: "flex", flex: 1, minHeight: 0, minWidth: 0, ...style }}
      onFocusCapture={waitingForInitialEnd ? releaseInitialEnd : undefined}
      onWheelCapture={() => {
        releaseInitialEnd();
        retainFocusVisibility.current = false;
      }}
      onInputCapture={() => {
        retainFocusVisibility.current = true;
      }}
      onPointerDownCapture={(event) => {
        releaseInitialEnd();
        if (event.target === list?.getScrollableNode())
          retainFocusVisibility.current = false;
      }}
    >
      <LegendList
        key={windowed.generation}
        ref={bind}
        role={role}
        aria-label={label}
        tabIndex={0}
        onKeyDown={onViewportKeyDown}
        className={className}
        data={data}
        extraData={renderItem}
        keyExtractor={keyExtractor}
        estimatedItemSize={160}
        recycleItems={false}
        maintainVisibleContentPosition
        maintainScrollAtEnd={{ animated: false }}
        maintainScrollAtEndThreshold={maintainScrollAtEndThreshold}
        onReady={waitingForInitialEnd ? onInitialEndReady : undefined}
        onLayout={
          waitingForInitialEnd && initialEndLayoutReady
            ? settleInitialEnd
            : undefined
        }
        initialScrollAtEnd={windowed.anchor === null}
        initialScrollIndex={windowed.anchor ?? undefined}
        alwaysRender={renderAll ? { top: data.length } : { keys: pinned }}
        style={{ width: "100%", height: "100%", minHeight: 0, flex: 1 }}
        ListHeaderComponent={header ? <>{header}</> : undefined}
        ListFooterComponent={footer ? <>{footer}</> : undefined}
        renderItem={({ item, index }) => {
          const key = keyExtractor(item);
          return (
            <div
              data-reading-key={key}
              data-reading-owner={ownerId}
              onFocusCapture={(event) => {
                retainFocusVisibility.current = true;
                focusOwner.current = { key, target: event.target };
                setFocused(key);
                list?.cancelScroll();
                list?.anchorVisibleContent({ key });
              }}
              onCompositionStartCapture={() => {
                retainFocusVisibility.current = true;
                setComposing(key);
                list?.cancelScroll();
              }}
              onCompositionEndCapture={() => setComposing(null)}
            >
              {renderItem(item, index)}
            </div>
          );
        }}
      />
    </div>
  );
}
