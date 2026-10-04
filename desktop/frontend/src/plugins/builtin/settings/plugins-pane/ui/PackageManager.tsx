import { parseReviewedJSON } from "@flame/runtime-contract/client/json";
import { checkRequest } from "@flame/runtime-contract/client/request";
import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { z } from "zod";
import type {
  PluginDiagnostic,
  PluginInstallation,
  PluginReleaseRequest,
  PluginRequest,
} from "@flame/runtime-contract/wire";
import { useT } from "@/lib/i18n";
import { DataView, PillButton, SystemMessage, TextEditorDialog, TextField, vocab } from "@/ui";
import { space } from "@/styles/tokens.stylex";
import { SettingsGroup, useAsyncFeedback } from "../../kit";
import { settingStyles as ss } from "../../kit/settingStyles";
import { packageOperations, usePackages } from "../application/packages";

const styles = stylex.create({
  row: { padding: space.s4, display: "flex", flexDirection: "column", gap: space.s3 },
  preview: { whiteSpace: "pre-wrap", overflowWrap: "anywhere", maxHeight: 240, overflow: "auto" },
});

type PackageEdit =
  | { type: "approve" | "configure"; target: PluginReleaseRequest; value: string }
  | { type: "stage"; target: PluginRequest; value: string };

export function PackageManager() {
  const t = useT();
  const catalog = usePackages();
  const [source, setSource] = useState("");
  const [diagnostics, setDiagnostics] = useState<{
    signal: AbortSignal;
    items: PluginDiagnostic[];
  }>();
  const operations = packageOperations.peek();
  const { feedback, run } = useAsyncFeedback(operations?.signal);
  const busy = feedback.state === "busy";
  const error = feedback.state === "error" ? feedback.reason : "";
  const install = () => {
    if (!operations) return;
    return run(async () => {
      const installed = await operations.install({ source });
      if (operations.signal.aborted) return { ok: true };
      setDiagnostics({ signal: operations.signal, items: installed.availability });
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
        {diagnostics?.signal === operations?.signal &&
          diagnostics?.items.map((diagnostic) => (
            <SystemMessage key={`${diagnostic.component}:${diagnostic.code}`} variant="warning">
              {diagnostic.component}: {diagnostic.code}
            </SystemMessage>
          ))}
      </div>
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
                  onAvailability={(items) => setDiagnostics({ signal: operations.signal, items })}
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
  onAvailability,
}: {
  installation: PluginInstallation;
  operations: ReturnType<typeof packageOperations.get>;
  refresh: () => Promise<unknown>;
  onAvailability: (items: PluginDiagnostic[]) => void;
}) {
  const t = useT();
  const [edit, setEdit] = useState<PackageEdit>();
  const { feedback, run: runFeedback } = useAsyncFeedback(operations.signal);
  const busy = feedback.state === "busy";
  const error = feedback.state === "error" ? feedback.reason : "";
  const request = { installationId: installation.id, digest: installation.selected.digest };
  const run = (operation: () => Promise<{ availability: PluginDiagnostic[] } | undefined>) =>
    runFeedback(async () => {
      const result = await operation();
      if (operations.signal.aborted) return { ok: true };
      if (result) onAvailability(result.availability);
      setEdit(undefined);
      await refresh();
      return { ok: true };
    }, t("packages.configure"));
  const save = () =>
    run(async () => {
      if (edit?.type === "approve") {
        return operations.approve(
          checkRequest("plugins.approve", {
            ...edit.target,
            grants: parseReviewedJSON(edit.value),
          }),
        );
      } else if (edit?.type === "configure") {
        const changes = z.record(z.string(), z.unknown()).parse(parseReviewedJSON(edit.value));
        for (const field of Object.keys(edit.target)) {
          if (Object.hasOwn(changes, field))
            throw new Error(`${field} belongs to the reviewed release`);
        }
        return operations.configure(
          checkRequest("plugins.configure", { ...edit.target, ...changes }),
        );
      } else if (edit?.type === "stage") {
        return operations.stage({ ...edit.target, source: edit.value });
      }
    });
  return (
    <div {...stylex.props(styles.row)}>
      <div {...stylex.props(ss.label, vocab.truncate)} title={installation.selected.name}>
        {installation.selected.name} · {installation.selected.version}
      </div>
      <div {...stylex.props(vocab.muted, vocab.truncate)} title={request.digest}>
        {request.digest}
      </div>
      <div {...stylex.props(ss.lineWrap)}>
        <PillButton
          size="sm"
          pending={busy}
          onClick={() => {
            setEdit({
              type: "approve",
              target: request,
              value: JSON.stringify(installation.selected.requests, null, 2),
            });
          }}
        >
          {t("packages.approve")}
        </PillButton>
        <PillButton
          size="sm"
          pending={busy}
          onClick={() => {
            setEdit({
              type: "configure",
              target: request,
              value: JSON.stringify(
                {
                  valueChanges: {},
                  disabledServers: installation.disabledServers,
                  disabledSkills: installation.disabledSkills,
                },
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
            void run(() =>
              operations.setEnablement({
                installationId: installation.id,
                enabled: !installation.enabled,
              }),
            )
          }
        >
          {t(installation.enabled ? "packages.disable" : "packages.enable")}
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
              void run(() =>
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
          onClick={() => void run(() => operations.revoke(installation.id))}
        >
          {t("packages.revoke")}
        </PillButton>
        <PillButton
          size="sm"
          pending={busy}
          onClick={() => void run(() => operations.uninstall(installation.id))}
        >
          {t("packages.uninstall")}
        </PillButton>
      </div>
      <pre {...stylex.props(styles.preview)}>
        {JSON.stringify(
          {
            servers: installation.selected.servers,
            inputs: installation.selected.inputs,
            skills: installation.selected.skills,
            diagnostics: [...installation.selected.diagnostics, ...installation.availability],
          },
          null,
          2,
        )}
      </pre>
      {error && <SystemMessage variant="warning">{error}</SystemMessage>}
      <TextEditorDialog
        open={edit !== undefined}
        onOpenChange={(open) => {
          if (!open) setEdit(undefined);
        }}
        title={t(
          edit?.type === "approve"
            ? "packages.approve"
            : edit?.type === "stage"
              ? "packages.stage"
              : "packages.configure",
        )}
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
