import { useEffect, useId, useMemo, useState } from "react";
import type { PrimitiveSchemaDefinition } from "@modelcontextprotocol/sdk/types.js";
import { failureText, type Translate } from "@flame/component-sdk";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
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
import { PermissionRequest } from "../contracts/elicitation";
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

function choices(field: Field) {
  if (field.type === "string") {
    if ("oneOf" in field)
      return field.oneOf.map((item) => ({
        value: item.const,
        label: item.title,
      }));
    if ("enum" in field)
      return field.enum.map((value, index) => ({
        value,
        label:
          "enumNames" in field ? (field.enumNames?.[index] ?? value) : value,
      }));
  }
  if (field.type === "array") {
    if ("anyOf" in field.items)
      return field.items.anyOf.map((item) => ({
        value: item.const,
        label: item.title,
      }));
    return field.items.enum.map((value) => ({ value, label: value }));
  }
  return undefined;
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
  const options = choices(field);
  const description = field.description;
  let control;
  if (field.type === "array" && options) {
    const selected = Array.isArray(value) ? value : [];
    control = (
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
  } else if (field.type === "boolean") {
    control = (
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
  } else if (options) {
    const selected = options.findIndex((option) => option.value === value);
    control = (
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
  } else {
    const format = "format" in field ? field.format : undefined;
    control = (
      <FormField label={title}>
        <TextField
          id={id}
          type={
            format === "email"
              ? "email"
              : format === "date"
                ? "date"
                : format === "uri"
                  ? "url"
                  : "text"
          }
          inputMode={
            field.type === "number" || field.type === "integer"
              ? "decimal"
              : undefined
          }
          value={typeof value === "string" ? value : ""}
          disabled={disabled}
          required={required}
          aria-describedby={description ? `${id}-description` : undefined}
          onChange={(event) => update(event.target.value)}
        />
      </FormField>
    );
  }
  return (
    <RecordGroup>
      {control}
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
  opener,
  t,
}: {
  approval: Approval;
  pending: boolean;
  respond(input: ReplyInput): Promise<boolean>;
  opener(): HTMLElement | null;
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

  async function answer(
    action: "accept" | "decline" | "cancel",
    persist?: "session" | "always",
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
    <RecordForm aria-label={t("approval.title")} opener={opener}>
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
        {native?.persist.includes("session") && (
          <Button
            disabled={disabled}
            focusableWhenDisabled
            onClick={() => void answer("accept", "session")}
          >
            {t("approval.session")}
          </Button>
        )}
        {native?.persist.includes("always") && (
          <Button
            disabled={disabled}
            focusableWhenDisabled
            onClick={() => void answer("accept", "always")}
          >
            {t("approval.always")}
          </Button>
        )}
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
        <Source value={approval.requestJSON} label={t("approval.original")} />
      </Disclosure>
    </RecordForm>
  );
}
