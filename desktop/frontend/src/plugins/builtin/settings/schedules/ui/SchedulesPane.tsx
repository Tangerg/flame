import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { DataView, EmptyState, gap, Icon, PillButton, vocab } from "@/ui";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { useT } from "@/lib/i18n";
import { useScheduleConfigs } from "../application/scheduleCommands";
import { ScheduleForm } from "./ScheduleForm";
import { ScheduleRow } from "./ScheduleRow";
import { type as typeStep } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

export function SchedulesPane() {
  const enabled = useRuntimeCapability("schedules");
  const t = useT();
  if (!enabled) {
    return (
      <EmptyState
        icon="command"
        title={t("schedules.unavailable")}
        sub={t("schedules.unavailable.sub")}
      />
    );
  }
  return <EnabledSchedulesPane />;
}

function EnabledSchedulesPane() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const { data, isLoading, isError, refetch } = useScheduleConfigs();
  const [adding, setAdding] = useState(false);

  return (
    <div {...stylex.props(ss.stack)}>
      <p {...stylex.props(ss.intro, typeStep.uiMd)}>{t("schedules.intro")}</p>

      {adding ? (
        <ScheduleForm
          defaultCwd={cwd}
          onDone={() => setAdding(false)}
          onCancel={() => setAdding(false)}
        />
      ) : (
        <div {...stylex.props(ss.end)}>
          <PillButton
            variant="outlined"
            size="sm"
            disabled={workspace.status === "resolving"}
            onClick={() => setAdding(true)}
          >
            <Icon name="plus" size="sm" />
            {t("schedules.add")}
          </PillButton>
        </div>
      )}

      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        onRetry={refetch}
        skeletonCount={3}
        empty={{ icon: "command", title: t("schedules.empty"), sub: t("schedules.empty.sub") }}
      >
        {(rows) => (
          <div {...stylex.props(vocab.column, gap.s2)}>
            {rows.map((schedule) => (
              <ScheduleRow key={schedule.id} schedule={schedule} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}
