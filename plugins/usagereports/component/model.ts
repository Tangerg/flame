import {
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import { type ScrollPosition } from "@flame/component-sdk";
import type { JSONNumber } from "@frame/sdk/json";
import { METHODS } from "../contracts/methods";
import { type MethodInputs, type MethodOutputs } from "./methods.generated";

export type Header = MethodOutputs[typeof METHODS.list]["reports"][number];
export type Report = MethodOutputs[typeof METHODS.get];
export type Period = MethodInputs[typeof METHODS.capture]["period"];
export type BoundContext = ComponentContext & {
  readonly binding: NonNullable<ComponentContext["binding"]>;
};
export type ArchiveRead = {
  loading: boolean;
  accepted?: Header[];
  error?: string;
};
export type OpenedReport = {
  header: Header;
  loading: boolean;
  accepted?: Report;
  error?: string;
};
export type ArchiveReturn = { opener?: HTMLElement; scroll: ScrollPosition[] };
export type Action =
  { kind: "capture" | "removal" } | { kind: "check" | "retire"; key: string };
export type Outcome =
  | {
      kind: "saved" | "removed" | "removalConfirmed" | "unknown" | "retired";
    }
  | { kind: "savedArchived"; header: Header }
  | {
      kind:
        | "archiveFailed"
        | "captureRejected"
        | "removalRejected"
        | "retireFailed";
      error: string;
    };
export const PERIODS = ["daily", "weekly"] as const satisfies readonly Period[];
export type Formatters = {
  count: (value?: JSONNumber) => string;
  amount: (value?: JSONNumber) => string;
};
export function makeFormatters(locale: string, t: Translate): Formatters {
  const number = new Intl.NumberFormat(locale);
  const dollars = new Intl.NumberFormat(locale, { maximumFractionDigits: 6 });
  // Six places would round a nonzero sub-micro charge into a recorded zero.
  const tiny = new Intl.NumberFormat(locale, { maximumSignificantDigits: 6 });
  const format = (
    value: JSONNumber | undefined,
    formatter: Intl.NumberFormat,
  ) =>
    value === undefined
      ? t("cost.notReported")
      : typeof value === "object"
        ? value.toString()
        : formatter.format(value);
  return {
    count: (value) => format(value, number),
    amount: (value) =>
      typeof value === "number" && value !== 0 && Math.abs(value) < 0.000001
        ? tiny.format(value)
        : format(value, dollars),
  };
}
export const failure = (error: unknown) => failureText(error, String(error));
export const observed = (header: Header, locale: string) =>
  new Date(header.observedAt).toLocaleString(locale);
export const lowerBound = (header: Header, t: Translate, locale: string) =>
  header.since
    ? new Date(header.since).toLocaleString(locale)
    : t("evidence.beginning");
export const reportTitle = (header: Header, t: Translate, locale: string) =>
  t("report.title", {
    period: t(`period.${header.period}.label`),
    observed: observed(header, locale),
  });
