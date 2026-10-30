import {
  Alert,
  CircleInfo,
  Lightbulb,
  MessageAlert,
  ShieldAlert,
} from "@flame/icons/react";
import { createContext, useContext, type ReactNode } from "react";
import { Icon, type IconGlyph } from "./font-scale";

const CALLOUT = {
  note: CircleInfo,
  tip: Lightbulb,
  important: MessageAlert,
  warning: Alert,
  caution: ShieldAlert,
} as const satisfies Record<string, IconGlyph>;

export type CalloutKind = keyof typeof CALLOUT;

export type CalloutLabels = Readonly<Record<CalloutKind, string>>;

const CalloutLabelsContext = createContext<CalloutLabels | null>(null);

export function CalloutLabelsProvider({
  labels,
  children,
}: {
  labels: CalloutLabels;
  children: ReactNode;
}) {
  for (const kind of Object.keys(CALLOUT) as CalloutKind[]) {
    if (typeof labels[kind] !== "string" || !labels[kind].trim()) {
      throw new Error(`callout label is missing for ${kind}`);
    }
  }
  return <CalloutLabelsContext value={labels}>{children}</CalloutLabelsContext>;
}

const MARKER = /^\[!([a-z]+)\](?:[ \t]+|\r?\n|$)(?:\r?\n)?/i;

export function readCalloutMarker(
  text: string,
): { kind: CalloutKind; rest: string } | null {
  const match = MARKER.exec(text);
  if (!match) return null;
  const kind = match[1]!.toLowerCase();
  if (!Object.hasOwn(CALLOUT, kind)) return null;
  return {
    kind: kind as CalloutKind,
    rest: text.slice(match[0].length),
  };
}

export function Callout({
  kind,
  children,
}: {
  kind: CalloutKind;
  children: ReactNode;
}) {
  const labels = useContext(CalloutLabelsContext);
  if (!labels) throw new Error("callout labels provider is missing");
  const glyph = CALLOUT[kind];
  return (
    <div data-slot="callout" data-callout={kind} role="note" dir="auto">
      <p data-slot="callout-title" dir="auto">
        <Icon glyph={glyph} size="sm" aria-hidden />
        {labels[kind]}
      </p>
      {children}
    </div>
  );
}
