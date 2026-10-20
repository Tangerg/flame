import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { DataView, SectionLabel, SystemMessage } from "@/ui";
import { gap, RecordGroup, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { SettingsGroup } from "../../settings/kit";
import { useProviderConfigs } from "../application/providerConfig";
import { needsProviderSetup } from "../application/providerSetup";
import { ProviderRow } from "./ProviderRow";
import { EmbeddingModelSection, UtilityModelSection } from "./RoleSections";

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
