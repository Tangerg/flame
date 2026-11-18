import { Component, type ComponentPropsWithoutRef, type SyntheticEvent } from "react";
import { readingRow, type ReadingListHandle } from "@flame/ui";

interface Props extends ComponentPropsWithoutRef<"div"> {
  reader: ReadingListHandle | null;
  visible: boolean;
  fullWidth: boolean;
}

interface BlockAnchor {
  node: HTMLElement;
  line: { text: Text; offset: number } | null;
  portion: number;
  viewportOffset: number;
}

interface ReadingAnchor {
  block: BlockAnchor | null;
  row: ReturnType<ReadingListHandle["anchorVisibleContent"]>;
}

interface PresentationSnapshot {
  anchor: ReadingAnchor;
  width: number;
}

function captureAnchor(reader: ReadingListHandle, viewport: HTMLElement): ReadingAnchor | null {
  const view = viewport.getBoundingClientRect();
  const blocks = viewport.querySelectorAll<HTMLElement>(
    ".md :is(p,h1,h2,h3,h4,h5,h6,li,pre,figure,table)",
  );
  for (const node of blocks) {
    const bounds = node.getBoundingClientRect();
    if (
      !bounds.height ||
      bounds.bottom <= view.top ||
      bounds.top >= view.bottom ||
      node.closest("[hidden], [inert]")
    )
      continue;

    let top = Math.max(bounds.top, view.top);
    let line: BlockAnchor["line"] = null;
    if (bounds.top < view.top) {
      const caret = node.ownerDocument.caretRangeFromPoint?.(
        Math.max(bounds.left, view.left) + 2,
        Math.min(bounds.bottom - 1, view.top + 3),
      );
      if (caret && caret.startContainer instanceof Text && node.contains(caret.startContainer)) {
        const rect = caret.getBoundingClientRect();
        if (rect.height && rect.bottom >= view.top) {
          line = { text: caret.startContainer, offset: caret.startOffset };
          top = rect.top;
        }
      }
    }

    const key = readingRow(reader, node)?.key;
    return {
      block: {
        node,
        line,
        portion: (top - bounds.top) / bounds.height,
        viewportOffset: top - view.top,
      },
      row: reader.anchorVisibleContent({ key, hold: false }),
    };
  }

  const row = reader.anchorVisibleContent({ hold: false });
  return row ? { block: null, row } : null;
}

function blockTop(anchor: BlockAnchor, viewport: HTMLElement): number | undefined {
  const { node, line, portion } = anchor;
  if (!viewport.contains(node)) return;
  const bounds = node.getBoundingClientRect();
  if (!bounds.height) return;
  if (line && node.contains(line.text) && line.offset <= line.text.length) {
    const caret = node.ownerDocument.createRange();
    caret.setStart(line.text, line.offset);
    caret.collapse(true);
    const rect = caret.getBoundingClientRect();
    if (rect.height) return rect.top;
  }
  return bounds.top + bounds.height * portion;
}

export class SubagentReadingSurface extends Component<Props, unknown, PresentationSnapshot | null> {
  private anchor: ReadingAnchor | null = null;
  private stopSettling: (() => void) | undefined;

  private retire = () => {
    this.anchor = null;
    this.stopSettling?.();
  };

  private retireOnInput = (event: SyntheticEvent<HTMLDivElement>) => {
    if (event.target instanceof Node && event.currentTarget.contains(event.target)) this.retire();
  };

  override getSnapshotBeforeUpdate(previous: Props): PresentationSnapshot | null {
    const { reader, visible, fullWidth } = this.props;
    if (
      !visible ||
      !previous.visible ||
      !reader ||
      reader !== previous.reader ||
      fullWidth === previous.fullWidth
    )
      return null;
    const viewport = reader.getScrollableNode();
    if (!viewport?.isConnected) return null;

    // A presentation can clamp to the physical end while the reader is still reading history.
    const anchor =
      this.anchor ??
      (viewport.scrollHeight - viewport.clientHeight - viewport.scrollTop > 1 &&
      !reader.getState().isFollowingEnd
        ? captureAnchor(reader, viewport)
        : null);
    return anchor ? { anchor, width: viewport.clientWidth } : null;
  }

  override componentDidUpdate(
    previous: Props,
    _state: unknown,
    snapshot: PresentationSnapshot | null,
  ) {
    const { reader, visible, fullWidth } = this.props;
    if (!visible || reader !== previous.reader) {
      this.retire();
      return;
    }
    if (fullWidth === previous.fullWidth) return;
    this.stopSettling?.();
    this.anchor = snapshot?.anchor ?? null;
    if (!snapshot || !reader) return;

    const viewport = reader.getScrollableNode();
    if (!viewport?.isConnected) {
      this.retire();
      return;
    }

    const { block, row } = snapshot.anchor;
    let queued = false;
    const settle = () => {
      queued = false;
      if (this.stopSettling !== stop) return;
      if (!viewport.isConnected) {
        this.retire();
        return;
      }
      const state = reader.getState();
      const height = viewport.clientHeight;
      if (
        viewport.clientWidth === snapshot.width ||
        height === 0 ||
        Math.abs(height - state.scrollLength) > 1 ||
        Math.abs(Math.max(height, state.contentLength) - viewport.scrollHeight) > 1
      )
        return;

      const index = row ? state.indexByKey(row.key) : undefined;
      const bounds =
        index === undefined ? undefined : state.elementAtIndex(index)?.getBoundingClientRect();
      if (bounds && index !== undefined) {
        const measured = state.sizeAtIndex(index);
        if (!Number.isFinite(measured) || Math.abs(measured - bounds.height) > 1) return;
      }

      const top = block ? blockTop(block, viewport) : undefined;
      const desiredTop =
        top !== undefined && block
          ? top - block.viewportOffset
          : bounds && row
            ? bounds.top + row.offset
            : undefined;
      if (desiredTop === undefined) {
        if (!row || index === undefined) {
          this.retire();
          return;
        }
        stop();
        void reader.scrollToKey({
          key: row.key,
          viewPosition: 0,
          viewOffset: -row.offset,
          animated: false,
        });
        return;
      }

      const offset = Math.max(
        0,
        viewport.scrollTop + desiredTop - viewport.getBoundingClientRect().top,
      );
      stop();
      if (Math.abs(offset - viewport.scrollTop) > 1)
        void reader.scrollToOffset({ offset, animated: false });
    };
    const schedule = () => {
      if (queued) return;
      queued = true;
      // Read geometry after the current resize notifications finish.
      queueMicrotask(settle);
    };
    const state = reader.getState();
    const observer = new ResizeObserver(schedule);
    const stopSize = state.listen("totalSize", schedule);
    const stopPosition = state.listen("lastPositionUpdate", schedule);
    const stop = () => {
      observer.disconnect();
      stopSize();
      stopPosition();
      this.stopSettling = undefined;
    };
    this.stopSettling = stop;
    observer.observe(viewport);
    const index = row ? state.indexByKey(row.key) : undefined;
    const item = index === undefined ? undefined : state.elementAtIndex(index);
    if (item) observer.observe(item);
    schedule();
  }

  override componentWillUnmount() {
    this.retire();
  }

  override render() {
    const { reader: _reader, visible: _visible, fullWidth: _fullWidth, ...props } = this.props;
    return (
      <div
        {...props}
        onClickCapture={this.retireOnInput}
        onWheelCapture={this.retireOnInput}
        onFocusCapture={this.retireOnInput}
        onPointerDownCapture={this.retireOnInput}
        onKeyDownCapture={this.retireOnInput}
      />
    );
  }
}
