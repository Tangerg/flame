import * as stylex from "@stylexjs/stylex";
import { DataView, Surface } from "@/ui";
import { useT } from "@/lib/i18n";
import { useProviderConfigs } from "../application/providerConfig";
import { ProviderRow } from "./ProviderRow";
import { EmbeddingModelSection, UtilityModelSection } from "./RoleSections";
import { settingStyles as ss } from "../../kit/settingStyles";

export function ProvidersPane() {
  const t = useT();
  const { data, isLoading, isError, refetch } = useProviderConfigs();

  return (
    <div {...stylex.props(ss.pane)}>
      <div {...stylex.props(ss.stack)}>
        <UtilityModelSection />
        <EmbeddingModelSection />
      </div>
      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        onRetry={refetch}
        skeletonCount={3}
        empty={{
          icon: "spark",
          title: t("providers.empty"),
          sub: t("providers.empty.sub"),
        }}
      >
        {(rows) => (
          <Surface inset="xs" className={stylex.props(ss.stackRows).className}>
            {rows.map((p) => (
              <ProviderRow key={p.id} p={p} />
            ))}
          </Surface>
        )}
      </DataView>
    </div>
  );
}
