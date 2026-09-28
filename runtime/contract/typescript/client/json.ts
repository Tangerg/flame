import { printParseErrorCode, visit } from "jsonc-parser";

// Unicode mode consumes a valid surrogate pair as one code point.
const unpairedSurrogate = /\p{Surrogate}/u;

export function parseWireJSON(text: string): unknown {
  const objects: Array<Set<string>> = [];
  visit(
    text,
    {
      onObjectBegin() {
        objects.push(new Set());
      },
      onObjectProperty(name, offset) {
        if (unpairedSurrogate.test(name)) {
          throw new SyntaxError("invalid Unicode in JSON member at offset " + offset);
        }
        const members = objects.at(-1)!;
        if (members.has(name)) {
          throw new SyntaxError("duplicate JSON member at offset " + offset);
        }
        members.add(name);
      },
      onObjectEnd() {
        objects.pop();
      },
      onLiteralValue(value: unknown, offset) {
        if (typeof value === "string" && unpairedSurrogate.test(value)) {
          throw new SyntaxError("invalid Unicode in JSON string at offset " + offset);
        }
      },
      onError(code, offset) {
        throw new SyntaxError(printParseErrorCode(code) + " at offset " + offset);
      },
    },
    { disallowComments: true, allowTrailingComma: false, allowEmptyContent: false },
  );
  // Native construction preserves JSON number and "__proto__" member semantics
  // after the visitor checks occurrences that JSON.parse would discard.
  return JSON.parse(text) as unknown;
}
