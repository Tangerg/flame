import { useMemo, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { useTelemetryStore } from "@/lib/observability/stores";
import { useT } from "@/lib/i18n";
import { Button, vocab } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import type { PluginViewReads } from "../application/pluginView";
import type { ClientDiagnosticsSnapshot } from "../adapters/clientViewReads";
import { PackageView } from "./PackageView";

const styles = stylex.create({
  root: { height: "100%", minHeight: 0 },
  controls: { padding: space.s3, flexShrink: 0 },
  page: { flex: 1, minHeight: 0 },
});

export function ClientDiagnosticsPackageView({
  reads: createReads,
  ...props
}: {
  reads: () => PluginViewReads<ClientDiagnosticsSnapshot>;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
}) {
  const t = useT();
  const clear = useTelemetryStore((state) => state.clear);
  const [revision, setRevision] = useState(0);
  const reads = useMemo(() => createReads(), [createReads]);
  return (
    <div {...stylex.props(vocab.column, styles.root)}>
      <div {...stylex.props(styles.controls)}>
        <Button
          variant="ghost"
          size="sm"
          title={t("diagnostics.clear.hint")}
          onClick={() => {
            clear();
            setRevision((value) => value + 1);
          }}
        >
          {t("diagnostics.clear")}
        </Button>
      </div>
      <div {...stylex.props(styles.page)}>
        <PackageView key={revision} {...props} reads={reads} />
      </div>
    </div>
  );
}
