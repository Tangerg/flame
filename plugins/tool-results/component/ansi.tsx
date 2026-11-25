import * as token from "@flame/ui/tokens.stylex";
import type { CSSProperties } from "react";

const INK = {
  muted: token.color.fgMuted,
  negative: token.color.negative,
  success: token.color.success,
  warning: token.color.warning,
  accent: token.color.accentText,
} as const;

type AnsiStyle = {
  tone?: keyof typeof INK;
  bold?: boolean;
  dim?: boolean;
  underline?: boolean;
};
const tones: readonly (keyof typeof INK)[] = [
  "muted",
  "negative",
  "success",
  "warning",
  "accent",
  "accent",
  "accent",
  "muted",
];

// eslint-disable-next-line no-control-regex -- Recorded terminal output contains ANSI escape bytes.
const ANSI_SEQUENCE = /\u001b\[([0-9;?]*)([A-Za-z])/g;

export function ansiText(input: string): string {
  return input.replace(ANSI_SEQUENCE, "");
}

// Select Graphic Rendition codes Flame presents; others leave the style unchanged.
const SGR: Readonly<Record<number, (style: AnsiStyle) => AnsiStyle>> = {
  0: () => ({}),
  1: (style) => ({ ...style, bold: true }),
  2: (style) => ({ ...style, dim: true }),
  4: (style) => ({ ...style, underline: true }),
  22: (style) => ({ ...style, bold: false, dim: false }),
  24: (style) => ({ ...style, underline: false }),
  39: (style) => ({ ...style, tone: undefined }),
};

function toneCode(code: number): keyof typeof INK | undefined {
  if (code >= 30 && code <= 37) return tones[code - 30];
  if (code >= 90 && code <= 97) return tones[code - 90];
  return undefined;
}

function sgr(style: AnsiStyle, params: string): AnsiStyle {
  const codes =
    params === ""
      ? [0]
      : params.split(";").map((code) => Number.parseInt(code, 10) || 0);
  let next = { ...style };
  for (let index = 0; index < codes.length; index++) {
    const code = codes[index]!;
    const effect = SGR[code];
    const tone = toneCode(code);
    if (effect) next = effect(next);
    else if (tone) next.tone = tone;
    // Extended colours carry their own arguments: 5;n or 2;r;g;b.
    else if (code === 38 || code === 48)
      index += codes[index + 1] === 5 ? 2 : 4;
  }
  return next;
}

function* ansiSpans(input: string, initialStyle: AnsiStyle = {}) {
  let style = initialStyle;
  let cursor = 0;
  for (const match of input.matchAll(ANSI_SEQUENCE)) {
    if (match.index > cursor)
      yield { text: input.slice(cursor, match.index), style };
    if (match[2] === "m") style = sgr(style, match[1]!);
    cursor = match.index + match[0].length;
  }
  if (cursor < input.length) yield { text: input.slice(cursor), style };
}

// A rendered window can omit the line that established the captured ANSI state.
export function ansiLineStyles(input: string) {
  const initial: AnsiStyle = {};
  const styles = [initial];
  for (const { text, style } of ansiSpans(input, initial)) {
    for (
      let index = text.indexOf("\n");
      index !== -1;
      index = text.indexOf("\n", index + 1)
    )
      styles.push(style);
  }
  return styles;
}

// ANSI determines local text style only; result bytes never become markup or URLs.
export function renderAnsi(input: string, initialStyle: AnsiStyle = {}) {
  return Array.from(
    ansiSpans(input, initialStyle),
    ({ text, style: value }, index) => {
      const style: CSSProperties = {
        color: value.tone && INK[value.tone],
        fontWeight: value.bold ? "bold" : undefined,
        opacity: value.dim ? 0.7 : undefined,
        textDecoration: value.underline ? "underline" : undefined,
      };
      return (
        <span key={index} style={style}>
          {text}
        </span>
      );
    },
  );
}
