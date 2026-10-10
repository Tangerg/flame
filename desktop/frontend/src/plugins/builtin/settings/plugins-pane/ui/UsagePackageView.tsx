import { useMemo, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import type { UsageSummary, UsageSummaryRequest } from "@flame/runtime-contract/wire";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { Segmented, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { space } from "@/styles/tokens.stylex";
import type { PluginViewReads } from "../application/pluginView";
import { PackageView } from "./PackageView";

const periods = [
  { value: "all", label: "usage.range.all", query: {} },
  { value: "30", label: "usage.range.30d", query: { sinceDays: 30 } },
  { value: "7", label: "usage.range.7d", query: { sinceDays: 7 } },
] as const;

const styles = stylex.create({
  root: { height: "100%", minHeight: 0 },
  controls: { padding: space.s3, flexShrink: 0 },
  page: { flex: 1, minHeight: 0 },
});

export function UsagePackageView({
  reads: createReads,
  ...props
}: {
  reads: (period: Readonly<UsageSummaryRequest>) => PluginViewReads<UsageSummary>;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const [period, setPeriod] = useState<(typeof periods)[number]>(periods[0]);
  const reads = useMemo(() => createReads(period.query), [createReads, period]);
  return (
    <div {...stylex.props(vocab.column, styles.root)}>
      <div {...stylex.props(styles.controls)}>
        <Segmented
          value={period.value}
          options={periods.map((item) => ({ value: item.value, label: t(item.label) }))}
          onChange={(value) => setPeriod(periods.find((item) => item.value === value)!)}
          ariaLabel={t("usage.rangeAria")}
        />
      </div>
      <div {...stylex.props(styles.page)}>
        <PackageView key={period.value} {...props} reads={reads} />
      </div>
    </div>
  );
}
