import { failureText } from "@/lib/rpcErrors";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useId, useLayoutEffect, useRef, useState } from "react";
import { ConfirmDialog } from "@/ui/index";
import { Icon } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { Segmented, Switch } from "@flame/ui";
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
import { type MCPServerSettings } from "@/features/mcp/application/mcpServerConfig";
import {
  type MCPTransport,
  useMCPServerMutationMaterialGeneration,
  useMCPServerCommands,
} from "@/features/mcp/application/mcpServerConfig";
import { useT } from "@/lib/i18n/index";
import { MCPServerEdit, type MCPServerFields } from "@/features/mcp/application/mcpServerDraft";
import { useAsyncFeedback } from "@/lib/useAsyncFeedback";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
interface Props {
  server?: MCPServerSettings;
  opener: HTMLElement;
  onDone: () => void;
  onCancel: () => void;
}

export function ServerForm({ server, opener, onDone, onCancel }: Props) {
  const t = useT();
  const currentCommands = useMCPServerCommands();
  const [commands] = useState(currentCommands);
  useLayoutEffect(() => {
    if (!commands || commands.retired) onCancel();
  }, [commands, currentCommands, onCancel]);
  const isEdit = server !== undefined;
  const firstField = useRef<HTMLInputElement>(null);
  const presented = useRef(false);
  useLayoutEffect(() => {
    presented.current = true;
    firstField.current?.focus();
    return () => {
      presented.current = false;
    };
  }, []);

  const [edit, setEdit] = useState<MCPServerEdit>(() => MCPServerEdit.of(server));
  const draft = edit.fields;
  const timeoutInvalid = edit.handshakeTimeout === undefined;
  const timeoutErrorId = useId();
  const jsonIssue = edit.jsonIssue;
  const jsonErrorId = useId();

  const [saving, setSaving] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const materialGeneration = useMCPServerMutationMaterialGeneration();
  const { feedback, reset, fail, run } = useAsyncFeedback(materialGeneration);

  const hasAuthStored = (server?.authorizationMasked ?? "") !== "";
  const hasHeadersStored = Object.keys(server?.headersMasked ?? {}).length > 0;
  const hasEnvironmentStored = Object.keys(server?.envMasked ?? {}).length > 0;

  const updateDraft = <K extends keyof MCPServerFields>(key: K, value: MCPServerFields[K]) => {
    setEdit((current) => current.with(key, value));
    reset();
  };

  const buildInput = () => edit.toInput();

  const needsAuthorizationDisposition = edit.authorizationNeedsDisposition;
  const needsHeadersDisposition = edit.headersNeedDisposition;
  const needsEnvironmentDisposition = edit.environmentNeedsDisposition;
  const valid = edit.isValid;

  if (!commands || commands.retired) return null;

  const onSave = async () => {
    setSaving(true);
    reset();
    try {
      if (server) {
        const update = edit.toUpdate();
        if (update) await commands.update(server.id, update);
      } else await commands.create(buildInput());
      if (presented.current) onDone();
    } catch (err) {
      if (!presented.current || wasGenerationRetired(err)) return;
      fail(failureText(err, t("mcp.error.save")));
    } finally {
      if (presented.current) setSaving(false);
    }
  };

  const onTest = () =>
    run(() => commands.test(buildInput()), t("mcp.error.test"), wasGenerationRetired);

  const onDelete = async () => {
    if (!server) return;
    setSaving(true);
    reset();
    try {
      await commands.delete(server.id);
      if (presented.current) onDone();
    } catch (err) {
      if (!presented.current || wasGenerationRetired(err)) return;
      fail(failureText(err, t("mcp.error.remove")));
    } finally {
      if (presented.current) setSaving(false);
    }
  };

  return (
    <RecordForm opener={opener}>
      <FormRow>
        <FormField label={t("mcp.form.name.aria")}>
          <TextField
            ref={isEdit ? undefined : firstField}
            type="text"
            required
            aria-label={t("mcp.form.name.aria")}
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            pending={saving}
            value={draft.name}
            disabled={isEdit}
            onChange={(e) => updateDraft("name", e.target.value)}
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
            disabled={saving}
          />
        </FormGroup>
      </FormRow>

      {draft.transport === "stdio" ? (
        <>
          <FormField label={t("mcp.form.command.aria")}>
            <TextField
              ref={isEdit ? firstField : undefined}
              font="mono"
              type="text"
              required
              aria-label={t("mcp.form.command.aria")}
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              pending={saving}
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
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                pending={saving}
                value={draft.args}
                invalid={jsonIssue === "args"}
                aria-describedby={jsonIssue === "args" ? jsonErrorId : undefined}
                onChange={(event) => updateDraft("args", event.target.value)}
                placeholder={t("mcp.form.args.placeholder")}
              />
              {jsonIssue === "args" && (
                <span
                  id={jsonErrorId}
                  role="alert"
                  {...stylex.props(vocab.negative, typeStep.uiMd)}
                >
                  {t("mcp.form.args.invalid")}
                </span>
              )}
            </FormField>
            <div {...stylex.props(vocab.column, gap.s1_5)}>
              <FormField label={t("mcp.form.env")}>
                <TextArea
                  font="mono"
                  rows={3}
                  aria-label={t("mcp.form.env")}
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  pending={saving}
                  value={draft.environment.text}
                  invalid={jsonIssue === "environment"}
                  aria-describedby={jsonIssue === "environment" ? jsonErrorId : undefined}
                  onChange={(event) =>
                    updateDraft("environment", draft.environment.edited(event.target.value))
                  }
                  placeholder={
                    hasEnvironmentStored ? t("mcp.form.env.keep") : t("mcp.form.env.placeholder")
                  }
                />
                {jsonIssue === "environment" && (
                  <span
                    id={jsonErrorId}
                    role="alert"
                    {...stylex.props(vocab.negative, typeStep.uiMd)}
                  >
                    {t("mcp.form.env.invalid")}
                  </span>
                )}
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
                    disabled={saving}
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
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              pending={saving}
              value={draft.dir}
              onChange={(e) => updateDraft("dir", e.target.value)}
            />
          </FormField>
        </>
      ) : (
        <>
          <FormField label={t("mcp.form.url.aria")}>
            <TextField
              ref={isEdit ? firstField : undefined}
              font="mono"
              type="text"
              required
              aria-label={t("mcp.form.url.aria")}
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              pending={saving}
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
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              pending={saving}
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
                disabled={saving}
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
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              pending={saving}
              value={draft.headers.text}
              invalid={jsonIssue === "headers"}
              aria-describedby={jsonIssue === "headers" ? jsonErrorId : undefined}
              onChange={(event) => updateDraft("headers", draft.headers.edited(event.target.value))}
              placeholder={
                hasHeadersStored ? t("mcp.form.headers.keep") : t("mcp.form.headers.placeholder")
              }
            />
            {jsonIssue === "headers" && (
              <span id={jsonErrorId} role="alert" {...stylex.props(vocab.negative, typeStep.uiMd)}>
                {t("mcp.form.headers.invalid")}
              </span>
            )}
          </FormField>
          {hasHeadersStored && (
            <label {...stylex.props(ss.split, vocab.muted, typeStep.uiMd)}>
              <span>{t("mcp.form.headers.clear")}</span>
              <Switch
                checked={draft.headers.disposition === "clear"}
                onCheckedChange={(value) => updateDraft("headers", draft.headers.cleared(value))}
                ariaLabel={t("mcp.form.headers.clear")}
                disabled={saving}
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
            min={1}
            step={1}
            max={Number.MAX_SAFE_INTEGER}
            inputMode="numeric"
            invalid={timeoutInvalid}
            aria-describedby={timeoutInvalid ? timeoutErrorId : undefined}
            aria-label={t("mcp.form.timeout")}
            pending={saving}
            value={draft.timeoutSec}
            onChange={(e) => updateDraft("timeoutSec", e.target.value)}
            placeholder={t("mcp.form.timeout.placeholder")}
          />
          {timeoutInvalid && (
            <span id={timeoutErrorId} role="alert" {...stylex.props(vocab.negative, typeStep.uiMd)}>
              {t("mcp.form.timeout.invalid")}
            </span>
          )}
        </FormField>

        <FormField label={t("mcp.form.description.aria")}>
          <TextField
            type="text"
            aria-label={t("mcp.form.description.aria")}
            pending={saving}
            value={draft.description}
            onChange={(e) => updateDraft("description", e.target.value)}
          />
        </FormField>
      </FormRow>

      <FormActions
        label={t(isEdit ? "mcp.save" : "mcp.add")}
        auxiliaryAction={
          isEdit && (
            <Button
              variant="tonal"
              tone="negative"
              size="sm"
              pending={saving}
              onClick={() => setConfirmingDelete(true)}
            >
              {t("mcp.delete")}
            </Button>
          )
        }
        feedback={
          <>
            <span
              role="status"
              className={
                feedback.state === "ok"
                  ? stylex.props(ss.inline, vocab.success, typeStep.uiMd).className
                  : "sr-only"
              }
            >
              {feedback.state === "ok" && (
                <>
                  <Icon name="check" size="sm" /> {t("mcp.connectionOk")}
                </>
              )}
            </span>
            {feedback.state === "error" && (
              <SystemMessage variant="warning">{feedback.reason}</SystemMessage>
            )}
          </>
        }
      >
        <Button
          variant="outline"
          size="sm"
          disabled={!valid}
          pending={saving || feedback.state === "busy"}
          onClick={() => void onTest()}
        >
          {feedback.state === "busy" ? t("mcp.testing") : t("mcp.test")}
        </Button>
        <Button variant="ghost" size="sm" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
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
      {server && (
        <ConfirmDialog
          open={confirmingDelete}
          onOpenChange={setConfirmingDelete}
          title={t("mcp.delete.title")}
          body={t("mcp.delete.body", { server: server.id.name })}
          confirmLabel={t("mcp.delete")}
          cancelLabel={t("common.cancel")}
          destructive
          onConfirm={() => void onDelete()}
        />
      )}
    </RecordForm>
  );
}
