import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type RefObject,
} from "react";
import type { ComponentContext, Translate } from "@flame/component-sdk";
import {
  Button,
  FormActions,
  FormField,
  FormGroup,
  FormRow,
  isImeKey,
  ModalDialog,
  RecordForm,
  Segmented,
  Switch,
  TextArea,
  TextField,
} from "@flame/ui";
import type { MCPServerSettings, MCPTransport } from "./model";
import { MCPServerEdit, RetainedValue, type MCPServerFields } from "./draft";
import { MCPSettings, editorKey } from "./settings";
import { CommandFeedback } from "./feedback";
// Input examples are protocol values, not prose: they are written in the exact
// JSON the draft parser accepts and stay identical in every locale.
const EXAMPLE = {
  command: "npx",
  args: JSON.stringify([
    "-y",
    "@modelcontextprotocol/server-filesystem",
    "/path",
  ]),
  environment: JSON.stringify({ API_KEY: "…" }),
  url: "https://example.com/mcp",
  headers: JSON.stringify({ "X-API-Key": "…" }),
  timeoutSeconds: "60",
};
const split = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  gap: "var(--layout-content-gap)",
} as const;
// Credentials, commands and JSON are typed literally, never corrected.
const LITERAL_INPUT = {
  autoCapitalize: "none",
  autoCorrect: "off",
  spellCheck: false,
} as const;

function FieldIssue({
  id,
  shown,
  message,
}: {
  id: string;
  shown: boolean;
  message: string;
}) {
  if (!shown) return null;
  return (
    <span id={id} role="alert" style={{ color: "var(--color-negative)" }}>
      {message}
    </span>
  );
}

function FieldWarning({ message }: { message: string }) {
  return <span style={{ color: "var(--color-warning)" }}>{message}</span>;
}

function ClearStoredSecret({
  label,
  value,
  disabled,
  onChange,
}: {
  label: string;
  value: RetainedValue;
  disabled: boolean;
  onChange(value: RetainedValue): void;
}) {
  return (
    <div style={split}>
      <span>{label}</span>
      <Switch
        checked={value.disposition === "clear"}
        onCheckedChange={(clear) => onChange(value.cleared(clear))}
        ariaLabel={label}
        disabled={disabled}
      />
    </div>
  );
}
interface Props {
  context: ComponentContext;
  settings: MCPSettings;
  t: Translate;
  server?: MCPServerSettings;
  returnFocus: HTMLElement;
  onDone(): void;
  onCancel(): void;
}
export function ServerForm({
  context,
  settings,
  t,
  server,
  returnFocus,
  onDone,
  onCancel,
}: Props) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const key = editorKey(server?.id);
  const edit = state.drafts.get(key) ?? MCPServerEdit.of(server);
  const draft = edit.fields;
  const isEdit = server !== undefined;
  const firstField = useRef<HTMLInputElement>(null);
  const form = useRef<HTMLDivElement>(null);
  const close = (complete: () => void) => {
    const root = form.current;
    if (root?.contains(root.ownerDocument.activeElement))
      returnFocus.focus({ preventScroll: true });
    complete();
  };
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
  const busy = settings.busy(key);
  // Tool-policy work locks this server without being a form submission.
  const saving = state.commands.has(key);
  const blocked = busy && !saving;
  const probing = state.probing.has(key);
  const jsonErrorId = useId();
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const {
    authorization: hasAuthStored,
    headers: hasHeadersStored,
    environment: hasEnvironmentStored,
  } = storedSecrets(server);
  const updateDraft = <K extends keyof MCPServerFields>(
    key: K,
    value: MCPServerFields[K],
  ) => settings.edit(context, editorKey(server?.id), edit.with(key, value));
  const valid = edit.isValid;
  const onSave = async () => {
    if ((await settings.save(context, edit, server?.id)) && presented.current)
      close(onDone);
  };
  const onTest = () => settings.test(context, key, edit);
  const onDelete = async () => {
    if (
      server &&
      (await settings.remove(context, server.id)) &&
      presented.current
    )
      close(onDone);
  };
  return (
    <RecordForm
      ref={form}
      onKeyDown={(event) => {
        // Keyboard events from the Delete portal also bubble through this React handler.
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          isImeKey(event.nativeEvent) ||
          !event.currentTarget.contains(event.target as Node)
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        close(onCancel);
      }}
    >
      <FormRow>
        <FormField label={t("mcp.form.name.aria")}>
          <TextField
            ref={isEdit ? undefined : firstField}
            type="text"
            required
            aria-label={t("mcp.form.name.aria")}
            {...LITERAL_INPUT}
            pending={busy}
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
            disabled={busy}
          />
        </FormGroup>
      </FormRow>

      {draft.transport === "stdio" ? (
        <StdioFields
          t={t}
          edit={edit}
          busy={busy}
          firstField={isEdit ? firstField : undefined}
          jsonErrorId={jsonErrorId}
          environmentStored={hasEnvironmentStored}
          updateDraft={updateDraft}
        />
      ) : (
        <HttpFields
          t={t}
          edit={edit}
          busy={busy}
          firstField={isEdit ? firstField : undefined}
          jsonErrorId={jsonErrorId}
          authorizationStored={hasAuthStored}
          headersStored={hasHeadersStored}
          updateDraft={updateDraft}
        />
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
            pending={busy}
            value={draft.timeoutSec}
            onChange={(e) => updateDraft("timeoutSec", e.target.value)}
            placeholder={EXAMPLE.timeoutSeconds}
          />
          <FieldIssue
            id={timeoutErrorId}

            shown={timeoutInvalid}

            message={t("mcp.form.timeout.invalid")}
          />
        </FormField>

        <FormField label={t("mcp.form.description.aria")}>
          <TextField
            type="text"
            aria-label={t("mcp.form.description.aria")}
            pending={busy}
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
              disabled={blocked}
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
            onSettled={() => {
              if (presented.current) close(onDone);
            }}
          />
        }
      >
        <Button
          variant="outline"
          size="sm"
          disabled={!valid || blocked}
          pending={saving || probing}
          onClick={() => void onTest()}
        >
          {probing ? t("mcp.testing") : t("mcp.test")}
        </Button>
        <Button variant="ghost" size="sm" onClick={() => close(onCancel)}>
          {t("common.cancel")}
        </Button>
        <Button
          variant="primary"
          size="sm"
          disabled={!valid || blocked}
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
                disabled={blocked}
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

interface TransportFieldsProps {
  t: Translate;
  edit: MCPServerEdit;
  busy: boolean;
  firstField: RefObject<HTMLInputElement | null> | undefined;
  jsonErrorId: string;
  updateDraft<K extends keyof MCPServerFields>(
    key: K,
    value: MCPServerFields[K],
  ): void;
}

function StdioFields({
  t,
  edit,
  busy,
  firstField,
  jsonErrorId,
  environmentStored,
  updateDraft,
}: TransportFieldsProps & { environmentStored: boolean }) {
  const draft = edit.fields;
  return (
    <>
      <FormField label={t("mcp.form.command.aria")}>
        <TextField
          ref={firstField}
          font="mono"
          type="text"
          required
          aria-label={t("mcp.form.command.aria")}
          {...LITERAL_INPUT}
          pending={busy}
          value={draft.command}
          onChange={(e) => updateDraft("command", e.target.value)}
          placeholder={EXAMPLE.command}
        />
      </FormField>
      <FormRow>
        <FormField label={t("mcp.form.args")}>
          <TextArea
            font="mono"
            rows={3}
            autosize
            aria-label={t("mcp.form.args")}
            {...LITERAL_INPUT}
            pending={busy}
            value={draft.args}
            invalid={edit.jsonIssue === "args"}
            aria-describedby={
              edit.jsonIssue === "args" ? jsonErrorId : undefined
            }
            onChange={(event) => updateDraft("args", event.target.value)}
            placeholder={EXAMPLE.args}
          />
          <FieldIssue
            id={jsonErrorId}

            shown={edit.jsonIssue === "args"}

            message={t("mcp.form.args.invalid")}
          />
        </FormField>
        <FormGroup label={t("mcp.form.env")}>
          <TextArea
            font="mono"
            rows={3}
            autosize
            aria-label={t("mcp.form.env")}
            {...LITERAL_INPUT}
            pending={busy}
            value={draft.environment.text}
            invalid={edit.jsonIssue === "environment"}
            aria-describedby={
              edit.jsonIssue === "environment" ? jsonErrorId : undefined
            }
            onChange={(event) =>
              updateDraft(
                "environment",
                draft.environment.edited(event.target.value),
              )
            }
            placeholder={
              environmentStored ? t("mcp.form.env.keep") : EXAMPLE.environment
            }
          />
          <FieldIssue
            id={jsonErrorId}

            shown={edit.jsonIssue === "environment"}

            message={t("mcp.form.env.invalid")}
          />
          {environmentStored && (
            <ClearStoredSecret
              label={t("mcp.form.env.clear")}

              value={draft.environment}

              disabled={busy}

              onChange={(value) => updateDraft("environment", value)}
            />
          )}
          {edit.environmentNeedsDisposition && (
            <FieldWarning message={t("mcp.form.env.targetChanged")} />
          )}
        </FormGroup>
      </FormRow>
      <FormField label={t("mcp.form.dir.aria")}>
        <TextField
          font="mono"
          type="text"
          aria-label={t("mcp.form.dir.aria")}
          {...LITERAL_INPUT}
          pending={busy}
          value={draft.dir}
          onChange={(e) => updateDraft("dir", e.target.value)}
        />
      </FormField>
    </>
  );
}

function HttpFields({
  t,
  edit,
  busy,
  firstField,
  jsonErrorId,
  authorizationStored,
  headersStored,
  updateDraft,
}: TransportFieldsProps & {
  authorizationStored: boolean;
  headersStored: boolean;
}) {
  const draft = edit.fields;
  return (
    <>
      <FormField label={t("mcp.form.url.aria")}>
        <TextField
          ref={firstField}
          font="mono"
          type="text"
          required
          aria-label={t("mcp.form.url.aria")}
          {...LITERAL_INPUT}
          pending={busy}
          value={draft.url}
          onChange={(e) => updateDraft("url", e.target.value)}
          placeholder={EXAMPLE.url}
        />
      </FormField>
      <FormField label={t("mcp.form.auth.aria")}>
        <TextField
          font="mono"
          type="password"
          aria-label={t("mcp.form.auth.aria")}
          {...LITERAL_INPUT}
          pending={busy}
          value={draft.authorization.text}
          onChange={(e) =>
            updateDraft(
              "authorization",
              draft.authorization.edited(e.target.value),
            )
          }
          placeholder={
            authorizationStored
              ? t("mcp.form.auth.keep")
              : t("mcp.form.auth.placeholder")
          }
        />
      </FormField>
      {authorizationStored && (
        <ClearStoredSecret
          label={t("mcp.form.auth.clear")}

          value={draft.authorization}

          disabled={busy}

          onChange={(value) => updateDraft("authorization", value)}
        />
      )}
      {edit.authorizationNeedsDisposition && (
        <FieldWarning message={t("mcp.form.auth.originChanged")} />
      )}
      <FormField label={t("mcp.form.headers")}>
        <TextArea
          font="mono"
          rows={3}
          autosize
          aria-label={t("mcp.form.headers")}
          {...LITERAL_INPUT}
          pending={busy}
          value={draft.headers.text}
          invalid={edit.jsonIssue === "headers"}
          aria-describedby={
            edit.jsonIssue === "headers" ? jsonErrorId : undefined
          }
          onChange={(event) =>
            updateDraft("headers", draft.headers.edited(event.target.value))
          }
          placeholder={
            headersStored ? t("mcp.form.headers.keep") : EXAMPLE.headers
          }
        />
        <FieldIssue
          id={jsonErrorId}

          shown={edit.jsonIssue === "headers"}

          message={t("mcp.form.headers.invalid")}
        />
      </FormField>
      {headersStored && (
        <ClearStoredSecret
          label={t("mcp.form.headers.clear")}

          value={draft.headers}

          disabled={busy}

          onChange={(value) => updateDraft("headers", value)}
        />
      )}
      {edit.headersNeedDisposition && (
        <FieldWarning message={t("mcp.form.headers.originChanged")} />
      )}
    </>
  );
}

function storedSecrets(server: MCPServerSettings | undefined) {
  const hasAuthStored = (server?.authorizationMasked ?? "") !== "";
  const hasHeadersStored = Object.keys(server?.headersMasked ?? {}).length > 0;
  const hasEnvironmentStored = Object.keys(server?.envMasked ?? {}).length > 0;
  return {
    authorization: hasAuthStored,
    headers: hasHeadersStored,
    environment: hasEnvironmentStored,
  };
}
