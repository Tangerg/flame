import { pluginContract } from "@flame/runtime-client";
import { useLayoutEffect, useRef, useState } from "react";
import { type MutationPromise } from "@flame/runtime-client";
import { mutationSettlementIsUnknown } from "@flame/runtime-client";
import { type PluginInstallation, type PluginProviderSelection } from "@flame/runtime-client/wire";
import { Button, PanelSection, Select, Toolbar } from "@flame/ui";
import { DataView } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { SettingRow } from "@/ui/settings/SettingRow";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
import { useT } from "@/lib/i18n/index";
import { usePluginProviders, type PackageOperations } from "@/features/plugins/adapters/packages";
type Candidate = { installation: PluginInstallation; contribution: string; server: string };

const candidateKey = (candidate: Candidate) =>
  JSON.stringify([candidate.installation.id, candidate.contribution]);

export function ProviderSelections({
  installations,
  operations,
}: {
  installations: PluginInstallation[];
  operations: PackageOperations;
}) {
  const t = useT();
  const providers = usePluginProviders();
  const roles = new Set((providers.data ?? []).map((value) => value.role));
  for (const installation of installations)
    for (const contribution of installation.selected.contributions) {
      if (pluginContract(contribution.contract).composition === "selection")
        roles.add(pluginContract(contribution.contract).reference);
      for (const required of contribution.requires)
        if (pluginContract(required.contract).composition === "selection")
          roles.add(pluginContract(required.contract).reference);
    }
  return (
    <PanelSection
      title={t("packages.providers.title")}
      description={t("packages.providers.future")}
      actions={
        <Button
          variant="soft"
          size="sm"
          pending={providers.isFetching}
          onClick={() => void providers.refetch()}
        >
          {t("packages.providers.refresh")}
        </Button>
      }
    >
      {roles.size > 0 ? (
        <SettingsGroup>
          {[...roles].sort().map((role) => (
            <ProviderRole
              key={role}
              role={role}
              current={(providers.data ?? []).find((value) => value.role === role)}
              installations={installations}
              operations={operations}
              refresh={providers.refetch}
            />
          ))}
        </SettingsGroup>
      ) : (
        <DataView
          items={providers.data ?? []}
          isLoading={providers.isLoading}
          failure={providers.error}
          onRetry={providers.refetch}
          empty={{ icon: "blocks", title: t("packages.providers.none") }}
        >
          {() => null}
        </DataView>
      )}
    </PanelSection>
  );
}

function ProviderRole({
  role,
  current,
  installations,
  operations,
  refresh,
}: {
  role: string;
  current?: PluginProviderSelection;
  installations: PluginInstallation[];
  operations: PackageOperations;
  refresh: () => Promise<unknown>;
}) {
  const t = useT();
  const candidates: Candidate[] = installations.flatMap((installation) =>
    installation.selected.contributions
      .filter(
        (value) =>
          pluginContract(value.contract).composition === "selection" &&
          pluginContract(value.contract).reference === role,
      )
      .map((value) => ({ installation, contribution: value.id, server: value.server })),
  );
  type State = {
    operations: PackageOperations;
    draft: string | null;
    busy: boolean;
    failure?: string;
    unknown?: MutationPromise<PluginProviderSelection>;
  };
  const [state, setState] = useState<State>({ operations, draft: null, busy: false });
  const { draft, busy, failure, unknown } =
    state.operations === operations
      ? state
      : { draft: null, busy: false, failure: undefined, unknown: undefined };
  const update = (change: Partial<Omit<State, "operations">>) =>
    setState((previous) => ({
      ...(previous.operations === operations ? previous : { operations, draft: null, busy: false }),
      ...change,
      operations,
    }));
  const command = useRef<{
    operations: PackageOperations;
    mutation: MutationPromise<PluginProviderSelection>;
  }>(undefined);
  const observer = useRef<{ operations: PackageOperations; controller: AbortController }>(
    undefined,
  );
  useLayoutEffect(() => {
    const owned = { operations, controller: new AbortController() };
    observer.current = owned;
    return () => {
      owned.controller.abort();
      if (observer.current === owned) observer.current = undefined;
    };
  }, [operations]);
  const observationSignal = () => {
    const owned = observer.current;
    return owned?.operations === operations
      ? AbortSignal.any([owned.controller.signal, operations.signal])
      : undefined;
  };
  const candidate = candidates.find((value) => candidateKey(value) === draft);
  const chosen = candidates.find(
    (value) =>
      value.installation.id === current?.installationId &&
      value.contribution === current.contribution,
  );
  const active =
    chosen?.installation.enabled &&
    !chosen.installation.disabledServers.includes(chosen.server) &&
    chosen.installation.realization.type !== "releaseUnavailable" &&
    !chosen.installation.realization.unavailableBackends?.includes(chosen.server);
  async function observe(mutation: MutationPromise<PluginProviderSelection>, signal: AbortSignal) {
    command.current = { operations, mutation };
    update({ busy: true, failure: undefined });
    try {
      await operations.track(mutation);
      if (signal.aborted) return;
      command.current = undefined;
      update({ unknown: undefined, draft: null });
      await refresh();
    } catch (error) {
      if (signal.aborted) return;
      if (mutationSettlementIsUnknown(error)) update({ unknown: mutation });
      else {
        command.current = undefined;
        update({ unknown: undefined });
      }
      update({ failure: error instanceof Error ? error.message : t("packages.providers.failed") });
    } finally {
      if (!signal.aborted) update({ busy: false });
    }
  }
  const select = () => {
    if (!candidate || command.current?.operations === operations || operations.signal.aborted)
      return;
    const signal = observationSignal();
    if (!signal) return;
    try {
      const mutation = operations.selectProvider(
        { role, installationId: candidate.installation.id, contribution: candidate.contribution },
        signal,
      );
      void observe(mutation, signal);
    } catch (error) {
      update({ failure: error instanceof Error ? error.message : t("packages.providers.failed") });
    }
  };
  return (
    <SettingRow
      label={role}
      sub={
        current
          ? t(active ? "packages.providers.current" : "packages.providers.unavailable", {
              name: chosen?.installation.selected.name ?? current.installationId,
              contribution: current.contribution,
            })
          : t("packages.providers.unselected")
      }
    >
      <Toolbar label={role}>
        <Select
          label={t("packages.providers.choose")}
          value={draft}
          options={candidates.map((value) => ({
            value: candidateKey(value),
            label: `${value.installation.selected.name} · ${value.contribution}${value.installation.enabled && !value.installation.disabledServers.includes(value.server) ? "" : ` · ${t("packages.disabled")}`}`,
          }))}
          onValueChange={(draft) => update({ draft })}
          disabled={busy || !!unknown}
        />
        <Button
          variant="soft"
          pending={busy}
          disabled={!candidate || !!unknown || operations.signal.aborted}
          onClick={select}
        >
          {t("packages.providers.apply")}
        </Button>
      </Toolbar>
      {failure && <SystemMessage variant="warning">{failure}</SystemMessage>}
      {unknown && (
        <SystemMessage variant="warning">
          {t("packages.providers.unknown", { id: unknown.idempotencyKey })}
          <Button
            pending={busy}
            disabled={operations.signal.aborted}
            onClick={() => {
              try {
                const signal = observationSignal();
                if (!signal) return;
                void observe(unknown.retry({ signal }), signal);
              } catch (error) {
                update({
                  failure: error instanceof Error ? error.message : t("packages.providers.failed"),
                });
              }
            }}
          >
            {t("packages.providers.reconcile")}
          </Button>
        </SystemMessage>
      )}
    </SettingRow>
  );
}
