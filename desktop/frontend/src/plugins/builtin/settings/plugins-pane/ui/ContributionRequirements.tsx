import { pluginContract } from "@flame/runtime-contract/client/request";
import { useEffect, useRef, useState } from "react";
import type {
  PluginCollaborator,
  PluginContribution,
  PluginInstallation,
} from "@flame/runtime-contract/wire";
import { Button, ListRow, Metadata, PanelSection, RecordGroup } from "@flame/ui";
import { SystemMessage } from "@/ui";
import { useT } from "@/lib/i18n";
import type { PackageOperations } from "../application/packages";

type Read = {
  operations: PackageOperations;
  bindings?: PluginCollaborator[];
  error?: string;
  pending: boolean;
};
export function ContributionRequirements({
  installation,
  operations,
}: {
  installation: PluginInstallation;
  operations: PackageOperations;
}) {
  return (
    <>
      {installation.selected.contributions
        .filter((value) => value.requires.length > 0)
        .map((contribution) => (
          <RequirementBindings
            key={`${installation.selected.digest}/${contribution.id}`}
            installation={installation}
            contribution={contribution}
            operations={operations}
          />
        ))}
    </>
  );
}

function RequirementBindings({
  installation,
  contribution,
  operations,
}: {
  installation: PluginInstallation;
  contribution: PluginContribution;
  operations: PackageOperations;
}) {
  const t = useT();
  const [read, setRead] = useState<Read>();
  const controller = useRef<AbortController>(undefined);
  useEffect(() => () => controller.current?.abort(), [operations]);
  const current = read?.operations === operations ? read : undefined;
  async function inspect() {
    controller.current?.abort();
    const owned = new AbortController();
    controller.current = owned;
    const signal = AbortSignal.any([owned.signal, operations.signal]);
    if (signal.aborted) return;
    setRead((previous) => ({
      operations,
      bindings: previous?.operations === operations ? previous.bindings : undefined,
      pending: true,
    }));
    try {
      const result = await operations.track(
        operations.resolveRequirements(
          {
            installationId: installation.id,
            digest: installation.selected.digest,
            contribution: contribution.id,
          },
          signal,
        ),
      );
      if (!signal.aborted) setRead({ operations, bindings: result.data, pending: false });
    } catch (error) {
      if (!signal.aborted)
        setRead((previous) => ({
          operations,
          bindings: previous?.operations === operations ? previous.bindings : undefined,
          pending: false,
          error: error instanceof Error ? error.message : t("packages.requirements.failed"),
        }));
    }
  }
  return (
    <PanelSection
      title={`${contribution.id} · ${t("packages.requirements.title")}`}
      description={t("packages.requirements.future")}
      actions={
        <Button
          variant="soft"
          size="sm"
          pending={current?.pending}
          disabled={operations.signal.aborted}
          onClick={() => void inspect()}
        >
          {t("packages.requirements.inspect")}
        </Button>
      }
    >
      {current?.error && <SystemMessage variant="warning">{current.error}</SystemMessage>}
      <RecordGroup variant="list">
        {contribution.requires.map((required) => {
          const resolved = current?.bindings?.find((value) => value.id === required.id);
          return (
            <ListRow key={required.id}>
              <Metadata>
                {required.id} · {pluginContract(required.contract).reference} ·{" "}
                {t(
                  required.optional
                    ? "packages.requirements.optional"
                    : "packages.requirements.required",
                )}
              </Metadata>
              {resolved ? (
                resolved.bindings.length ? (
                  resolved.bindings.map((binding) => (
                    <Metadata key={`${binding.installationId}/${binding.contribution}`}>
                      {binding.installationId} · {binding.contribution} · {binding.digest}
                    </Metadata>
                  ))
                ) : (
                  <Metadata>{t("packages.requirements.absent")}</Metadata>
                )
              ) : (
                <Metadata>{t("packages.requirements.unread")}</Metadata>
              )}
            </ListRow>
          );
        })}
      </RecordGroup>
    </PanelSection>
  );
}
