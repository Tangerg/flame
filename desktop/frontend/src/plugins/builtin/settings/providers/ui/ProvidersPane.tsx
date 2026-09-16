import * as stylex from "@stylexjs/stylex";
import { DataView, gap, Surface, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { useProviderConfigs } from "../application/providerConfig";
import { ProviderRow } from "./ProviderRow";
import { EmbeddingModelSection, UtilityModelSection } from "./RoleSections";
import { SettingsGroup } from "../../kit";

export function ProvidersPane() {
  const t = useT();
  const { data, isLoading, error, refetch } = useProviderConfigs();

  return (
    <div {...stylex.props(vocab.column, gap.s6)}>
      <SettingsGroup>
        <UtilityModelSection />
        <EmbeddingModelSection />
      </SettingsGroup>
      <DataView
        items={data}
        isLoading={isLoading}
        failure={error}
        onRetry={refetch}
        skeletonCount={3}
        empty={{
          icon: "spark",
          title: t("providers.empty"),
          sub: t("providers.empty.sub"),
        }}
      >
        {(rows) => (
          <Surface
            variant="group"
            inset="xs"
            className={stylex.props(vocab.column, gap.s1).className}
          >
            {rows.map((p) => (
              <ProviderRow key={p.id} p={p} />
            ))}
          </Surface>
        )}
      </DataView>
    </div>
  );
}
