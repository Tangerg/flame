import type { BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import { useCallback, useEffect, useRef, useState } from "react";
import { MarkdownMessage } from "../markdown/MarkdownMessage";
import { Icon, Loader } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";

interface Props {
  text: string;
  status: BlockStatus;
  superseded?: boolean;
}

export function ReasoningBlock({ text, status, superseded = false }: Props) {
  const t = useT();
  const streaming = status === "running";
  const [openOverride, setOpenOverride] = useState<boolean | null>(null);
  const isOpen = openOverride ?? (streaming && !superseded);

  const toggle = () => {
    setOpenOverride(!isOpen);
  };

  const label = streaming ? t("reasoning.thinking") : t("reasoning.thought");

  const scrollRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({ scrolled: false, atBottom: true, overflowing: false });
  const measure = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const next = {
      scrolled: el.scrollTop > 0,
      atBottom: el.scrollHeight - el.scrollTop - el.clientHeight < 4,
      overflowing: el.scrollHeight > el.clientHeight,
    };
    setEdges((prev) =>
      prev.scrolled === next.scrolled &&
      prev.atBottom === next.atBottom &&
      prev.overflowing === next.overflowing
        ? prev
        : next,
    );
  }, []);

  useEffect(() => {
    if (!streaming) return;
    const scrollEl = scrollRef.current;
    const contentEl = contentRef.current;
    if (!scrollEl || !contentEl) return;
    const pin = () => {
      const distanceFromBottom = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight;
      if (distanceFromBottom < 4) {
        scrollEl.scrollTop = scrollEl.scrollHeight;
      }
      measure();
    };
    pin();
    const ro = new ResizeObserver(pin);
    ro.observe(contentEl);
    return () => ro.disconnect();
  }, [streaming, measure]);

  useEffect(() => {
    measure();
  }, [text, isOpen, measure]);

  const showTopFade = isOpen && edges.scrolled;
  const showBottomFade = isOpen && streaming && edges.overflowing && !edges.atBottom;

  return (
    <AgentActivityDisclosure
      icon="sparkle"
      shell="line"
      label={streaming ? <Loader variant="text-shimmer" size="sm" text={label} /> : label}
      toggleLabel={label}
      open={isOpen}
      onToggle={toggle}
      contentClassName="ml-5 border-l border-field pt-0.5 pl-6"
    >
      <div
        ref={scrollRef}
        // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        tabIndex={streaming && isOpen ? 0 : undefined}
        onScroll={measure}
        className={cn(
          "relative overflow-hidden pr-2",
          streaming && isOpen && "max-h-48 overflow-y-auto",
        )}
      >
        <div
          className={cn(
            "pointer-events-none absolute inset-x-0 top-0 z-1 h-6",
            "bg-[linear-gradient(to_bottom,var(--app-content-surface),transparent)]",
            "transition-opacity duration-[var(--dur-fast)]",
            showTopFade ? "opacity-100" : "opacity-0",
          )}
        />
        <div
          ref={contentRef}
          className="whitespace-pre-wrap text-ui-sm leading-prose text-fg-muted"
        >
          <MarkdownMessage text={text} streaming={streaming} reveal="smooth" />
          {status === "incomplete" && (
            <div className="mt-1 font-mono text-ui-sm text-fg-faint">
              <Icon name="x" size="xs" /> {t("reasoning.interrupted")}
            </div>
          )}
        </div>
        <div
          className={cn(
            "pointer-events-none absolute inset-x-0 bottom-0 z-1 h-6",
            "bg-[linear-gradient(to_top,var(--app-content-surface),transparent)]",
            "transition-opacity duration-[var(--dur-fast)]",
            showBottomFade ? "opacity-100" : "opacity-0",
          )}
        />
      </div>
    </AgentActivityDisclosure>
  );
}
