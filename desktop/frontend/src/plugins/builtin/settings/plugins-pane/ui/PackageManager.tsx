import { parseReviewedJSON } from "@flame/runtime-contract/client/json";
import { checkRequest } from "@flame/runtime-contract/client/request";
import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { z } from "zod";
import type {
  PluginDiagnostic,
  PluginInstallation,
  PluginRealization,
  PluginReleaseRequest,
  PluginRequest,
} from "@flame/runtime-contract/wire";
import { useT } from "@/lib/i18n";
import { DataView, PillButton, SystemMessage, TextEditorDialog } from "@/ui";
import { TextField, vocab } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { SettingsGroup, useAsyncFeedback } from "../../kit";
import { settingStyles as ss } from "../../kit/settingStyles";
import { packageOperations, usePackageRealization, usePackages } from "../application/packages";
import { ProviderSelections } from "./ProviderSelections";
import { ContributionRequirements } from "./ContributionRequirements";
import { PointInspection } from "./PointInspection";
import { pointMetadata } from "../adapters/pointContracts";
import { PackageActions } from "./PackageActions";

const styles = stylex.create({
  row: { padding: space.s4, display: "flex", flexDirection: "column", gap: space.s3 },
  preview: { whiteSpace: "pre-wrap", overflowWrap: "anywhere", maxHeight: 240, overflow: "auto" },
});

type PackageEdit =
  | { type: "configure"; target: PluginReleaseRequest; value: string }
  | { type: "stage"; target: PluginRequest; value: string };

const EDIT_LABEL: Record<PackageEdit["type"], string> = {
  configure: "packages.configure",
  stage: "packages.stage",
};

export function PackageManager() {
  const t = useT();
  const catalog = usePackages();
  const realization = usePackageRealization((state) => state.failure);
  const [source, setSource] = useState("");
  const [installed, setInstalled] = useState<{ signal: AbortSignal; name: string }>();
  const operations = packageOperations.peek();
  const { feedback, run } = useAsyncFeedback(operations?.signal);
  const busy = feedback.state === "busy";
  const error = feedback.state === "error" ? feedback.reason : "";
  const install = () => {
    if (!operations) return;
    return run(async () => {
      const installation = await operations.install({ source });
      if (operations.signal.aborted) return { ok: true };
      setInstalled({ signal: operations.signal, name: installation.selected.name });
      setSource("");
      await catalog.refetch();
      return { ok: true };
    }, t("packages.install"));
  };
  return (
    <SettingsGroup>
      <div {...stylex.props(styles.row)}>
        <SystemMessage>{t("packages.trust")}</SystemMessage>
        <TextField
          aria-label={t("packages.source")}
          placeholder={t("packages.source")}
          value={source}
          onChange={(event) => setSource(event.target.value)}
          pending={busy}
        />
        <PillButton pending={busy} disabled={!source || !operations} onClick={() => void install()}>
          {t("packages.install")}
        </PillButton>
        {error && <SystemMessage variant="warning">{error}</SystemMessage>}
        {installed && installed.signal === operations?.signal && (
          <SystemMessage>{t("packages.installed", { name: installed.name })}</SystemMessage>
        )}
        {realization && (
          <SystemMessage
            variant="warning"
            action={{ label: t("common.retry"), onClick: realization.retry }}
          >
            {t("packages.realizationFailed", { reason: realization.reason })}
          </SystemMessage>
        )}
      </div>
      {operations && (
        <ProviderSelections installations={catalog.data ?? []} operations={operations} />
      )}
      <DataView
        items={catalog.data ?? []}
        isLoading={catalog.isLoading}
        failure={catalog.error}
        onRetry={catalog.refetch}
        empty={{ icon: "blocks", title: t("packages.empty") }}
      >
        {(rows) => (
          <>
            {operations &&
              rows.map((installation) => (
                <PackageRow
                  key={installation.id}
                  installation={installation}
                  operations={operations}
                  refresh={catalog.refetch}
                />
              ))}
          </>
        )}
      </DataView>
    </SettingsGroup>
  );
}
function PackageRow({
  installation,
  operations,
  refresh,
}: {
  installation: PluginInstallation;
  operations: ReturnType<typeof packageOperations.get>;
  refresh: () => Promise<unknown>;
}) {
  const t = useT();
  const [edit, setEdit] = useState<PackageEdit>();
  const { feedback, run: runFeedback } = useAsyncFeedback(operations.signal);
  const busy = feedback.state === "busy";
  const error = feedback.state === "error" ? feedback.reason : "";
  const request = { installationId: installation.id, digest: installation.selected.digest };
  const enabled = installation.enabled;
  const enablementLabel = enabled ? "packages.disable" : "packages.enable";
  const run = (label: string, operation: () => Promise<unknown>) =>
    runFeedback(async () => {
      await operation();
      if (operations.signal.aborted) return { ok: true };
      setEdit(undefined);
      await refresh();
      return { ok: true };
    }, label);
  const preview = JSON.stringify(
    {
      contributions: installation.selected.contributions.map((value) => ({
        id: value.id,
        server: value.server,
        export: value.export,
        contract: value.contract.reference,
        composition: value.contract.composition,
        requires: value.requires.map((required) => ({
          id: required.id,
          contract: required.contract.reference,
          optional: required.optional,
        })),
      })),
      components: installation.selected.components,
      points: installation.selected.points.map((value) => ({
        id: value.id,
        ...pointMetadata(value.contract),
      })),
      pointValues: installation.selected.pointValues.map((value) => ({
        id: value.id,
        contract: pointMetadata(value.contract).reference,
      })),
      servers: installation.selected.servers,
      inputs: installation.selected.inputs,
      skills: installation.selected.skills,
      views: installation.selected.views,
      renderers: installation.selected.renderers,
      inputStates: installation.inputStates,
      disabledServers: installation.disabledServers,
      disabledSkills: installation.disabledSkills,
    },
    null,
    2,
  );
  const save = () => {
    if (!edit) return;
    return run(t(EDIT_LABEL[edit.type]), async () => {
      if (edit.type === "configure") {
        const changes = z.record(z.string(), z.unknown()).parse(parseReviewedJSON(edit.value));
        for (const field of Object.keys(edit.target)) {
          if (Object.hasOwn(changes, field))
            throw new Error(t("packages.reviewedField", { field }));
        }
        return operations.configure(
          checkRequest("plugins.configure", { ...edit.target, ...changes }),
        );
      }
      return operations.stage({ ...edit.target, source: edit.value });
    });
  };
  return (
    <div {...stylex.props(styles.row)}>
      <div {...stylex.props(ss.label, vocab.truncate)} title={installation.selected.name}>
        {installation.selected.version
          ? `${installation.selected.name} · ${installation.selected.version}`
          : installation.selected.name}
      </div>
      <div {...stylex.props(vocab.muted, vocab.truncate)} title={request.digest}>
        {request.digest}
      </div>
      <div {...stylex.props(vocab.muted)}>
        {t(enabled ? "packages.enabled" : "packages.disabled")}
      </div>
      {installation.presentation === "admitted" && installation.selected.actions.length > 0 && (
        <PackageActions
          key={installation.selected.digest}
          installation={installation}
          operations={operations}
        />
      )}
      <div {...stylex.props(ss.lineWrap)}>
        <PillButton
          size="sm"
          pending={busy}
          onClick={() => {
            setEdit({
              type: "configure",
              target: request,
              value: JSON.stringify(
                { valueChanges: {}, serverChanges: {}, skillChanges: {} },
                null,
                2,
              ),
            });
          }}
        >
          {t("packages.configure")}
        </PillButton>
        <PillButton
          size="sm"
          pending={busy}
          onClick={() =>
            void run(t(enablementLabel), () =>
              operations.setEnablement({
                installationId: installation.id,
                enabled: !enabled,
              }),
            )
          }
        >
          {t(enablementLabel)}
        </PillButton>
        <PillButton
          size="sm"
          pending={busy}
          onClick={() => {
            setEdit({
              type: "stage",
              target: { installationId: installation.id },
              value: installation.source,
            });
          }}
        >
          {t("packages.stage")}
        </PillButton>
        {installation.staged && (
          <PillButton
            size="sm"
            pending={busy}
            onClick={() =>
              void run(t("packages.select"), () =>
                operations.select({
                  installationId: installation.id,
                  digest: installation.staged!.digest,
                }),
              )
            }
          >
            {t("packages.select")}
          </PillButton>
        )}
        <PillButton
          size="sm"
          pending={busy}
          onClick={() =>
            void run(t("packages.uninstall"), () => operations.uninstall(installation.id))
          }
        >
          {t("packages.uninstall")}
        </PillButton>
      </div>
      <ContributionRequirements installation={installation} operations={operations} />
      <PointInspection installation={installation} operations={operations} />
      <pre {...stylex.props(styles.preview)}>{preview}</pre>
      {realizationMessages(t, installation.realization).map((message) => (
        <SystemMessage key={message} variant="warning">
          {message}
        </SystemMessage>
      ))}
      {installation.selected.diagnostics.map((diagnostic, index) => {
        const message = describeDiagnostic(t, diagnostic);
        return (
          <SystemMessage
            key={`${index}:${diagnostic.code}:${diagnostic.component.type}`}
            variant="warning"
          >
            {message}
          </SystemMessage>
        );
      })}
      {error && <SystemMessage variant="warning">{error}</SystemMessage>}
      <TextEditorDialog
        open={edit !== undefined}
        onOpenChange={(open) => {
          if (!open) setEdit(undefined);
        }}
        title={edit ? t(EDIT_LABEL[edit.type]) : ""}
        closeLabel={t("common.close")}
        label={t("packages.input")}
        value={edit?.value ?? ""}
        onChange={(value) => setEdit((current) => current && { ...current, value })}
        cancelLabel={t("common.cancel")}
        saveLabel={t("packages.save")}
        savingLabel={t("packages.loading")}
        busy={busy}
        onSave={() => void save()}
        font="mono"
      />
    </div>
  );
}

type Translate = ReturnType<typeof useT>;

function describeDiagnostic(t: Translate, diagnostic: PluginDiagnostic): string {
  const { component } = diagnostic;
  const name = "name" in component ? component.name : "";
  const label = t(`packages.component.${component.type}`, { name });
  return t(`packages.diagnostic.${diagnostic.code}`, { component: label });
}

function realizationMessages(t: Translate, realization: PluginRealization): string[] {
  if (realization.type === "releaseUnavailable") return [t("packages.releaseUnavailable")];
  return (realization.unavailableBackends ?? []).map((name) =>
    t("packages.backendUnavailable", { name }),
  );
}
