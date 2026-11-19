import * as stylex from "@stylexjs/stylex";
import { type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { useT } from "@/lib/i18n/index";
import { Icon } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { space } from "@flame/ui/tokens.stylex";
const FOLDED_LINES = 7;
const FADE_LINES = 2;
const FADE_MASK = `linear-gradient(to bottom, #000 calc(100% - ${FADE_LINES}lh), transparent)`;

const styles = stylex.create({
  frame: { position: "relative" },
  folded: {
    maxHeight: `${FOLDED_LINES}lh`,
    overflow: "hidden",
  },
  fade: {
    maskImage: FADE_MASK,
    WebkitMaskImage: FADE_MASK,
  },
  toggle: { marginTop: space.s1 },
});

const revealFold = Symbol();
type FoldFrame = HTMLDivElement & { [revealFold]?: (range: Range) => void };

export function revealUserMessageFold(element: HTMLElement, range?: Range): boolean {
  const reveal = (element as FoldFrame)[revealFold];
  if (!reveal) return false;
  if (range) reveal(range);
  return true;
}

export function UserMessageFold({ children }: { children: ReactNode }) {
  const t = useT();
  const frame = useRef<FoldFrame>(null);
  const toggle = useRef<HTMLButtonElement>(null);
  const [overflows, setOverflows] = useState(false);
  const [expanded, setExpanded] = useState(false);

  useLayoutEffect(() => {
    const element = frame.current;
    if (!element || expanded) return;
    const measure = () => {
      const nextOverflows = element.scrollHeight > element.clientHeight + 1;
      const retiring = toggle.current;
      if (!nextOverflows && retiring && retiring === retiring.ownerDocument.activeElement) {
        // This local measurement can retire the focused toggle without a parent message render.
        retiring.parentElement?.closest<HTMLElement>("[tabindex]")?.focus({ preventScroll: true });
      }
      setOverflows(nextOverflows);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    for (const child of element.children) observer.observe(child);
    return () => observer.disconnect();
  }, [expanded, children]);

  useLayoutEffect(() => {
    const element = frame.current;
    if (!element) return;
    if (!expanded) {
      element.scrollTop = 0;
      return;
    }
    const focused = element.ownerDocument.activeElement;
    if (focused && element.contains(focused)) focused.scrollIntoView({ block: "nearest" });
  }, [expanded]);

  useLayoutEffect(() => {
    const element = frame.current;
    if (!element) return;
    element[revealFold] = (range) => {
      if (
        expanded ||
        !overflows ||
        !element.contains(range.startContainer) ||
        !element.contains(range.endContainer)
      ) {
        return;
      }
      const clip = element.getBoundingClientRect();
      const clearBottom = clip.bottom - (FADE_LINES * clip.height) / FOLDED_LINES;
      const obscured = [...range.getClientRects()].some(
        (box) =>
          box.width > 0 &&
          box.height > 0 &&
          (box.left < clip.left - 0.5 ||
            box.right > clip.right + 0.5 ||
            box.top < clip.top - 0.5 ||
            box.bottom > clearBottom + 0.5),
      );
      // Find measures the transcript row immediately after this explicit reveal.
      if (obscured) flushSync(() => setExpanded(true));
    };
    return () => {
      delete element[revealFold];
    };
  }, [expanded, overflows]);

  const folded = !expanded;
  return (
    <>
      <div
        ref={frame}
        data-slot="user-message-fold"
        data-folded={folded && overflows ? "" : undefined}
        onFocusCapture={() => {
          if (folded && overflows) setExpanded(true);
        }}
        {...stylex.props(styles.frame, folded && styles.folded, folded && overflows && styles.fade)}
      >
        {children}
      </div>
      {(overflows || expanded) && (
        <TextButton
          ref={toggle}
          tone="muted"
          size="sm"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
          className={stylex.props(styles.toggle).className}
        >
          <Icon name={expanded ? "chevron-up" : "chevron-down"} size="xs" />
          {expanded ? t("message.user.fold") : t("message.user.unfold")}
        </TextButton>
      )}
    </>
  );
}
