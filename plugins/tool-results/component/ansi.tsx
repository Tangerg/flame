import type { CSSProperties } from "react";

const INK = {
  muted: "var(--color-text-muted)",
  negative: "var(--color-negative)",
  success: "var(--color-success)",
  warning: "var(--color-warning)",
  accent: "var(--color-accent-text)",
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

function sgr(style: AnsiStyle, params: string): AnsiStyle {
  const codes =
    params === ""
      ? [0]
      : params.split(";").map((code) => Number.parseInt(code, 10) || 0);
  let next = { ...style };
  for (let index = 0; index < codes.length; index++) {
    const code = codes[index]!;
    if (code === 0) next = {};
    else if (code === 1) next.bold = true;
    else if (code === 2) next.dim = true;
    else if (code === 4) next.underline = true;
    else if (code === 22) {
      next.bold = false;
      next.dim = false;
    } else if (code === 24) next.underline = false;
    else if (code === 39) next.tone = undefined;
    else if (code >= 30 && code <= 37) next.tone = tones[code - 30];
    else if (code >= 90 && code <= 97) next.tone = tones[code - 90];
    else if (code === 38 || code === 48)
      index += codes[index + 1] === 5 ? 2 : 4;
  }
  return next;
}

// ANSI determines local text style only; result bytes never become markup or URLs.
export function renderAnsi(input: string) {
  const spans: { text: string; style: AnsiStyle }[] = [];
  let style: AnsiStyle = {};
  let cursor = 0;
  for (const match of input.matchAll(ANSI_SEQUENCE)) {
    if (match.index > cursor)
      spans.push({ text: input.slice(cursor, match.index), style });
    if (match[2] === "m") style = sgr(style, match[1]!);
    cursor = match.index + match[0].length;
  }
  if (cursor < input.length) spans.push({ text: input.slice(cursor), style });
  return spans.map(({ text, style: value }, index) => {
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
  });
}
