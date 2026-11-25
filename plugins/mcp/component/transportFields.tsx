import { type RefObject } from "react";
import * as token from "@flame/ui/tokens.stylex";
import type { Translate } from "@flame/component-sdk";
import {
  FormField,
  FormGroup,
  FormRow,
  Switch,
  TextArea,
  TextField,
} from "@flame/ui";
import { MCPServerEdit, RetainedValue, type MCPServerFields } from "./draft";

const split = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  gap: token.layout.contentGap,
} as const;

// Input examples are protocol values, not prose: they are written in the exact
// JSON the draft parser accepts and stay identical in every locale.
export const EXAMPLE = {
  command: "npx",
  args: JSON.stringify([
    "-y",
    "@modelcontextprotocol/server-filesystem",
    "/path",
  ]),
  environment: JSON.stringify({ API_KEY: "…" }),
  url: "https://example.com/mcp",
  headers: JSON.stringify({ "X-API-Key": "…" }),
};

// Credentials, commands and JSON are typed literally, never corrected.
export const LITERAL_INPUT = {
  autoCapitalize: "none",
  autoCorrect: "off",
  spellCheck: false,
} as const;

export function FieldIssue({
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
    <span id={id} role="alert" style={{ color: token.color.negative }}>
      {message}
    </span>
  );
}

function FieldWarning({ message }: { message: string }) {
  return <span style={{ color: token.color.warning }}>{message}</span>;
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

export interface TransportFieldsProps {
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

export function StdioFields({
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

export function HttpFields({
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
