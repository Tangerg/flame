import { useMemo } from "react";
import { type Translate } from "@flame/component-sdk";
import { projectNumber } from "@frame/sdk/json";
import {
  Badge,
  Button,
  Callout,
  Facts,
  FormActions,
  FormField,
  FormRow,
  Metadata,
  PanelSection,
  TextField,
} from "@flame/ui";
import type { Snapshot } from "./types";
import { LIMITS } from "../contracts/limits";

function ReceiptCapacity({
  value,
  locale,
  t,
}: {
  value: Snapshot["receipts"];
  locale: string;
  t: Translate;
}) {
  const number = useMemo(() => new Intl.NumberFormat(locale), [locale]);
  return (
    <PanelSection title={t("receipts.title")}>
      <Facts
        values={[
          [
            t("receipts.records"),
            `${number.format(projectNumber(value.records))} / ${number.format(projectNumber(value.maxRecords))}`,
          ],
          [
            t("receipts.bytes"),
            `${number.format(projectNumber(value.bytes))} / ${number.format(projectNumber(value.maxBytes))}`,
          ],
          [t("receipts.pending"), number.format(projectNumber(value.pending))],
        ]}
      />
      <Metadata>{t("receipts.help")}</Metadata>
    </PanelSection>
  );
}

export function HostDetails({
  snapshot,
  locale,
  t,
}: {
  snapshot: Snapshot;
  locale: string;
  t: Translate;
}) {
  return (
    <>
      <PanelSection title={t("host.title")}>
        <Facts
          values={[
            [t("host.name"), snapshot.host.hostname],
            [
              t("host.platform"),
              `${snapshot.host.platform} / ${snapshot.host.arch}`,
            ],
            [
              t("host.configuration"),
              t(snapshot.configured ? "host.configured" : "host.missing"),
            ],
          ]}
        />
        <Metadata>{t("host.scope")}</Metadata>
        {!snapshot.configured && (
          <Callout kind="warning">{t("host.setup")}</Callout>
        )}
      </PanelSection>
      <ReceiptCapacity value={snapshot.receipts} locale={locale} t={t} />
    </>
  );
}

export function SessionDetails({
  session,
  active,
  dateTime,
  t,
}: {
  session: NonNullable<Snapshot["session"]>;
  active: boolean;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
}) {
  return (
    <PanelSection
      title={t("session.title")}
      actions={
        <Badge
          tone={
            session.state === "faulted"
              ? "negative"
              : session.state === "ready"
                ? "success"
                : "neutral"
          }
        >
          {t(`session.${session.state}`)}
        </Badge>
      }
    >
      <Facts
        values={[
          [t("session.id"), session.id],
          [t("session.worker"), session.workerId],
          [t("session.opened"), dateTime.format(new Date(session.openedAt))],
          [t("session.expires"), dateTime.format(new Date(session.expiresAt))],
          [t("session.surface"), session.surfaceDigest],
          [
            t("session.reason"),
            session.reason ? t(`reason.${session.reason}`) : undefined,
          ],
        ]}
      />
      {session.operation && (
        <Callout kind="note">
          <Facts
            values={[
              [t("operation.id"), session.operation.id],
              [t("execute.tool"), session.operation.tool],
              [
                t("operation.started"),
                dateTime.format(new Date(session.operation.startedAt)),
              ],
            ]}
          />
        </Callout>
      )}
      {active && <Metadata>{t("close.help")}</Metadata>}
    </PanelSection>
  );
}

export function LeaseForm({
  value,
  invalid,
  disabled,
  onChange,
  onOpen,
  t,
}: {
  value: string;
  invalid: boolean;
  disabled: boolean;
  onChange(value: string): void;
  onOpen(): void;
  t: Translate;
}) {
  return (
    <PanelSection title={t("open.title")} description={t("open.help")}>
      <FormRow>
        <FormField label={t("open.lease")}>
          <TextField
            value={value}
            inputMode="numeric"
            invalid={invalid}
            onChange={(event) => {
              onChange(event.target.value);
            }}
            disabled={disabled}
          />
        </FormField>
      </FormRow>
      {invalid && (
        <p role="alert">
          {t("open.invalid", {
            minimum: String(LIMITS.minLeaseSeconds),
            maximum: String(LIMITS.maxLeaseSeconds),
          })}
        </p>
      )}
      <FormActions label={t("open.title")}>
        <Button variant="primary" disabled={disabled} onClick={onOpen}>
          {t("open.action")}
        </Button>
      </FormActions>
    </PanelSection>
  );
}
