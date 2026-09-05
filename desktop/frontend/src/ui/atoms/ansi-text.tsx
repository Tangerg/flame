import { type AnsiSpan, type AnsiTone, parseAnsi } from "@/lib/ansi";
import { cn } from "@/lib/classNames";

/** What an SGR colour means HERE. The parser answers in tones so this stays the one place a
 *  tone becomes a token — two surfaces render command output, and a second copy of this map is
 *  a second answer to "what colour is a failure". */
const TONE_CLASS: Record<AnsiTone, string> = {
  negative: "text-negative",
  success: "text-success",
  warning: "text-warning",
  info: "text-info",
  accent: "text-accent",
  muted: "text-fg-faint",
};

function spanClass(span: AnsiSpan): string | undefined {
  return (
    cn(
      span.tone && TONE_CLASS[span.tone],
      span.bold && "font-semibold",
      span.dim && "opacity-70",
      span.underline && "underline",
    ) || undefined
  );
}

/** One line of terminal output, with its escape codes read as tone rather than printed. */
export function AnsiText({ text }: { text: string }) {
  return (
    <>
      {parseAnsi(text).map((span, index) => (
        <span key={index} className={spanClass(span)}>
          {span.text}
        </span>
      ))}
    </>
  );
}
