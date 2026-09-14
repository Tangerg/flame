import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { useModelInvocations, type ModelInvocation } from "@/plugins/builtin/agent/public/run";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { useT, activeLocale } from "@/lib/i18n";
import { fmtDuration, fmtTokens } from "@/lib/format";
import { Badge, Button, DataView, IconButton, vocab } from "@/ui";
import { face, type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./viewStyles";

function duration(call: ModelInvocation): string {
  if ((call.state !== "completed" && call.state !== "failed") || call.settledAt === undefined)
    return "—";
  return fmtDuration(Date.parse(call.settledAt) - Date.parse(call.startedAt));
}

export function ModelInvocationHistory({ run }: { run: AgentRunView }) {
  const t = useT();
  const [cursor, setCursor] = useState<string>();
  const { data, isLoading, isError, refetch } = useModelInvocations(run, cursor);
  return (
    <div {...stylex.props(vs.gutter, vs.rowPad)}>
      <div {...stylex.props(vs.splitLine)}>
        <span {...stylex.props(typeStep.uiSm)}>{t("timeline.modelCalls")}</span>
        <IconButton
          icon="loop"
          title={t("timeline.refreshCalls")}
          onClick={() => {
            void refetch();
          }}
        />
      </div>
      {run.modelSelection && (
        <div {...stylex.props(vs.titleLine, vocab.faint, face.mono, typeStep.uiXs)}>
          <span
            title={`${run.modelSelection.provider}/${run.modelSelection.model}`}
            {...stylex.props(vocab.truncate)}
          >
            {run.modelSelection.provider}/{run.modelSelection.model}
          </span>
          {run.modelSelection.reasoningEffort && (
            <span>
              {t("composer.model.reasoning")} {run.modelSelection.reasoningEffort}
            </span>
          )}
        </div>
      )}
      <DataView
        items={data?.data ?? []}
        isLoading={isLoading}
        isError={isError}
        onRetry={refetch}
        skeletonCount={2}
        empty={{ icon: "bot", title: t("timeline.noModelCalls") }}
      >
        {(calls) =>
          calls.map((call) => (
            <div key={call.callId} {...stylex.props(vs.lineBaseline, vs.rowPad)}>
              <div {...stylex.props(vocab.fill)}>
                <div
                  title={call.callId}
                  {...stylex.props(vocab.truncate, face.mono, typeStep.uiXs)}
                >
                  {call.callId}
                </div>
                <div title={call.segmentId} {...stylex.props(vocab.faint, typeStep.uiXs)}>
                  {new Date(call.startedAt).toLocaleString(activeLocale())}
                </div>
                <div {...stylex.props(vs.titleLine, vocab.faint, face.mono, typeStep.uiXs)}>
                  <span title={call.usage?.inputTokens.toString()}>
                    ↑{call.usage ? fmtTokens(call.usage.inputTokens) : "—"}
                  </span>
                  <span title={call.usage?.outputTokens.toString()}>
                    ↓{call.usage ? fmtTokens(call.usage.outputTokens) : "—"}
                  </span>
                  {call.firstOutputLatencyMillis !== undefined && (
                    <span>
                      {t("timeline.firstOutput")} {fmtDuration(call.firstOutputLatencyMillis)}
                    </span>
                  )}
                  {call.usage && call.usage.cacheReadTokens > 0 && (
                    <span>
                      {t("usage.cache")} {fmtTokens(call.usage.cacheReadTokens)}
                    </span>
                  )}
                  {call.usage && call.usage.cacheWriteTokens > 0 && (
                    <span>
                      {t("usage.cacheWrite")} {fmtTokens(call.usage.cacheWriteTokens)}
                    </span>
                  )}
                  {call.usage && call.usage.reasoningTokens > 0 && (
                    <span>
                      {t("usage.reasoning")} {fmtTokens(call.usage.reasoningTokens)}
                    </span>
                  )}
                </div>
              </div>
              <Badge
                tone={
                  call.state === "failed"
                    ? "negative"
                    : call.state === "unknown"
                      ? "warning"
                      : "neutral"
                }
              >
                {t(`timeline.modelCall.${call.state}`)}
              </Badge>
              <span {...stylex.props(vocab.hold, face.mono, typeStep.uiXs)}>{duration(call)}</span>
            </div>
          ))
        }
      </DataView>
      <div {...stylex.props(vs.splitLine)}>
        {cursor && (
          <Button variant="ghost" onClick={() => setCursor(undefined)}>
            {t("timeline.latestCalls")}
          </Button>
        )}
        {data?.nextCursor && (
          <Button variant="ghost" onClick={() => setCursor(data.nextCursor)}>
            {t("timeline.olderCalls")}
          </Button>
        )}
      </div>
    </div>
  );
}
