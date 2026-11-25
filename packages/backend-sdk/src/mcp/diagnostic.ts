import { types } from "node:util";

export type FailurePhase =
  | "startup"
  | "service"
  | "uncaught exception"
  | "unhandled rejection"
  | "cleanup";

const maximumDiagnosticBytes = 8 << 10;
const maximumErrors = 16;
const maximumFrames = 8;
const truncated = "  [diagnostic truncated]\n";
const sourceFrame =
  /^\s+at (?:.* \()?((?:file:\/\/\/|\/|[A-Za-z]:[\\/]|node:)[^()\r\n]+):(\d+):(\d+)\)?$/;

// Only frame locations leave the stack; a source that could forge log lines is withheld.
function sourceFrames(error: Error): string[] {
  const heading = error.message
    ? `${error.name}: ${error.message}\n`
    : `${error.name}\n`;
  const stack = error.stack;
  if (typeof stack !== "string" || !stack.startsWith(heading)) return [];
  // Removing the entire heading also removes multiline exception messages.
  return stack
    .slice(heading.length, heading.length + maximumDiagnosticBytes)
    .split("\n")
    .slice(0, maximumFrames)
    .flatMap((frame) => {
      const location = sourceFrame.exec(frame);
      if (!location) return [];
      const source = location[1]!.startsWith("file:///")
        ? location[1]!.split(/[?#]/, 1)[0]!
        : location[1]!;
      return /[\p{Cc}\p{Surrogate}\p{Zl}\p{Zp}]/u.test(source)
        ? ["    at [unprintable source]\n"]
        : [`    at ${source}:${location[2]}:${location[3]}\n`];
    });
}

// Exception text and arbitrary properties can contain requests or credentials.
// Only error categories, codes and source locations cross this log boundary.
export function failureDiagnostic(
  phase: FailurePhase,
  failure: unknown,
): string {
  const lines: string[] = [];
  const seen = new Set<Error>();
  let remaining = maximumDiagnosticBytes - Buffer.byteLength(truncated);
  let errors = 0;
  let limited = false;
  const append = (text: string): void => {
    const bytes = Buffer.byteLength(text);
    if (bytes > remaining) {
      limited = true;
      return;
    }
    remaining -= bytes;
    lines.push(text);
  };
  const visit = (value: unknown, relation: string): void => {
    if (errors === maximumErrors || remaining === 0) {
      limited = true;
      return;
    }
    errors++;
    if (!types.isNativeError(value)) {
      append(`  ${relation}: non-error thrown value\n`);
      return;
    }
    if (seen.has(value)) {
      append(`  ${relation}: [already reported]\n`);
      return;
    }
    seen.add(value);
    const category = errorCategory(value);
    append(`  ${relation}: ${category}\n`);

    for (const frame of sourceFrames(value)) append(frame);
    const cause = Object.getOwnPropertyDescriptor(value, "cause");
    if (cause && "value" in cause) visit(cause.value, "caused by");
    const members: unknown = Object.getOwnPropertyDescriptor(
      value,
      "errors",
    )?.value;
    if (value instanceof AggregateError && Array.isArray(members)) {
      for (let index = 0; index < members.length; index++) {
        if (errors === maximumErrors) {
          limited = true;
          break;
        }
        visit(members[index], `aggregate[${index}]`);
      }
    }
  };
  append(`plugin: backend ${phase} failed\n`);
  try {
    visit(failure, "error");
  } catch {
    append("  error metadata could not be read\n");
  }
  if (limited) lines.push(truncated);
  return lines.join("");
}

function errorCategory(value: Error): string {
  const name =
    typeof value.name === "string" &&
    /^(?:Error|[A-Za-z][A-Za-z0-9]{0,58}Error)$/.test(value.name)
      ? value.name
      : "Error";
  const code: unknown = Object.getOwnPropertyDescriptor(value, "code")?.value;
  const category =
    typeof code === "string" && /^(?:ERR_|SQLITE_|E)[A-Z0-9_]{1,64}$/.test(code)
      ? `${name} [${code}]`
      : name;
  return category;
}
