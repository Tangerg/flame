import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { useT } from "@/lib/i18n";
import type { AgentUnresolvedEffect } from "@/plugins/sdk/types/agentSessionView";
import { TextButton, Well } from "@/ui";
import { space } from "@/styles/tokens.stylex";

const styles = stylex.create({
  root: {
    display: "flex",
    flexDirection: "column",
    alignItems: "start",
    minWidth: 0,
    gap: space.s2,
  },
  detail: { width: "100%", boxSizing: "border-box" },
});

export function UnresolvedRunEffects({ effects }: { effects?: AgentUnresolvedEffect[] }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  if (!effects?.length) return null;
  return (
    <div {...stylex.props(styles.root)}>
      <TextButton tone="muted" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
        {t("agent.runOutcome.unresolvedEffects")}
      </TextButton>
      {open && (
        <Well wrap="anywhere" cap="md" className={stylex.props(styles.detail).className}>
          {JSON.stringify(effects, null, 2)}
        </Well>
      )}
    </div>
  );
}
