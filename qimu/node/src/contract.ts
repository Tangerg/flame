import { createHash } from "node:crypto";
import { cloneJSON, stringifyJSON } from "@qimu/sdk/json";
import { compileSchema } from "./schema.ts";
import wire from "./protocol.generated.json";
import { checkDescriptor, exceeded, PORTABLE_LIMITS } from "./limits.ts";

export const PROTOCOL = wire.constants.protocol;

export interface MethodDescriptor {
  name: string;
  kind: "query" | "command";
  target: string;
  description: string;
  input: unknown;
  output: unknown;
  stream?: unknown;
}
export interface CapabilityDescriptor {
  protocol: string;
  reference: string;
  composition: "collection" | "selection" | "decoration";
  scope: string;
  methods: MethodDescriptor[];
}
export function canonicalJSON(value: unknown): string {
  return stringifyJSON(value, true);
}
export function digestJSON(value: unknown): string {
  return createHash("sha256").update(canonicalJSON(value)).digest("hex");
}
export function freezeJSON<T>(value: T): T {
  const copy = cloneJSON(value);
  const freeze = (value: unknown): void => {
    if (value === null || typeof value !== "object" || Object.isFrozen(value))
      return;
    for (const child of Object.values(value)) freeze(child);
    Object.freeze(value);
  };
  freeze(copy);
  return copy;
}
export function validCommand(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    Buffer.byteLength(value) <= 256 &&
    !/\p{Surrogate}/u.test(value)
  );
}
export function validToken(value: unknown): value is string {
  return validCommand(value) && !/[\p{White_Space}\p{Cc}]/u.test(value);
}
export function compareTokens(left: string, right: string): number {
  // Go orders protocol tokens by UTF-8 bytes, independently of JSON key order.
  return Buffer.compare(Buffer.from(left), Buffer.from(right));
}
export function exactFields(
  value: unknown,
  required: string[],
  optional: string[] = [],
): asserts value is Record<string, unknown> {
  if (
    value === null ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    required.some((key) => !Object.hasOwn(value, key)) ||
    Object.keys(value).some(
      (key) => !required.includes(key) && !optional.includes(key),
    )
  )
    throw new Error("plugin: invalid protocol members");
}

export class Method {
  readonly descriptor: MethodDescriptor;
  readonly identity: string;
  readonly validateInput: (value: unknown) => void;
  readonly validateOutput: (value: unknown) => void;
  readonly validateChunk?: (value: unknown) => void;

  constructor(descriptor: MethodDescriptor) {
    checkDescriptor(descriptor);
    exactFields(
      descriptor,
      ["name", "kind", "target", "description", "input", "output"],
      ["stream"],
    );
    if (
      !validToken(descriptor.name) ||
      !validToken(descriptor.target) ||
      !["query", "command"].includes(descriptor.kind) ||
      typeof descriptor.description !== "string"
    )
      throw new Error("plugin: invalid method contract");
    this.descriptor = freezeJSON(descriptor);
    this.validateInput = compileSchema(descriptor.input);
    let admitsNull = true;
    try {
      this.validateInput(null);
    } catch {
      admitsNull = false;
    }
    if (admitsNull) throw new Error("plugin: method input must exclude null");
    this.validateOutput = compileSchema(descriptor.output);
    if (Object.hasOwn(descriptor, "stream"))
      this.validateChunk = compileSchema(descriptor.stream);
    this.identity = digestJSON({ ...descriptor, description: "" });
    Object.freeze(this);
  }
}

export class Capability {
  readonly descriptor: CapabilityDescriptor;
  readonly identity: string;
  readonly #methods: ReadonlyMap<string, Method>;

  constructor(descriptor: CapabilityDescriptor) {
    exactFields(descriptor, [
      "protocol",
      "reference",
      "composition",
      "scope",
      "methods",
    ]);
    if (
      descriptor.protocol !== PROTOCOL ||
      !validToken(descriptor.reference) ||
      !validToken(descriptor.scope) ||
      !["collection", "selection", "decoration"].includes(
        descriptor.composition,
      ) ||
      !Array.isArray(descriptor.methods) ||
      descriptor.methods.length === 0
    )
      throw new Error("plugin: invalid capability contract");
    if (descriptor.methods.length > PORTABLE_LIMITS.maxMethods)
      exceeded("capability methods", PORTABLE_LIMITS.maxMethods);
    checkDescriptor(descriptor);
    const methods = descriptor.methods
      .map((value) => new Method(value))
      .sort((a, b) => compareTokens(a.descriptor.name, b.descriptor.name));
    this.#methods = new Map(
      methods.map((method) => [method.descriptor.name, method]),
    );
    if (this.#methods.size !== methods.length)
      throw new Error("plugin: duplicate capability method");
    this.descriptor = freezeJSON({
      ...descriptor,
      methods: methods.map((method) => method.descriptor),
    });
    this.identity = digestJSON({
      ...this.descriptor,
      methods: methods.map((method) => ({
        ...method.descriptor,
        description: "",
      })),
    });
    Object.freeze(this);
  }
  method(name: string): Method {
    const method = this.#methods.get(name);
    if (!method) throw new Error("plugin: capability method is not published");
    return method;
  }
}
