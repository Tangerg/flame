import { backendAvailable } from "../adapters/workflowAuthors";
import { tupleKey } from "@/lib/tupleKey";
import { pluginContract } from "@flame/runtime-client";
import { useState, useSyncExternalStore } from "react";
import { type PluginInstallation, type PluginProviderSelection } from "@flame/runtime-client/wire";
import { Button, PanelSection, Select, Toolbar } from "@flame/ui";
import { DataError, DataView } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { SettingRow } from "@/ui/settings/SettingRow";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
import { useT } from "@/lib/i18n/index";
import { failureText } from "@/lib/rpcErrors";
import { usePluginProviders, type PackageOperations } from "@/features/plugins/adapters/packages";
type Candidate = { installation: PluginInstallation; contribution: string; server: string };

const candidateKey = (candidate: Candidate) =>
  tupleKey(candidate.installation.id, candidate.contribution);

export function ProviderSelections({
  installations,
  operations,
}: {
  installations: PluginInstallation[];
  operations: PackageOperations;
}) {
  const t = useT();
  const providers = usePluginProviders();
  const hasRead = providers.data !== undefined;
  const roles = new Set((providers.data ?? []).map((value) => value.role));
  for (const installation of installations)
    for (const contribution of installation.selected.contributions) {
      for (const role of contribution.selectionRoles ?? []) roles.add(role);
      for (const required of contribution.requires)
        if (pluginContract(required.contract).composition === "selection")
          roles.add(pluginContract(required.contract).reference);
    }
  return (
    <PanelSection
      title={t("packages.providers.title")}
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
        <>
          {providers.error != null && (
            <DataError
              failure={providers.error}
              sub={hasRead ? t("packages.providers.stale") : undefined}
              size="compact"
            />
          )}
          <SettingsGroup>
            {[...roles].sort().map((role) => (
              <ProviderRole
                key={role}
                role={role}
                hasRead={hasRead}
                current={(providers.data ?? []).find((value) => value.role === role)}
                installations={installations}
                operations={operations}
              />
            ))}
          </SettingsGroup>
        </>
      ) : (
        <DataView
          items={providers.data ?? []}
          isLoading={providers.isPending}
          failure={providers.error}
          onRetry={providers.refetch}
          empty={{ icon: "plug", title: t("packages.providers.none") }}
        >
          {() => null}
        </DataView>
      )}
    </PanelSection>
  );
}

function ProviderRole({
  role,
  hasRead,
  current,
  installations,
  operations,
}: {
  role: string;
  hasRead: boolean;
  current?: PluginProviderSelection;
  installations: PluginInstallation[];
  operations: PackageOperations;
}) {
  const t = useT();
  const candidates: Candidate[] = installations.flatMap((installation) =>
    installation.selected.contributions
      .filter((value) => value.selectionRoles?.includes(role))
      .map((value) => ({ installation, contribution: value.id, server: value.server })),
  );
  const commands = operations.providerSelections;
  const state = useSyncExternalStore(commands.subscribe, () => commands.snapshot(role));
  const [draftChoice, setDraftChoice] = useState<{ source: typeof commands; value: string | null }>();
  const draft = draftChoice?.source === commands ? draftChoice.value : null;
  const clearDraft = () =>
    setDraftChoice((previous) => (previous?.source === commands ? undefined : previous));
  const busy = state?.kind === "observing";
  const unconfirmed = state?.kind === "unconfirmed" ? state : undefined;
  const failure =
    state && state.kind !== "observing"
      ? failureText(state.error, t("packages.providers.failed"))
      : undefined;
  const candidate = candidates.find((value) => candidateKey(value) === draft);
  const chosen = candidates.find(
    (value) =>
      value.installation.id === current?.installationId &&
      value.contribution === current.contribution,
  );
  const active = chosen && backendAvailable(chosen.installation, chosen.server);
  const chosenKey = chosen ? candidateKey(chosen) : null;
  async function select() {
    if (!candidate) return;
    const accepted = await commands.select({
      role,
      installationId: candidate.installation.id,
      contribution: candidate.contribution,
    });
    if (accepted) clearDraft();
  }
  return (
    <SettingRow
      label={role}
      // The select shows a usable selection itself; only an unread or unusable one needs a note.
      sub={
        !hasRead
          ? t("packages.providers.unread")
          : current && !active
            ? t("packages.providers.unavailable", {
                name: chosen?.installation.selected.name ?? current.installationId,
                contribution: current.contribution,
              })
            : undefined
      }
    >
      <Toolbar label={role}>
        <Select
          label={t("packages.providers.choose")}
          value={draft ?? chosenKey}
          options={candidates.map((value) => ({
            value: candidateKey(value),
            label: `${value.installation.selected.name} · ${value.contribution}${value.installation.enabled && !value.installation.disabledServers.includes(value.server) ? "" : ` · ${t("packages.disabled")}`}`,
          }))}
          onValueChange={(value) => setDraftChoice({ source: commands, value })}
          disabled={busy || !!unconfirmed || operations.signal.aborted}
        />
        {candidate && draft !== chosenKey && (
          <Button
            variant="soft"
            pending={busy}
            disabled={!!unconfirmed || operations.signal.aborted}
            onClick={() => void select()}
          >
            {t("packages.providers.apply")}
          </Button>
        )}
      </Toolbar>
      {failure && <SystemMessage variant="warning">{failure}</SystemMessage>}
      {unconfirmed && (
        <SystemMessage variant="warning">
          {t("packages.providers.unknown", { id: unconfirmed.idempotencyKey })}
          <Button
            pending={busy}
            disabled={operations.signal.aborted}
            onClick={async () => {
              if (await commands.check(role, unconfirmed)) clearDraft();
            }}
          >
            {t("packages.providers.reconcile")}
          </Button>
        </SystemMessage>
      )}
    </SettingRow>
  );
}
