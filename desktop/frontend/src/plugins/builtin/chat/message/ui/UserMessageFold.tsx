import * as stylex from "@stylexjs/stylex";
import { type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { useT } from "@/lib/i18n";
import { Icon, TextButton } from "@/ui";
import { space } from "@/styles/tokens.stylex";

const FOLDED_LINES = 7;

const styles = stylex.create({
  frame: { position: "relative" },
  folded: {
    maxHeight: `${FOLDED_LINES}lh`,
    overflow: "hidden",
  },
  fade: {
    maskImage: "linear-gradient(to bottom, #000 calc(100% - 2lh), transparent)",
    WebkitMaskImage: "linear-gradient(to bottom, #000 calc(100% - 2lh), transparent)",
  },
  toggle: { marginTop: space.s1 },
});

export function UserMessageFold({ children }: { children: ReactNode }) {
  const t = useT();
  const frame = useRef<HTMLDivElement>(null);
  const [overflows, setOverflows] = useState(false);
  const [expanded, setExpanded] = useState(false);

  useLayoutEffect(() => {
    const element = frame.current;
    if (!element || expanded) return;
    const measure = () => setOverflows(element.scrollHeight > element.clientHeight + 1);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    for (const child of element.children) observer.observe(child);
    return () => observer.disconnect();
  }, [expanded, children]);

  const folded = !expanded;
  return (
    <>
      <div
        ref={frame}
        data-slot="user-message-fold"
        data-folded={folded && overflows ? "" : undefined}
        {...stylex.props(styles.frame, folded && styles.folded, folded && overflows && styles.fade)}
      >
        {children}
      </div>
      {(overflows || expanded) && (
        <TextButton
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
