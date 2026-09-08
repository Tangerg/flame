import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { useState } from "react";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { IconButton, StatusDot } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { delegatedRunCardModel } from "../application/delegatedRunCardModel";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { face, type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../chatStyles";

interface Props {
  run: AgentRunView;
  ordinal: number;
  siblingCount: number;
  hasMaterial: boolean;
  onCancel: () => void;
  onOpenAudit: () => void;
  children: ReactNode;
}

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
      contentClassName="pb-2.5"
      label={model.label}
      detail={
        model.detail ? (
          <span title={model.detail} {...stylex.props(ct.pretty)}>
            {model.detail}
          </span>
        ) : undefined
      }
      trailing={
        <>
          <span
            className={cn(
              "inline-flex items-center gap-1 text-ui-xs font-medium",
              model.status === "running"
                ? "text-info"
                : model.status === "waiting" || model.status === "limit"
                  ? "text-warning"
                  : model.status === "error"
                    ? "text-negative"
                    : "text-fg-muted",
            )}
          >
            <StatusDot tone={model.dotTone} />
            {model.statusLabel}
          </span>
          <span {...stylex.props(ct.faint, typeStep.uiXs, face.mono)}>{model.stepsLabel}</span>
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
      tone={
        model.status === "error"
          ? "negative"
          : model.status === "waiting" || model.status === "limit"
            ? "warning"
            : "neutral"
      }
    >
      {hasMaterial ? (
        children
      ) : (
        <p {...stylex.props(ct.pretty, ct.muted, typeStep.uiSm)}>
          {t("agent.runTree.material.empty")}
        </p>
      )}
    </AgentActivityDisclosure>
  );
}
