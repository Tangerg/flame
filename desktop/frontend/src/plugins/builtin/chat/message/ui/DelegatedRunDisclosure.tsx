import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { useState } from "react";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { IconButton, StatusDot, toneInk, vocab } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { delegatedRunCardModel } from "../application/delegatedRunCardModel";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { face, space, type as typeStep } from "@/styles/tokens.stylex";
import { messageStyles } from "./messageStyles";

interface Props {
  run: AgentRunView;
  ordinal: number;
  siblingCount: number;
  hasMaterial: boolean;
  onCancel: () => void;
  onOpenAudit: () => void;
  children: ReactNode;
}

/** A delegated run's body clears its own bottom edge; the card above owns the rest. */
const dr = stylex.create({
  body: { paddingBottom: space.s2_5 },
});

export function DelegatedRunDisclosure({
  run,
  ordinal,
  siblingCount,
  hasMaterial,
  onCancel,
  onOpenAudit,
  children,
}: Props) {
  const t = useT();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const model = delegatedRunCardModel(t, run, ordinal, siblingCount);
  const [pinnedExpanded, setPinnedExpanded] = useState<boolean | null>(null);
  const expanded = pinnedExpanded ?? model.autoExpanded;

  return (
    <AgentActivityDisclosure
      icon="bot"
      shell="card"
      contentClassName={stylex.props(dr.body).className}
      label={model.label}
      detail={
        model.detail ? (
          <span title={model.detail} {...stylex.props(vocab.pretty)}>
            {model.detail}
          </span>
        ) : undefined
      }
      trailing={
        <>
          <span {...stylex.props(messageStyles.statusWord, toneInk[model.ink], typeStep.uiXs)}>
            <StatusDot tone={model.dotTone} />
            <span {...stylex.props(vocab.min, vocab.truncate)}>{model.statusLabel}</span>
          </span>
          <span {...stylex.props(vocab.hold, vocab.faint, typeStep.uiXs, face.mono)}>
            {model.stepsLabel}
          </span>
        </>
      }
      actions={
        <>
          <IconButton
            icon="history"
            size="sm"
            quiet
            title={t("agent.runTree.action.audit")}
            onClick={onOpenAudit}
          />
          {model.cancelable && (
            <IconButton
              icon="stop"
              size="sm"
              quiet
              disabled={!runtimeAvailable}
              title={t("agent.runTree.action.cancel")}
              onClick={onCancel}
            />
          )}
        </>
      }
      open={expanded}
      onToggle={() => setPinnedExpanded(!expanded)}
      tone={model.shell}
    >
      {hasMaterial ? (
        children
      ) : (
        <p {...stylex.props(vocab.pretty, vocab.muted, typeStep.uiSm)}>
          {t("agent.runTree.material.empty")}
        </p>
      )}
    </AgentActivityDisclosure>
  );
}
