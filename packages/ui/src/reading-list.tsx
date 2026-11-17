import {
  useCallback,
  useEffect,
  useEffectEvent,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
  type Ref,
} from "react";
import { LegendList, type LegendListRef } from "@legendapp/list/react";
import { useContentVisible } from "./content-visibility";
export type ReadingListHandle = LegendListRef;
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
  onFollowChange?(list: ReadingListHandle, atEnd: boolean): void;
}) {
  const ownerId = useId();
  const rowSelector = `[data-reading-key][data-reading-owner="${CSS.escape(ownerId)}"]`;
  const root = useRef<HTMLDivElement>(null);
  const [list, setList] = useState<ReadingListHandle | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [focused, setFocused] = useState<string | null>(null);
  const [composing, setComposing] = useState<string | null>(null);
  const focusOwner = useRef<{ key: string; target: EventTarget | null } | null>(
    null,
  );
  const retainFocusVisibility = useRef(true);
  const visible = useContentVisible();
  const publish = useEffectEvent((value: ReadingListHandle, atEnd: boolean) =>
    onFollowChange?.(value, atEnd),
  );
  const bind = useCallback(
    (value: ReadingListHandle | null) => {
      setList(value);
      if (typeof ref === "function") return ref(value);
      if (ref) ref.current = value;
    },
    [ref],
  );
  useEffect(() => {
    if (!list) return;
    const state = list.getState();
    publish(list, state.isAtEnd);
    return state.listen("isAtEnd", (value) => publish(list, value));
  }, [list]);
  useLayoutEffect(() => {
    if (!list || !visible) return;
    return () => {
      list.cancelScroll();
      list.anchorVisibleContent();
    };
  }, [list, visible]);
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
      onWheelCapture={() => {
        retainFocusVisibility.current = false;
      }}
      onInputCapture={() => {
        retainFocusVisibility.current = true;
      }}
      onPointerDownCapture={(event) => {
        if (event.target === list?.getScrollableNode())
          retainFocusVisibility.current = false;
      }}
    >
      <LegendList
        ref={bind}
        role={role}
        aria-label={label}
        tabIndex={0}
        className={className}
        data={data}
        extraData={renderItem}
        keyExtractor={keyExtractor}
        estimatedItemSize={160}
        recycleItems={false}
        maintainVisibleContentPosition
        maintainScrollAtEnd={{ animated: false }}
        initialScrollAtEnd
        alwaysRender={{ keys: pinned }}
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
