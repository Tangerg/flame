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
  isImeKey,
  ModalDialog,
  RecordForm,
  Segmented,
  TextField,
} from "@flame/ui";
import type { MCPServerSettings, MCPTransport } from "./model";
import { MCPServerEdit, type MCPServerFields } from "./draft";
import { MCPSettings, editorKey } from "./settings";
import { CommandFeedback } from "./feedback";
import {
  EXAMPLE,
  FieldIssue,
  HttpFields,
  LITERAL_INPUT,
  StdioFields,
  type TransportFieldsProps,
} from "./transportFields";

interface Props {
  context: ComponentContext;
  settings: MCPSettings;
  t: Translate;
  server?: MCPServerSettings;
  returnFocus: HTMLElement;
  onDone(): void;
  onCancel(): void;
}

function saveLabel(t: Translate, saving: boolean, isEdit: boolean): string {
  if (saving) return t("mcp.saving");
  return isEdit ? t("mcp.save") : t("mcp.add");
}

// Each transport edits its own fields; stored secrets stay masked until replaced.
function ServerTransportFields({
  t,
  edit,
  busy,
  firstField,
  jsonErrorId,
  stored,
  updateDraft,
}: TransportFieldsProps & { stored: ReturnType<typeof storedSecrets> }) {
  if (edit.fields.transport === "stdio")
    return (
      <StdioFields
        t={t}
        edit={edit}
        busy={busy}
        firstField={firstField}
        jsonErrorId={jsonErrorId}
        environmentStored={stored.environment}
        updateDraft={updateDraft}
      />
    );
  return (
    <HttpFields
      t={t}
      edit={edit}
      busy={busy}
      firstField={firstField}
      jsonErrorId={jsonErrorId}
      authorizationStored={stored.authorization}
      headersStored={stored.headers}
      updateDraft={updateDraft}
    />
  );
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

      <ServerTransportFields
        t={t}
        edit={edit}
        busy={busy}
        firstField={isEdit ? firstField : undefined}
        jsonErrorId={jsonErrorId}
        stored={storedSecrets(server)}
        updateDraft={updateDraft}
      />

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
          {saveLabel(t, saving, isEdit)}
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
