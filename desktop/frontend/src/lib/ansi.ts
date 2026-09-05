// Hand-written rather than `anser` / `ansi-to-html`: those emit literal colours, which cannot
// follow the scheme or a contributed theme. This emits spans plus a TONE.
//
// In `lib` because two contexts read command output — the transcript's tool panel and the
// workspace terminal — and neither owns what an SGR code means. It lived under the chat
// plugin's domain ring, out of the terminal view's reach, which is why that view rendered
// escape codes as text for as long as it did.

export type AnsiTone = "negative" | "success" | "warning" | "info" | "accent" | "muted";

export interface AnsiSpan {
  text: string;
  tone?: AnsiTone;
  bold?: boolean;
  dim?: boolean;
  underline?: boolean;
}

// The eight SGR colours onto this app's tones. Cyan and magenta borrow `info` / `accent` to
// stay on the ramp; bright variants (90-97) share their base tone and differ by weight.
const TONE_BY_SGR: Record<number, AnsiTone> = {
  30: "muted",
  31: "negative",
  32: "success",
  33: "warning",
  34: "info",
  35: "accent",
  36: "info",
  37: "muted",
};

// oxlint-disable-next-line no-control-regex -- the escape byte is the thing being matched
const CSI = /\u001b\[([0-9;?]*)([A-Za-z])/g;

interface Style {
  tone?: AnsiTone;
  bold?: boolean;
  dim?: boolean;
  underline?: boolean;
}

function applySgr(style: Style, params: string): Style {
  // An empty parameter list means SGR 0.
  const codes = params === "" ? [0] : params.split(";").map((p) => Number.parseInt(p, 10) || 0);
  let next = { ...style };
  for (let i = 0; i < codes.length; i += 1) {
    const code = codes[i]!;
    if (code === 0) next = {};
    else if (code === 1) next.bold = true;
    else if (code === 2) next.dim = true;
    else if (code === 4) next.underline = true;
    else if (code === 22) {
      next.bold = undefined;
      next.dim = undefined;
    } else if (code === 24) next.underline = undefined;
    else if (code === 39) next.tone = undefined;
    else if (code in TONE_BY_SGR) next.tone = TONE_BY_SGR[code];
    else if (code >= 90 && code <= 97) next.tone = TONE_BY_SGR[code - 60];
    // 256-colour and truecolour selectors carry arguments; skip them rather than read the
    // arguments as further codes.
    else if (code === 38 || code === 48) i += codes[i + 1] === 5 ? 2 : 4;
  }
  return next;
}

/** Cursor moves and erases are DROPPED: a transcript has no cursor, and a progress bar
 *  redrawing with `\r` would otherwise stack every intermediate frame. */
export function parseAnsi(input: string): AnsiSpan[] {
  const spans: AnsiSpan[] = [];
  let style: Style = {};
  let cursor = 0;

  const push = (text: string) => {
    if (text === "") return;
    const last = spans[spans.length - 1];
    if (
      last &&
      last.tone === style.tone &&
      last.bold === style.bold &&
      last.dim === style.dim &&
      last.underline === style.underline
    ) {
      last.text += text;
      return;
    }
    spans.push({ text, ...style });
  };

  CSI.lastIndex = 0;
  for (let match = CSI.exec(input); match !== null; match = CSI.exec(input)) {
    push(input.slice(cursor, match.index));
    if (match[2] === "m") style = applySgr(style, match[1] ?? "");
    cursor = match.index + match[0].length;
  }
  push(input.slice(cursor));
  return spans;
}

/** Cheap pre-check so a caller can skip the parse for the common plain case. */
export function hasAnsi(input: string): boolean {
  return input.includes("\u001b");
}
