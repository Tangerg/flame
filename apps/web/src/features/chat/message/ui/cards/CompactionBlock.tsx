import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { AgentActivityDisclosure } from "@/ui/agent/index";
import { useT } from "@/lib/i18n/index";
import { color, leading, type as typeStep } from "@flame/ui/tokens.stylex";
const cb = stylex.create({
  summary: {
    maxWidth: "640px",
    whiteSpace: "pre-wrap",
    textAlign: "left",
    lineHeight: leading.prose,
    color: color.fgMuted,
  },
});

export function CompactionBlock({ summary }: { summary: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const label = t("compaction.compacted");

  return (
    <AgentActivityDisclosure
      icon="fold"
      shell="line"
      label={label}
      open={open}
      onToggle={() => setOpen((value) => !value)}
      contentClassName={stylex.props(cb.summary, typeStep.uiSm).className}
    >
      {summary}
    </AgentActivityDisclosure>
  );
}
