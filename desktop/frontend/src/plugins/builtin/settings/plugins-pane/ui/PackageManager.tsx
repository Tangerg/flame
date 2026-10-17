import { pluginContract } from "@flame/runtime-contract/client/request";
import { parseJSON } from "@flame/runtime-contract/client/json";
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
import { DataView, SystemMessage, TextEditorDialog } from "@/ui";
import {
  ActionMenu,
  Badge,
  Button,
  CodeBlock,
  Disclosure,
  Facts,
  ListRow,
  PanelSection,
  RecordGroup,
  RecordHeading,
  TextField,
  Toolbar,
} from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { useAsyncFeedback } from "../../kit";
import { packageOperations, usePackageRealization, usePackages } from "../application/packages";
import { ProviderSelections } from "./ProviderSelections";
import { ContributionRequirements } from "./ContributionRequirements";
import { PointInspection } from "./PointInspection";
import { PendingCommands } from "./PendingCommands";
import { pointMetadata } from "../adapters/pointContracts";

const styles = stylex.create({
  install: {
    display: "grid",
    gridTemplateColumns: {
      default: "minmax(0,1fr) auto",
      "@container (max-width: 440px)": "minmax(0,1fr)",
    },
    alignItems: "start",
    gap: space.s2,
    minWidth: 0,
  },
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
    <>
      <PanelSection title={t("packages.installTitle")} description={t("packages.trust")}>
        <div {...stylex.props(styles.install)}>
          <TextField
            aria-label={t("packages.source")}
            placeholder={t("packages.source")}
            value={source}
            onChange={(event) => setSource(event.target.value)}
            pending={busy}
          />
          <Button
            variant="primary"
            pending={busy}
            disabled={!source || !operations}
            onClick={() => void install()}
          >
            {t("packages.install")}
          </Button>
        </div>
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
      </PanelSection>
      {operations && (
        <ProviderSelections installations={catalog.data ?? []} operations={operations} />
      )}
      <PanelSection title={t("packages.installedTitle")}>
        <DataView
          items={catalog.data ?? []}
          isLoading={catalog.isLoading}
          failure={catalog.error}
          onRetry={catalog.refetch}
          empty={{ icon: "blocks", title: t("packages.empty") }}
        >
          {(rows) => (
            <RecordGroup variant="list">
              {operations &&
                rows.map((installation) => (
                  <PackageRow
                    key={installation.id}
                    installation={installation}
                    operations={operations}
                    refresh={catalog.refetch}
                  />
                ))}
            </RecordGroup>
          )}
        </DataView>
      </PanelSection>
      {operations && <PendingCommands operations={operations} />}
    </>
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
  const { feedback, reset, run: runFeedback } = useAsyncFeedback(operations.signal);
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
        contract: pluginContract(value.contract).reference,
        composition: pluginContract(value.contract).composition,
        requires: value.requires.map((required) => ({
          id: required.id,
          contract: pluginContract(required.contract).reference,
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
        const changes = z.record(z.string(), z.unknown()).parse(parseJSON(edit.value));
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
    <ListRow>
      <RecordHeading
        title={installation.selected.name}
        description={installation.selected.version}
        status={
          <Badge tone={enabled ? "success" : "neutral"}>
            {t(enabled ? "packages.enabled" : "packages.disabled")}
          </Badge>
        }
        actions={
          <Toolbar label={t("packages.actions")}>
            <Button
              variant="soft"
              size="sm"
              pending={busy}
              onClick={() => {
                reset();
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
            </Button>
            <ActionMenu
              label={t("packages.actions")}
              disabled={busy}
              items={[
                {
                  label: t(enablementLabel),
                  onSelect: () =>
                    void run(t(enablementLabel), () =>
                      operations.setEnablement({
                        installationId: installation.id,
                        enabled: !enabled,
                      }),
                    ),
                },
                {
                  label: t("packages.stage"),
                  onSelect: () => {
                    reset();
                    setEdit({
                      type: "stage",
                      target: { installationId: installation.id },
                      value: installation.source,
                    });
                  },
                },
                ...(installation.staged
                  ? [
                      {
                        label: t("packages.select"),
                        onSelect: () =>
                          void run(t("packages.select"), () =>
                            operations.select({
                              installationId: installation.id,
                              digest: installation.staged!.digest,
                            }),
                          ),
                      },
                    ]
                  : []),
                {
                  label: t("packages.uninstall"),
                  tone: "negative",
                  onSelect: () =>
                    void run(t("packages.uninstall"), () => operations.uninstall(installation.id)),
                },
              ]}
            />
          </Toolbar>
        }
      />
      <Disclosure title={t("packages.details")}>
        <Facts
          values={[
            [t("packages.digest"), request.digest],
            [t("packages.source"), installation.source],
          ]}
        />
        <ContributionRequirements installation={installation} operations={operations} />
        <PointInspection installation={installation} operations={operations} />
        <CodeBlock label={t("packages.details")}>{preview}</CodeBlock>
      </Disclosure>
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
      {!edit && error && <SystemMessage variant="warning">{error}</SystemMessage>}
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
        error={error}
        onSave={() => void save()}
        font="mono"
      />
    </ListRow>
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
