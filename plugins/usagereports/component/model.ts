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
      kind: "saved" | "removed" | "unknown" | "retired";
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
  time: (iso: string) => string;
  timeWithSeconds: (iso: string) => string;
};
export function makeFormatters(locale: string, t: Translate): Formatters {
  const number = new Intl.NumberFormat(locale);
  const dollars = new Intl.NumberFormat(locale, { maximumFractionDigits: 6 });
  // Six places would round a nonzero sub-micro charge into a recorded zero.
  const tiny = new Intl.NumberFormat(locale, { maximumSignificantDigits: 6 });
  const dateTime = new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  });
  const dateTimeWithSeconds = new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  });
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
    time: (iso) => dateTime.format(new Date(iso)),
    timeWithSeconds: (iso) => dateTimeWithSeconds.format(new Date(iso)),
  };
}
export const failure = (error: unknown) => failureText(error, String(error));
export const observed = (header: Header, fmt: Formatters) =>
  fmt.time(header.observedAt);
export const lowerBound = (header: Header, t: Translate, fmt: Formatters) =>
  header.since ? fmt.time(header.since) : t("evidence.beginning");
export function reportTitles(
  headers: readonly (Header | undefined)[],
  t: Translate,
  fmt: Formatters,
) {
  const byId = new Map<string, Header>();
  for (const header of headers) if (header) byId.set(header.id, header);
  const byMinute = Map.groupBy(
    byId.values(),
    (header) => `${header.period}:${observed(header, fmt)}`,
  );
  const observations = new Map<string, string>();
  for (const sameMinute of byMinute.values()) {
    if (sameMinute.length === 1) continue;
    const bySecond = Map.groupBy(sameMinute, ({ observedAt }) =>
      fmt.timeWithSeconds(observedAt),
    );
    for (const [time, sameSecond] of bySecond) {
      if (sameSecond.length === 1) {
        observations.set(sameSecond[0]!.id, time);
        continue;
      }
      for (const { id } of sameSecond) {
        const tail = id.slice(-8);
        const tied = sameSecond.some(
          (other) => other.id !== id && other.id.endsWith(tail),
        );
        observations.set(id, `${time} · ${tied ? id : tail}`);
      }
    }
  }
  return (
    header: Header,
    message: "report.title" | "row.title" = "report.title",
  ) =>
    t(message, {
      period: t(`period.${header.period}.label`),
      observed: observations.get(header.id) ?? observed(header, fmt),
    });
}

export type ReportTitleFor = ReturnType<typeof reportTitles>;
