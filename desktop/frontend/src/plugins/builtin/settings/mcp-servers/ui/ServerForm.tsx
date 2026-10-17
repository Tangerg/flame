import { failureText } from "@/lib/rpcErrors";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useState } from "react";
import { Icon, Segmented, Switch } from "@/ui";
import {
  Button,
  FormActions,
  FormField,
  FormGroup,
  FormRow,
  RecordForm,
  gap,
  TextArea,
  TextField,
  vocab,
} from "@flame/ui";
import {
  type MCPServerSettings,
  type MCPTransport,
  useMCPServerMutationMaterialGeneration,
  useCreateMCPServer,
  useDeleteMCPServer,
  useTestMCPServer,
  useUpdateMCPServer,
} from "../application/mcpServerConfig";
import { useT } from "@/lib/i18n";
import { MCPServerEdit, type MCPServerFields } from "../application/mcpServerDraft";
import { ToolControls } from "./ToolControls";
import { useAsyncFeedback } from "../../kit";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

interface Props {
  server?: MCPServerSettings;
  onDone: () => void;
  onCancel: () => void;
}

export function ServerForm({ server, onDone, onCancel }: Props) {
  const t = useT();
  const create = useCreateMCPServer();
  const update = useUpdateMCPServer();
  const remove = useDeleteMCPServer();
  const test = useTestMCPServer();
  const isEdit = server !== undefined;

  const [edit, setEdit] = useState<MCPServerEdit>(() => MCPServerEdit.of(server));
  const draft = edit.fields;

  const [saving, setSaving] = useState(false);
  const materialGeneration = useMCPServerMutationMaterialGeneration();
  const { feedback, reset, fail, run } = useAsyncFeedback(materialGeneration);

  const hasAuthStored = (server?.authorizationMasked ?? "") !== "";
  const hasHeadersStored = Object.keys(server?.headersMasked ?? {}).length > 0;
  const hasEnvironmentStored = Object.keys(server?.envMasked ?? {}).length > 0;

  const updateDraft = <K extends keyof MCPServerFields>(key: K, value: MCPServerFields[K]) => {
    setEdit((current) => current.with(key, value));
  };

  const buildInput = () => edit.toInput();

  const needsAuthorizationDisposition = edit.authorizationNeedsDisposition;
  const needsHeadersDisposition = edit.headersNeedDisposition;
  const needsEnvironmentDisposition = edit.environmentNeedsDisposition;
  const valid = edit.isValid;

  const onSave = async () => {
    setSaving(true);
    reset();
    try {
      const input = buildInput();
      if (server) await update(server.id, input);
      else await create(input);
      onDone();
    } catch (err) {
      if (wasGenerationRetired(err)) return;
      fail(failureText(err, t("mcp.error.save")));
    } finally {
      setSaving(false);
    }
  };

  const onTest = () => run(() => test(buildInput()), t("mcp.error.test"), wasGenerationRetired);

  const onDelete = async () => {
    if (!server) return;
    setSaving(true);
    try {
      await remove(server.id);
      onDone();
    } catch (err) {
      if (wasGenerationRetired(err)) return;
      fail(failureText(err, t("mcp.error.remove")));
    } finally {
      setSaving(false);
    }
  };

  return (
    <RecordForm>
      <p {...stylex.props(typeStep.uiSm, vocab.muted)}>{t("mcp.saveEffect")}</p>
      <FormRow>
        <FormField label={t("mcp.form.name.aria")}>
          <TextField
            type="text"
            aria-label={t("mcp.form.name.aria")}
            value={draft.name}
            disabled={isEdit}
            onChange={(e) => updateDraft("name", e.target.value)}
            placeholder={t("mcp.form.name.placeholder")}
          />
        </FormField>
        <FormGroup label={t("mcp.form.transport.aria")}>
          <Segmented<MCPTransport>
            value={draft.transport}
            options={[
              { value: "stdio", label: t("mcp.transport.stdio") },
              { value: "streamableHttp", label: t("mcp.transport.http") },
            ]}
            onChange={(value) => updateDraft("transport", value)}
            ariaLabel={t("mcp.form.transport.aria")}
          />
        </FormGroup>
      </FormRow>

      {draft.transport === "stdio" ? (
        <>
          <FormField label={t("mcp.form.command.aria")}>
            <TextField
              font="mono"
              type="text"
              aria-label={t("mcp.form.command.aria")}
              value={draft.command}
              onChange={(e) => updateDraft("command", e.target.value)}
              placeholder={t("mcp.form.command.placeholder")}
            />
          </FormField>
          <FormRow>
            <FormField label={t("mcp.form.args")}>
              <TextArea
                font="mono"
                rows={3}
                aria-label={t("mcp.form.args")}
                value={draft.args}
                onChange={(event) => updateDraft("args", event.target.value)}
                placeholder={t("mcp.form.args.placeholder")}
              />
            </FormField>
            <div {...stylex.props(vocab.column, gap.s1_5)}>
              <FormField label={t("mcp.form.env")}>
                <TextArea
                  font="mono"
                  rows={3}
                  aria-label={t("mcp.form.env")}
                  value={draft.environment.text}
                  onChange={(event) =>
                    updateDraft("environment", draft.environment.edited(event.target.value))
                  }
                  placeholder={
                    hasEnvironmentStored ? t("mcp.form.env.keep") : t("mcp.form.env.placeholder")
                  }
                />
              </FormField>
              {hasEnvironmentStored && (
                <label {...stylex.props(ss.split, vocab.muted, typeStep.uiMd)}>
                  <span>{t("mcp.form.env.clear")}</span>
                  <Switch
                    checked={draft.environment.disposition === "clear"}
                    onCheckedChange={(value) =>
                      updateDraft("environment", draft.environment.cleared(value))
                    }
                    ariaLabel={t("mcp.form.env.clear")}
                  />
                </label>
              )}
              {needsEnvironmentDisposition && (
                <span {...stylex.props(vocab.warning, typeStep.uiMd)}>
                  {t("mcp.form.env.targetChanged")}
                </span>
              )}
            </div>
          </FormRow>
          <FormField label={t("mcp.form.dir.aria")}>
            <TextField
              font="mono"
              type="text"
              aria-label={t("mcp.form.dir.aria")}
              value={draft.dir}
              onChange={(e) => updateDraft("dir", e.target.value)}
              placeholder={t("mcp.form.dir.placeholder")}
            />
          </FormField>
        </>
      ) : (
        <>
          <FormField label={t("mcp.form.url.aria")}>
            <TextField
              font="mono"
              type="text"
              aria-label={t("mcp.form.url.aria")}
              value={draft.url}
              onChange={(e) => updateDraft("url", e.target.value)}
              placeholder={t("mcp.form.url.placeholder")}
            />
          </FormField>
          <FormField label={t("mcp.form.auth.aria")}>
            <TextField
              font="mono"
              type="password"
              aria-label={t("mcp.form.auth.aria")}
              value={draft.authorization.text}
              onChange={(e) =>
                updateDraft("authorization", draft.authorization.edited(e.target.value))
              }
              placeholder={hasAuthStored ? t("mcp.form.auth.keep") : t("mcp.form.auth.placeholder")}
            />
          </FormField>
          {hasAuthStored && (
            <label {...stylex.props(ss.split, vocab.muted, typeStep.uiMd)}>
              <span>{t("mcp.form.auth.clear")}</span>
              <Switch
                checked={draft.authorization.disposition === "clear"}
                onCheckedChange={(value) =>
                  updateDraft("authorization", draft.authorization.cleared(value))
                }
                ariaLabel={t("mcp.form.auth.clear")}
              />
            </label>
          )}
          {needsAuthorizationDisposition && (
            <span {...stylex.props(vocab.warning, typeStep.uiMd)}>
              {t("mcp.form.auth.originChanged")}
            </span>
          )}
          <FormField label={t("mcp.form.headers")}>
            <TextArea
              font="mono"
              rows={3}
              aria-label={t("mcp.form.headers")}
              value={draft.headers.text}
              onChange={(event) => updateDraft("headers", draft.headers.edited(event.target.value))}
              placeholder={
                hasHeadersStored ? t("mcp.form.headers.keep") : t("mcp.form.headers.placeholder")
              }
            />
          </FormField>
          {hasHeadersStored && (
            <label {...stylex.props(ss.split, vocab.muted, typeStep.uiMd)}>
              <span>{t("mcp.form.headers.clear")}</span>
              <Switch
                checked={draft.headers.disposition === "clear"}
                onCheckedChange={(value) => updateDraft("headers", draft.headers.cleared(value))}
                ariaLabel={t("mcp.form.headers.clear")}
              />
            </label>
          )}
          {needsHeadersDisposition && (
            <span {...stylex.props(vocab.warning, typeStep.uiMd)}>
              {t("mcp.form.headers.originChanged")}
            </span>
          )}
        </>
      )}

      <FormRow>
        <FormField label={t("mcp.form.timeout")}>
          <TextField
            type="number"
            min={0}
            inputMode="numeric"
            aria-label={t("mcp.form.timeout")}
            value={draft.timeoutSec}
            onChange={(e) => updateDraft("timeoutSec", e.target.value)}
            placeholder={t("mcp.form.timeout.placeholder")}
          />
        </FormField>

        <FormField label={t("mcp.form.description.aria")}>
          <TextField
            type="text"
            aria-label={t("mcp.form.description.aria")}
            value={draft.description}
            onChange={(e) => updateDraft("description", e.target.value)}
            placeholder={t("mcp.form.description.placeholder")}
          />
        </FormField>
      </FormRow>
      {server && (
        <div {...stylex.props(vocab.column, gap.s1_5)}>
          <span {...stylex.props(ss.label, typeStep.uiMd)}>{t("mcp.tools.manage")}</span>
          <ToolControls server={server.id} />
        </div>
      )}

      <FormActions label={t(isEdit ? "mcp.save" : "mcp.add")}>
        <Button
          variant="outline"
          size="sm"
          disabled={!valid}
          pending={feedback.state === "busy"}
          onClick={() => void onTest()}
        >
          {feedback.state === "busy" ? t("mcp.testing") : t("mcp.test")}
        </Button>
        <Button variant="outline" size="sm" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        {isEdit && (
          <Button
            variant="tonal"
            tone="negative"
            size="sm"
            pending={saving}
            onClick={() => void onDelete()}
          >
            {t("mcp.delete")}
          </Button>
        )}

        <Button
          variant="primary"
          size="sm"
          disabled={!valid}
          pending={saving}
          onClick={() => void onSave()}
        >
          {saving ? t("mcp.saving") : isEdit ? t("mcp.save") : t("mcp.add")}
        </Button>
      </FormActions>
      {feedback.state === "ok" && (
        <span {...stylex.props(ss.inline, vocab.success, typeStep.uiMd)}>
          <Icon name="check" size="sm" /> {t("mcp.connectionOk")}
        </span>
      )}
      {feedback.state === "error" && (
        <span {...stylex.props(ss.inline, vocab.min, vocab.negative, typeStep.uiMd)}>
          <Icon name="alert" size="sm" />
          <span>{feedback.reason}</span>
        </span>
      )}
    </RecordForm>
  );
}
