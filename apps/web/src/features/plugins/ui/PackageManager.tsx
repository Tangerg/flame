import { useInstalledPlugins } from "@/features/plugins/context";
import { checkRequest, pluginContract, RpcProtocolError } from "@flame/runtime-client";
import { parseJSON } from "@frame/sdk/json";
import { useRef, useState, type RefObject } from "react";
import * as stylex from "@stylexjs/stylex";
import { z } from "zod";
import {
  type PluginDiagnostic,
  type PluginInstallation,
  type PluginRealization,
  type PluginReleaseRequest,
  type PluginRequest,
} from "@flame/runtime-client/wire";
import { useT, type MessageKey, type Translate } from "@/lib/i18n/index";
import { useCodeActionLabels } from "@/ui/index";
import { DataError, DataView, SkeletonList } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { TextEditorDialog } from "@/ui/index";
import {
  ActionMenu,
  Badge,
  Button,
  CodeBlock,
  Disclosure,
  Facts,
  Hang,
  ListRow,
  PanelSection,
  RecordGroup,
  RecordHeading,
  TextField,
  Toolbar,
} from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { useAsyncFeedback } from "@/lib/useAsyncFeedback";
import { failureText } from "@/lib/rpcErrors";
import {
  type PackageOperations,
  type PackageRealizationFailure,
  usePackages,
} from "@/features/plugins/adapters/packages";
import { ProviderSelections } from "@/features/plugins/ui/ProviderSelections";
import { ContributionRequirements } from "@/features/plugins/ui/ContributionRequirements";
import { PointInspection } from "@/features/plugins/ui/PointInspection";
import { PendingCommands } from "@/features/plugins/ui/PendingCommands";
import { pointMetadata } from "@/features/plugins/adapters/pointContracts";
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

// The dialog presents the identity captured when the edit opened, so a live
// catalog replacement cannot relabel a task that still submits the old target.
type PackageEdit = { name: string; version?: string; value: string; validationError?: string } & (
  { type: "configure"; target: PluginReleaseRequest } | { type: "stage"; target: PluginRequest }
);

const EDIT_LABEL: Record<PackageEdit["type"], MessageKey> = {
  configure: "packages.configure",
  stage: "packages.stage",
};

const EDIT_INPUT: Record<PackageEdit["type"], MessageKey> = {
  configure: "packages.configureInput",
  stage: "packages.source",
};

const EDIT_SAVE: Record<PackageEdit["type"], MessageKey> = {
  configure: "packages.save",
  stage: "packages.stage",
};

function InstallFeedback({
  error,
  installed,
  realization,
}: {
  error: string;
  installed: string | undefined;
  realization: ReturnType<ReturnType<typeof useInstalledPlugins>["useFailure"]>;
}) {
  const t = useT();
  return (
    <>
      {error && <SystemMessage variant="warning">{error}</SystemMessage>}
      {installed && <SystemMessage>{t("packages.installed", { name: installed })}</SystemMessage>}
      {realization && <RealizationFailure failure={realization} />}
    </>
  );
}

// A failed refresh keeps the listed packages and offers a retry above them.
function CatalogRefreshFailure({
  catalog,
  focusTarget,
}: {
  catalog: ReturnType<typeof usePackages>;
  focusTarget: RefObject<HTMLSpanElement | null>;
}) {
  const t = useT();
  if (catalog.isFetching)
    return <SkeletonList count={1} variant="compact" label={t("common.loading")} />;
  return (
    <div role="status">
      <DataError
        failure={catalog.error}
        focusTarget={focusTarget}
        title={t("packages.refreshError.title")}
        sub={t("packages.refreshError.sub")}
        size="compact"
        unsupported={{
          icon: "blocks",
          sub: t("packages.refreshError.sub"),
          size: "compact",
        }}
        onRetry={catalog.refetch}
      />
    </div>
  );
}

export function PackageManager() {
  const packages = useInstalledPlugins();
  const t = useT();
  const catalog = usePackages();
  const catalogTitle = useRef<HTMLSpanElement>(null);
  const retained = catalog.data !== undefined && catalog.data.length > 0;
  const realization = packages.useFailure();
  const operations = packages.useOperations();
  const [draft, setDraft] = useState({ signal: operations?.signal, source: "" });
  if (draft.signal !== operations?.signal) setDraft({ signal: operations?.signal, source: "" });
  const source = draft.source;
  const [installed, setInstalled] = useState<{ signal: AbortSignal; name: string }>();
  const { feedback, reset, run } = useAsyncFeedback(operations?.signal);
  const busy = feedback.state === "busy";
  const error = feedback.state === "error" ? feedback.reason : "";
  const install = () => {
    if (!operations) return;
    return run(async () => {
      const installation = await operations.install({ source });
      if (operations.signal.aborted) return { ok: true };
      setInstalled({ signal: operations.signal, name: installation.selected.name });
      setDraft({ signal: operations.signal, source: "" });
      await catalog.refetch();
      return { ok: true };
    }, t("packages.install"));
  };
  return (
    <>
      <PanelSection title={t("packages.installTitle")} description={t("packages.trust")}>
        <div {...stylex.props(styles.install)}>
          <Hang>
            <TextField
              aria-label={t("packages.source")}
              placeholder={t("packages.source")}
              value={source}
              onChange={(event) => {
                reset();
                setInstalled(undefined);
                setDraft({ signal: operations?.signal, source: event.target.value });
              }}
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              pending={busy}
            />
          </Hang>
          <Button
            variant="primary"
            pending={busy}
            aria-busy={busy || undefined}
            disabled={!source || !operations}
            onClick={() => void install()}
          >
            {t("packages.install")}
          </Button>
        </div>
        <InstallFeedback
          error={error}
          installed={installed?.signal === operations?.signal ? installed?.name : undefined}
          realization={realization}
        />
      </PanelSection>
      <PanelSection
        title={
          <span ref={catalogTitle} tabIndex={-1}>
            {t("packages.installedTitle")}
          </span>
        }
      >
        {retained && catalog.error != null && (
          <CatalogRefreshFailure catalog={catalog} focusTarget={catalogTitle} />
        )}
        <DataView
          items={catalog.data ?? []}
          isLoading={catalog.isPending}
          failure={retained ? undefined : catalog.error}
          onRetry={catalog.refetch}
          empty={{ icon: "blocks", title: t("packages.empty") }}
        >
          {(rows) => (
            <RecordGroup variant="list" hidden={!operations}>
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
      {operations && catalog.data && (
        <ProviderSelections installations={catalog.data} operations={operations} />
      )}
      {operations && <PendingCommands operations={operations} />}
    </>
  );
}

// The manifest projection shown for review: contracts by reference, plus the
// installation's own configuration state.
function manifestPreview(installation: PluginInstallation): string {
  const selected = installation.selected;
  return JSON.stringify(
    {
      contributions: selected.contributions.map((value) => ({
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
      components: selected.components,
      locales: selected.locales,
      points: selected.points.map((value) => ({
        id: value.id,
        ...pointMetadata(value.contract),
      })),
      pointValues: selected.pointValues.map((value) => ({
        id: value.id,
        contract: pointMetadata(value.contract).reference,
      })),
      servers: selected.servers,
      inputs: selected.inputs,
      skills: selected.skills,
      inputStates: installation.inputStates,
      disabledServers: installation.disabledServers,
      disabledSkills: installation.disabledSkills,
    },
    null,
    2,
  );
}

function editTitle(t: Translate, edit: PackageEdit): string {
  const action = t(EDIT_LABEL[edit.type]);
  return edit.version
    ? t("packages.editTitleVersion", { action, name: edit.name, version: edit.version })
    : t("packages.editTitle", { action, name: edit.name });
}

function editContext(t: Translate, edit: PackageEdit): string {
  return edit.type === "configure"
    ? t("packages.configureContext", { digest: edit.target.digest })
    : t("packages.stageContext");
}

function PackageRow({
  installation,
  operations,
  refresh,
}: {
  installation: PluginInstallation;
  operations: PackageOperations;
  refresh: () => Promise<unknown>;
}) {
  const t = useT();
  const codeLabels = useCodeActionLabels();
  const [edit, setEdit] = useState<PackageEdit>();
  const { feedback, reset, run: runFeedback } = useAsyncFeedback(operations.signal);
  const busy = feedback.state === "busy";
  const error = feedback.state === "error" ? feedback.reason : "";
  const request = { installationId: installation.id, digest: installation.selected.digest };
  const identity = { name: installation.selected.name, version: installation.selected.version };
  const enabled = installation.enabled;
  const { inputs, servers, skills } = installation.selected;
  const configurable = inputs.length + servers.length + skills.length > 0;
  const enablementLabel = enabled ? "packages.disable" : "packages.enable";
  const run = (label: string, operation: () => Promise<unknown>) =>
    runFeedback(async () => {
      await operation();
      if (operations.signal.aborted) return { ok: true };
      setEdit(undefined);
      await refresh();
      return { ok: true };
    }, label);
  const preview = manifestPreview(installation);
  const save = () => {
    if (!edit) return;
    if (edit.type === "stage")
      return run(t("packages.stage"), () =>
        operations.stage({ ...edit.target, source: edit.value }),
      );
    try {
      const changes = z.record(z.string(), z.unknown()).parse(parseJSON(edit.value));
      for (const field of Object.keys(edit.target)) {
        if (Object.hasOwn(changes, field)) throw new Error(t("packages.reviewedField", { field }));
      }
      const configuration = checkRequest("plugins.configure", { ...edit.target, ...changes });
      return run(t("packages.configure"), () => operations.configure(configuration));
    } catch (error) {
      const validationError = configurationErrorText(error, t);
      setEdit((current) => current && { ...current, validationError });
    }
  };
  return (
    <ListRow>
      <RecordHeading
        title={installation.selected.name}
        status={enabled ? undefined : <Badge tone="neutral">{t("packages.disabled")}</Badge>}
        actions={
          <Toolbar label={t("packages.actions")}>
            {configurable && (
              <Button
                variant="soft"
                size="sm"
                pending={busy}
                onClick={() => {
                  reset();
                  setEdit({
                    type: "configure",
                    target: request,
                    ...identity,
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
            )}
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
                      ...identity,
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
      <Disclosure
        title={identity.version || t("packages.details")}
        label={
          identity.version
            ? t("packages.details.label", { name: identity.name, version: identity.version })
            : undefined
        }
      >
        <Facts
          values={[
            [t("packages.digest"), request.digest],
            [t("packages.source.label"), installation.source],
          ]}
        />
        <ContributionRequirements installation={installation} operations={operations} />
        <PointInspection installation={installation} operations={operations} />
        <CodeBlock label={t("packages.manifest")} language="json" labels={codeLabels}>
          {preview}
        </CodeBlock>
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
        title={edit ? editTitle(t, edit) : ""}
        description={edit && editContext(t, edit)}
        closeLabel={t("common.close")}
        label={edit ? t(EDIT_INPUT[edit.type]) : ""}
        value={edit?.value ?? ""}
        onChange={(value) => {
          reset();
          setEdit((current) => current && { ...current, value, validationError: undefined });
        }}
        cancelLabel={t("common.cancel")}
        saveLabel={edit ? t(EDIT_SAVE[edit.type]) : ""}
        savingLabel={t("packages.loading")}
        busy={busy}
        error={edit?.validationError ?? error}
        invalid={edit?.validationError !== undefined}
        onSave={() => void save()}
        font="mono"
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
      />
    </ListRow>
  );
}

function configurationErrorText(error: unknown, t: Translate): string {
  if (error instanceof SyntaxError) return t("packages.configureJson");
  if (error instanceof z.ZodError) return t("packages.configureObject");
  if (error instanceof RpcProtocolError) {
    return error.violations
      .map(({ path, detail }) =>
        t("packages.configureField", {
          field: path.replace(/^plugins\.configure\.params\./, ""),
          reason: detail,
        }),
      )
      .join("\n");
  }
  return failureText(error, t("packages.configure"));
}

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

function RealizationFailure({ failure }: { failure: PackageRealizationFailure }) {
  const t = useT();
  return (
    <SystemMessage
      variant="warning"
      action={failure.retry ? { label: t("common.retry"), onClick: failure.retry } : undefined}
    >
      {t("packages.realizationFailed", { reason: failure.reason })}
    </SystemMessage>
  );
}
