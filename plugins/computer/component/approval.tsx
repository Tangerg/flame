import { useEffect, useId, useMemo, useRef, useState } from "react";
import type { PrimitiveSchemaDefinition } from "@modelcontextprotocol/sdk/types.js";
import { failureText, type Translate } from "@flame/component-sdk";
import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import {
  Button,
  Callout,
  Checkbox,
  Disclosure,
  Facts,
  FormField,
  FormSection,
  Metadata,
  RecordBody,
  RecordForm,
  RecordGroup,
  Select,
  TextField,
  Toolbar,
} from "@flame/ui";
import {
  fieldChoices,
  PermissionRequest,
  type FieldChoice,
  type PermissionPersistence,
} from "../contracts/elicitation";
import type { Approval, ReplyInput } from "./types";
import { externalURL, Source } from "./result";

type DraftValue = string | boolean | string[];
type Field = PrimitiveSchemaDefinition;

function initialValues(request: PermissionRequest): Record<string, DraftValue> {
  if (request.request.mode === "url") return {};
  return Object.fromEntries(
    Object.entries(request.request.requestedSchema.properties)
      .filter(([, field]) => field.default !== undefined)
      .map(([key, field]) => [
        key,
        typeof field.default === "number"
          ? String(field.default)
          : field.default!,
      ]),
  );
}

const PERSISTENCE_LABEL: Record<PermissionPersistence, string> = {
  session: "approval.session",
  always: "approval.always",
};

// date-time stays a text field: the contract requires an offset that
// datetime-local cannot express.
const FORMAT_INPUT_TYPE = {
  email: "email",
  uri: "url",
  date: "date",
  "date-time": "text",
} as const;

interface ControlProps<Value extends DraftValue> {
  title: string;
  value: DraftValue | undefined;
  update(value: Value | undefined): void;
  disabled: boolean;
}

function ChoiceSet({
  title,
  options,
  value,
  update,
  disabled,
}: ControlProps<string[]> & { options: FieldChoice[] }) {
  const selected = Array.isArray(value) ? value : [];
  return (
    <FormSection title={title}>
      {options.map((option) => (
        <Checkbox
          key={option.value}
          label={option.label}
          checked={selected.includes(option.value)}
          disabled={disabled}
          onCheckedChange={(checked) =>
            update(
              checked
                ? [...selected, option.value]
                : selected.filter((item) => item !== option.value),
            )
          }
        />
      ))}
    </FormSection>
  );
}

function BooleanSelect({
  title,
  value,
  update,
  disabled,
  t,
}: ControlProps<boolean> & { t: Translate }) {
  return (
    <Select
      label={title}
      value={value === undefined ? "unset" : value ? "true" : "false"}
      disabled={disabled}
      options={[
        { value: "unset", label: t("approval.choose") },
        { value: "true", label: t("approval.true") },
        { value: "false", label: t("approval.false") },
      ]}
      onValueChange={(selected) =>
        update(selected === "unset" ? undefined : selected === "true")
      }
    />
  );
}

function ChoiceSelect({
  title,
  options,
  value,
  update,
  disabled,
  t,
}: ControlProps<string> & { options: FieldChoice[]; t: Translate }) {
  const selected = options.findIndex((option) => option.value === value);
  return (
    <Select
      label={title}
      value={selected < 0 ? "" : String(selected)}
      disabled={disabled}
      options={[
        { value: "", label: t("approval.choose") },
        ...options.map((option, index) => ({
          value: String(index),
          label: option.label,
        })),
      ]}
      onValueChange={(selected) =>
        update(selected === "" ? undefined : options[Number(selected)]!.value)
      }
    />
  );
}

function TextControl({
  id,
  field,
  title,
  value,
  update,
  disabled,
  required,
  described,
}: ControlProps<string> & {
  id: string;
  field: Field;
  required: boolean;
  described: boolean;
}) {
  const format = "format" in field ? field.format : undefined;
  return (
    <FormField label={title}>
      <TextField
        id={id}
        type={format ? FORMAT_INPUT_TYPE[format] : "text"}
        inputMode={
          field.type === "number" || field.type === "integer"
            ? "decimal"
            : undefined
        }
        value={typeof value === "string" ? value : ""}
        disabled={disabled}
        required={required}
        aria-describedby={described ? `${id}-description` : undefined}
        onChange={(event) => update(event.target.value)}
      />
    </FormField>
  );
}

function PermissionField({
  name,
  field,
  required,
  value,
  update,
  disabled,
  t,
}: {
  name: string;
  field: Field;
  required: boolean;
  value: DraftValue | undefined;
  update(value: DraftValue | undefined): void;
  disabled: boolean;
  t: Translate;
}) {
  const id = useId();
  const title =
    (field.title ?? name) + (required ? t("approval.required") : "");
  const options = fieldChoices(field);
  const description = field.description;
  const control = { title, value, update, disabled };
  return (
    <RecordGroup>
      {field.type === "array" && options ? (
        <ChoiceSet {...control} options={options} />
      ) : field.type === "boolean" ? (
        <BooleanSelect {...control} t={t} />
      ) : options ? (
        <ChoiceSelect {...control} options={options} t={t} />
      ) : (
        <TextControl
          {...control}
          id={id}
          field={field}
          required={required}
          described={description !== undefined}
        />
      )}
      {description && (
        <Metadata id={`${id}-description`}>{description}</Metadata>
      )}
    </RecordGroup>
  );
}

export function ApprovalForm({
  approval,
  pending,
  respond,
  beforeRespond,
  t,
}: {
  approval: Approval;
  pending: boolean;
  respond(input: ReplyInput): Promise<boolean>;
  beforeRespond(source: HTMLElement): void;
  t: Translate;
}) {
  const permission = useMemo(
    () => new PermissionRequest(approval.requestJSON),
    [approval.requestJSON],
  );
  const [draft, setDraft] = useState(() => initialValues(permission));
  const [error, setError] = useState<string>();
  const [submitted, setSubmitted] = useState(false);
  const lifetime = useMemo(() => new AbortController(), []);
  useEffect(() => () => lifetime.abort(), [lifetime]);
  const request = permission.request;
  const native = permission.native;
  const disabled = pending || submitted;
  const form = useRef<HTMLElement>(null);

  async function answer(
    action: "accept" | "decline" | "cancel",
    persist?: PermissionPersistence,
  ) {
    if (disabled) return;
    setError(undefined);
    try {
      const content: Record<string, unknown> = {};
      if (action === "accept" && request.mode !== "url") {
        for (const [key, value] of Object.entries(draft)) {
          const field = request.requestedSchema.properties[key]!;
          if (field.type === "number" || field.type === "integer") {
            if (value === "") continue;
            content[key] = parseJSON(String(value));
          } else content[key] = value;
        }
      }
      const responseJSON = stringifyJSON({
        action,
        ...(action === "accept" && request.mode !== "url" ? { content } : {}),
        ...(persist ? { _meta: { persist } } : {}),
      });
      permission.reply(responseJSON);
      if (form.current) beforeRespond(form.current);
      if (
        (await respond({
          sessionId: approval.sessionId,
          requestId: approval.id,
          responseJSON,
        })) &&
        !lifetime.signal.aborted
      )
        setSubmitted(true);
    } catch (cause) {
      if (!lifetime.signal.aborted) setError(failureText(cause));
    }
  }

  return (
    <RecordForm ref={form} aria-label={t("approval.title")}>
      <Callout kind="important">
        <RecordBody>{request.message}</RecordBody>
      </Callout>
      <Facts
        values={[
          [t("approval.request"), approval.id],
          [t("approval.operation"), approval.operationId],
          [t("approval.app"), native?.app],
          [t("approval.risk"), native?.riskLevel],
        ]}
      />
      {native?.subtitle && <Callout kind="warning">{native.subtitle}</Callout>}
      {error && (
        <div role="alert">
          <RecordBody>{error}</RecordBody>
        </div>
      )}
      {submitted && <p role="status">{t("approval.sent")}</p>}
      {request.mode === "url" ? (
        <RecordGroup>
          <Metadata>{t("approval.urlHelp")}</Metadata>
          {externalURL(request.url) ? (
            <a
              href={externalURL(request.url)}
              target="_blank"
              rel="noopener noreferrer"
            >
              {request.url}
            </a>
          ) : (
            <RecordBody>{request.url}</RecordBody>
          )}
        </RecordGroup>
      ) : (
        !native && (
          <FormSection title={t("approval.fields")}>
            {Object.entries(request.requestedSchema.properties).map(
              ([name, field]) => (
                <PermissionField
                  key={name}
                  name={name}
                  field={field}
                  value={draft[name]}
                  required={
                    request.requestedSchema.required?.includes(name) ?? false
                  }
                  disabled={disabled}
                  update={(value) =>
                    setDraft((previous) => {
                      const next = { ...previous };
                      if (value === undefined) delete next[name];
                      else next[name] = value;
                      return next;
                    })
                  }
                  t={t}
                />
              ),
            )}
          </FormSection>
        )
      )}
      <Toolbar label={t("approval.actions")}>
        <Button
          variant="primary"
          disabled={disabled}
          focusableWhenDisabled
          onClick={() => void answer("accept")}
        >
          {t(native ? "approval.once" : "approval.accept")}
        </Button>
        {native?.persist.map((scope) => (
          <Button
            key={scope}
            disabled={disabled}
            focusableWhenDisabled
            onClick={() => void answer("accept", scope)}
          >
            {t(PERSISTENCE_LABEL[scope])}
          </Button>
        ))}
        <Button
          disabled={disabled}
          focusableWhenDisabled
          onClick={() => void answer("decline")}
        >
          {t("approval.decline")}
        </Button>
        <Button
          disabled={disabled}
          focusableWhenDisabled
          onClick={() => void answer("cancel")}
        >
          {t("approval.cancel")}
        </Button>
      </Toolbar>
      <Disclosure title={t("approval.original")}>
        <Source
          value={approval.requestJSON}
          label={t("approval.original")}
          t={t}
        />
      </Disclosure>
    </RecordForm>
  );
}
