import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import type { ComponentContext, Translate } from "@flame/component-sdk";
import {
  Button,
  FormActions,
  FormField,
  FormGroup,
  FormRow,
  ModalDialog,
  RecordForm,
  Segmented,
  Switch,
  TextArea,
  TextField,
} from "@flame/ui";
import type { MCPServerSettings, MCPTransport } from "./model";
import { MCPServerEdit, type MCPServerFields } from "./draft";
import { MCPSettings, editorKey } from "./settings";
import { CommandFeedback } from "./feedback";
const column = {
  display: "grid",
  minWidth: 0,
  gap: "var(--layout-control-gap)",
} as const;
const split = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  gap: "var(--layout-content-gap)",
} as const;
interface Props {
  context: ComponentContext;
  settings: MCPSettings;
  t: Translate;
  server?: MCPServerSettings;
  opener: HTMLElement;
  onDone(): void;
  onCancel(): void;
}
export function ServerForm({
  context,
  settings,
  t,
  server,
  opener,
  onDone,
  onCancel,
}: Props) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const key = editorKey(server?.id);
  const [initialDraft] = useState(() => MCPServerEdit.of(server));
  const edit = state.drafts.get(key) ?? initialDraft;
  const draft = edit.fields;
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
  const timeoutInvalid = edit.handshakeTimeout === undefined;
  const timeoutErrorId = useId();
  const saving = settings.busy(key);
  const probing = state.probing.has(key);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const hasAuthStored = (server?.authorizationMasked ?? "") !== "";
  const hasHeadersStored = Object.keys(server?.headersMasked ?? {}).length > 0;
  const hasEnvironmentStored = Object.keys(server?.envMasked ?? {}).length > 0;
  const updateDraft = <K extends keyof MCPServerFields>(
    key: K,
    value: MCPServerFields[K],
  ) => settings.edit(context, editorKey(server?.id), edit.with(key, value));
  const needsAuthorizationDisposition = edit.authorizationNeedsDisposition;
  const needsHeadersDisposition = edit.headersNeedDisposition;
  const needsEnvironmentDisposition = edit.environmentNeedsDisposition;
  const valid = edit.isValid;
  const onSave = async () => {
    if ((await settings.save(context, edit, server?.id)) && presented.current)
      onDone();
  };
  const onTest = () => settings.test(context, key, edit);
  const onDelete = async () => {
    if (
      server &&
      (await settings.remove(context, server.id)) &&
      presented.current
    )
      onDone();
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
                onChange={(event) => updateDraft("args", event.target.value)}
                placeholder={t("mcp.form.args.placeholder")}
              />
            </FormField>
            <div style={column}>
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
                  onChange={(event) =>
                    updateDraft(
                      "environment",
                      draft.environment.edited(event.target.value),
                    )
                  }
                  placeholder={
                    hasEnvironmentStored
                      ? t("mcp.form.env.keep")
                      : t("mcp.form.env.placeholder")
                  }
                />
              </FormField>
              {hasEnvironmentStored && (
                <div style={split}>
                  <span>{t("mcp.form.env.clear")}</span>
                  <Switch
                    checked={draft.environment.disposition === "clear"}
                    onCheckedChange={(value) =>
                      updateDraft(
                        "environment",
                        draft.environment.cleared(value),
                      )
                    }
                    ariaLabel={t("mcp.form.env.clear")}
                    disabled={saving}
                  />
                </div>
              )}
              {needsEnvironmentDisposition && (
                <span style={{ color: "var(--color-warning)" }}>
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
                updateDraft(
                  "authorization",
                  draft.authorization.edited(e.target.value),
                )
              }
              placeholder={
                hasAuthStored
                  ? t("mcp.form.auth.keep")
                  : t("mcp.form.auth.placeholder")
              }
            />
          </FormField>
          {hasAuthStored && (
            <div style={split}>
              <span>{t("mcp.form.auth.clear")}</span>
              <Switch
                checked={draft.authorization.disposition === "clear"}
                onCheckedChange={(value) =>
                  updateDraft(
                    "authorization",
                    draft.authorization.cleared(value),
                  )
                }
                ariaLabel={t("mcp.form.auth.clear")}
                disabled={saving}
              />
            </div>
          )}
          {needsAuthorizationDisposition && (
            <span style={{ color: "var(--color-warning)" }}>
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
              onChange={(event) =>
                updateDraft("headers", draft.headers.edited(event.target.value))
              }
              placeholder={
                hasHeadersStored
                  ? t("mcp.form.headers.keep")
                  : t("mcp.form.headers.placeholder")
              }
            />
          </FormField>
          {hasHeadersStored && (
            <div style={split}>
              <span>{t("mcp.form.headers.clear")}</span>
              <Switch
                checked={draft.headers.disposition === "clear"}
                onCheckedChange={(value) =>
                  updateDraft("headers", draft.headers.cleared(value))
                }
                ariaLabel={t("mcp.form.headers.clear")}
                disabled={saving}
              />
            </div>
          )}
          {needsHeadersDisposition && (
            <span style={{ color: "var(--color-warning)" }}>
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
            <span
              id={timeoutErrorId}
              role="alert"
              style={{ color: "var(--color-negative)" }}
            >
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
          <CommandFeedback
            context={context}
            settings={settings}
            resource={key}
            t={t}
            onSettled={onDone}
          />
        }
      >
        <Button
          variant="outline"
          size="sm"
          disabled={!valid}
          pending={saving || probing}
          onClick={() => void onTest()}
        >
          {probing ? t("mcp.testing") : t("mcp.test")}
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
        <ModalDialog
          open={confirmingDelete}
          onOpenChange={setConfirmingDelete}
          title={t("mcp.delete.title")}
          closeLabel={t("common.close")}
          footer={
            <>
              <Button
                variant="ghost"
                onClick={() => setConfirmingDelete(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                tone="negative"
                pending={saving}
                onClick={() => {
                  setConfirmingDelete(false);
                  void onDelete();
                }}
              >
                {t("mcp.delete")}
              </Button>
            </>
          }
        >
          {t("mcp.delete.body", { server: server.id.name })}
        </ModalDialog>
      )}
    </RecordForm>
  );
}
