import { DataView, Surface } from "@/ui";
import { useT } from "@/lib/i18n";
import { useProviderConfigs } from "../application/providerConfig";
import { ProviderRow } from "./ProviderRow";
import { EmbeddingModelSection, UtilityModelSection } from "./RoleSections";

export function ProvidersPane() {
  const t = useT();
  const { data, isLoading, isError, refetch } = useProviderConfigs();

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3">
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
          <Surface inset="xs" className="flex flex-col gap-1">
            {rows.map((p) => (
              <ProviderRow key={p.id} p={p} />
            ))}
          </Surface>
        )}
      </DataView>
    </div>
  );
}
