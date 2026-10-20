import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { DataView } from "@/ui/index";
import { SectionLabel } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { gap, RecordGroup, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
import { useProviderConfigs } from "@/features/providers/application/providerConfig";
import { needsProviderSetup } from "@/features/providers/application/providerSetup";
import { ProviderRow } from "@/features/providers/ui/ProviderRow";
import { EmbeddingModelSection, UtilityModelSection } from "@/features/providers/ui/RoleSections";
export function ProvidersPane() {
  const t = useT();
  const { data, isLoading, error, refetch } = useProviderConfigs();
  const unset = needsProviderSetup(data);
  const [cameUnset, setCameUnset] = useState(false);
  if (unset && !cameUnset) setCameUnset(true);
  const justConfigured = cameUnset && data !== undefined && !unset;

  return (
    <div {...stylex.props(vocab.column, gap.s6)}>
      {unset && <SystemMessage variant="info">{t("providers.setup.steps")}</SystemMessage>}
      {justConfigured && (
        <SystemMessage variant="success">{t("providers.setup.done")}</SystemMessage>
      )}
      <DataView
        items={data}
        isLoading={isLoading}
        failure={error}
        onRetry={refetch}
        skeletonCount={3}
        empty={{
          icon: "server",
          title: t("providers.empty"),
          sub: t("providers.empty.sub"),
        }}
      >
        {(rows) => (
          <RecordGroup variant="list">
            {rows.map((p) => (
              <ProviderRow key={p.id} p={p} />
            ))}
          </RecordGroup>
        )}
      </DataView>
      <div {...stylex.props(vocab.column, gap.s2)}>
        <SectionLabel>{t("providers.roles.heading")}</SectionLabel>
        <SettingsGroup>
          <UtilityModelSection />
          <EmbeddingModelSection />
        </SettingsGroup>
      </div>
    </div>
  );
}
