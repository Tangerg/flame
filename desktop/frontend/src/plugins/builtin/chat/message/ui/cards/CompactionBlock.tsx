import * as stylex from "@stylexjs/stylex";
import { useId, useState } from "react";
import { Collapsible, Icon, TextButton, reveal } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";

const styles = stylex.create({
  open: { rotate: "180deg", opacity: 1 },
});

export function CompactionBlock({ summary }: { summary: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const label = t("compaction.compacted");

  return (
    <div data-slot="agent-activity-item" className="flex min-w-0 flex-col">
      <TextButton
        type="button"
        size="sm"
        onClick={() => setOpen((value) => !value)}
        aria-label={label}
        aria-expanded={open}
        aria-controls={panelId}
        className={cn(stylex.props(reveal.host).className, "max-w-full self-start py-1.5")}
      >
        <Icon name="minimize" size="xs" className="shrink-0 text-fg-faint" />
        <span className="min-w-0 truncate">{label}</span>
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
        <div
          id={panelId}
          className="mt-1.5 ml-5 max-w-[640px] whitespace-pre-wrap text-left text-ui-sm leading-prose text-fg-muted"
        >
          {summary}
        </div>
      </Collapsible>
    </div>
  );
}
