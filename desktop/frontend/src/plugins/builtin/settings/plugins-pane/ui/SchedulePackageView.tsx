import { useMemo, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import type { Page, Schedule, PluginScheduleTemplate } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { SystemMessage, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import type { PluginViewReads } from "../application/pluginView";
import { PackageView } from "./PackageView";
import { ScheduleActions } from "./ScheduleActions";

const styles = stylex.create({
  root: { height: "100%", minHeight: 0 },
  page: { flex: 1, minHeight: 0 },
});

export function SchedulePackageView({
  reads: createReads,
  templates,
  ...props
}: {
  reads: () => PluginViewReads<Page<Schedule>>;
  templates: PluginScheduleTemplate[];
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const available = useRuntimeCapability("schedules");
  const reads = useMemo(() => createReads(), [createReads]);
  const [refreshVersion, setRefreshVersion] = useState(0);
  if (!available) return <SystemMessage>{t("schedules.unavailable")}</SystemMessage>;
  return (
    <div {...stylex.props(vocab.column, styles.root)}>
      <ScheduleActions
        templates={templates}
        signal={props.lifetime.signal}
        onSaved={() => setRefreshVersion((value) => value + 1)}
      />
      <div {...stylex.props(styles.page)}>
        <PackageView key={refreshVersion} {...props} reads={reads} />
      </div>
    </div>
  );
}
