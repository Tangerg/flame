import {
  Alert,
  CircleInfo,
  Lightbulb,
  MessageAlert,
  ShieldAlert,
} from "@flame/icons/react";
import type { ReactNode } from "react";
import { Icon, type IconGlyph } from "./font-scale";

const CALLOUT = {
  note: { title: "Note", glyph: CircleInfo },
  tip: { title: "Tip", glyph: Lightbulb },
  important: { title: "Important", glyph: MessageAlert },
  warning: { title: "Warning", glyph: Alert },
  caution: { title: "Caution", glyph: ShieldAlert },
} as const satisfies Record<string, { title: string; glyph: IconGlyph }>;

export type CalloutKind = keyof typeof CALLOUT;

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
  const { title, glyph } = CALLOUT[kind];
  return (
    <div data-slot="callout" data-callout={kind} role="note" dir="auto">
      <p data-slot="callout-title" dir="auto">
        <Icon glyph={glyph} size="sm" aria-hidden />
        {title}
      </p>
      {children}
    </div>
  );
}
