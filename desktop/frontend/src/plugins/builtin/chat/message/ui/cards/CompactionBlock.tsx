import * as stylex from "@stylexjs/stylex";
import { useId, useState } from "react";
import { Collapsible, Icon, reveal, TextButton, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { color, leading, space, type as typeStep } from "@/styles/tokens.stylex";

const cb = stylex.create({
  host: { display: "flex", minWidth: 0, flexDirection: "column" },
  // The summary indents past the mark and holds a reading measure of its own.
  summary: {
    marginTop: space.s1_5,
    marginLeft: space.s5,
    maxWidth: "640px",
    whiteSpace: "pre-wrap",
    textAlign: "left",
    lineHeight: leading.prose,
    color: color.fgMuted,
  },
});

const styles = stylex.create({
  open: { rotate: "180deg", opacity: 1 },
});

export function CompactionBlock({ summary }: { summary: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const label = t("compaction.compacted");

  return (
    <div data-slot="agent-activity-item" {...stylex.props(cb.host)}>
      <TextButton
        type="button"
        size="sm"
        onClick={() => setOpen((value) => !value)}
        aria-label={label}
        aria-expanded={open}
        aria-controls={panelId}
        className={cn(stylex.props(reveal.host).className, "max-w-full self-start py-1.5")}
      >
        <Icon
          name="minimize"
          size="xs"
          className={stylex.props(vocab.hold, vocab.faint).className}
        />
        <span {...stylex.props(vocab.min, vocab.truncate)}>{label}</span>
        <Icon
          name="chevron-down"
          size="xs"
          data-reveal="hover"
          className={cn(
            "shrink-0 text-fg-faint transition-[opacity,transform] duration-[var(--dur-fast)]",
            // The open state has to be stated beside the reveal, not on top of it: two rules for
            // one property in different layers means the generated one simply wins.
            stylex.props(reveal.shown, open && styles.open).className,
          )}
        />
      </TextButton>
      <Collapsible open={open}>
        <div id={panelId} className={stylex.props(cb.summary, typeStep.uiSm).className}>
          {summary}
        </div>
      </Collapsible>
    </div>
  );
}
