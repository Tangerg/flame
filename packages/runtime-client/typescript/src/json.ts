import { parseTree, printParseErrorCode, visit, type Node } from "jsonc-parser";

// Unicode mode consumes a valid surrogate pair as one code point.
const unpairedSurrogate = /\p{Surrogate}/u;

function validateJSON(text: string): void {
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
}

export function parseWireJSON(text: string): unknown {
  validateJSON(text);
  return JSON.parse(text) as unknown;
}

// A feature owns arbitrary numeric ranges and receipt lexemes. Values that a
// JS number cannot round-trip stay textual without expanding huge exponents.
export class ExactNumber {
  readonly text: string;

  constructor(text: string) {
    validateJSON(text);
    const node = parseTree(text)!;
    if (node.type !== "number" || node.offset !== 0 || node.length !== text.length)
      throw new TypeError("exact number requires one JSON number literal");
    this.text = text;
    Object.freeze(this);
  }

  toString(): string {
    return this.text;
  }

  toJSON(): never {
    throw new TypeError("exact JSON numbers require stringifyJSON");
  }
}

export type JSONNumber = number | bigint | ExactNumber;

export function parseJSON(text: string): unknown {
  validateJSON(text);
  function read(node: Node): unknown {
    if (node.type === "number") {
      const raw = text.slice(node.offset, node.offset + node.length);
      const value = node.value as number;
      if (
        Number.isFinite(value) &&
        (!Number.isInteger(value) || Number.isSafeInteger(value)) &&
        JSON.stringify(value) === raw
      )
        return value;
      if (/^-?\d+$/.test(raw) && raw !== "-0") return BigInt(raw);
      return new ExactNumber(raw);
    }
    if (node.type === "array") return (node.children ?? []).map(read);
    if (node.type === "object") {
      const value: Record<string, unknown> = {};
      for (const property of node.children ?? []) {
        Object.defineProperty(value, property.children![0]!.value as string, {
          value: read(property.children![1]!),
          enumerable: true,
          configurable: true,
          writable: true,
        });
      }
      return value;
    }
    return node.value as unknown;
  }
  return read(parseTree(text)!);
}

export function stringifyJSON(value: unknown, sortKeys = false): string {
  const ancestors = new Set<object>();
  function write(value: unknown): string | undefined {
    if (value === null) return "null";
    if (typeof value === "string" || typeof value === "boolean") return JSON.stringify(value);
    if (typeof value === "bigint") return value.toString();
    if (typeof value === "number") {
      if (!Number.isFinite(value) || (Number.isInteger(value) && !Number.isSafeInteger(value)))
        throw new TypeError("unsafe JSON number; use bigint or ExactNumber");
      return JSON.stringify(value);
    }
    if (value === undefined) return undefined;
    if (typeof value !== "object") throw new TypeError("unsupported JSON value");
    if (value instanceof ExactNumber) return value.text;
    if (value instanceof Date) return JSON.stringify(value.toJSON());
    if (
      !Array.isArray(value) &&
      Object.getPrototypeOf(value) !== Object.prototype &&
      Object.getPrototypeOf(value) !== null
    )
      throw new TypeError("JSON object must be a plain record");
    if (ancestors.has(value)) throw new TypeError("cyclic JSON value");
    ancestors.add(value);
    try {
      if (Array.isArray(value))
        return `[${Array.from(value, (item) => write(item) ?? "null").join(",")}]`;
      const fields: string[] = [];
      const entries = Object.entries(value);
      if (sortKeys) entries.sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0));
      for (const [key, field] of entries) {
        const encoded = write(field);
        if (encoded !== undefined) fields.push(`${JSON.stringify(key)}:${encoded}`);
      }
      return `{${fields.join(",")}}`;
    } finally {
      ancestors.delete(value);
    }
  }
  const text = write(value);
  if (text === undefined) throw new TypeError("value is not JSON serializable");
  validateJSON(text);
  return text;
}
