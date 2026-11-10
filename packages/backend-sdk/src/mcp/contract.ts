import { createHash } from "node:crypto";
import {
  Capability as PortableCapability,
  type MethodDescriptor,
} from "@frame/sdk/node";
import { canonicalJSON, freezeJSON } from "@frame/sdk/json";
import wire from "./protocol.generated.json";

export { canonicalJSON, freezeJSON } from "@frame/sdk/json";
export type { MethodDescriptor } from "@frame/sdk/node";
export type Method = ReturnType<PortableCapability["method"]>;
export const PROTOCOL = wire.constants.protocol;

export interface CapabilityDescriptor {
  readonly protocol: string;
  readonly reference: string;
  readonly composition: "collection" | "selection" | "decoration";
  readonly scope: string;
  readonly methods: readonly MethodDescriptor[];
}

export function digestJSON(value: unknown): string {
  return createHash("sha256").update(canonicalJSON(value)).digest("hex");
}

export function validCommand(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    !/\p{Surrogate}/u.test(value) &&
    [...value].length <= wire.constants.maxCommandIDCharacters
  );
}

export function validToken(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    Buffer.byteLength(value) <= wire.limits.maxTokenBytes &&
    !/[\p{Surrogate}\p{White_Space}\p{Cc}]/u.test(value)
  );
}

export function compareTokens(left: string, right: string): number {
  return Buffer.compare(Buffer.from(left), Buffer.from(right));
}

export function exactFields(
  value: unknown,
  required: readonly string[],
  optional: readonly string[] = [],
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

export function declareMethod(source: unknown): Method {
  const descriptor: unknown = freezeJSON(source);
  if (
    descriptor === null ||
    typeof descriptor !== "object" ||
    !("name" in descriptor) ||
    typeof descriptor.name !== "string"
  )
    throw new Error("plugin: method contract requires a name");
  return new PortableCapability({
    protocol: wire.constants.portableProtocol,
    reference: descriptor.name,
    methods: [descriptor as MethodDescriptor],
  }).method(descriptor.name);
}

export class Capability {
  readonly descriptor: CapabilityDescriptor;
  readonly identity: string;
  readonly portable: PortableCapability;

  constructor(source: unknown) {
    const descriptor: unknown = freezeJSON(source);
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
      !Array.isArray(descriptor.methods) ||
      (descriptor.composition !== "collection" &&
        descriptor.composition !== "selection" &&
        descriptor.composition !== "decoration")
    )
      throw new Error("plugin: invalid capability contract");
    this.portable = new PortableCapability({
      protocol: wire.constants.portableProtocol,
      reference: descriptor.reference,
      methods: descriptor.methods,
    });
    const methods = [...this.portable.descriptor.methods].sort((left, right) =>
      compareTokens(left.name, right.name),
    );
    this.descriptor = Object.freeze({
      protocol: PROTOCOL,
      reference: this.portable.descriptor.reference,
      composition: descriptor.composition,
      scope: descriptor.scope,
      methods: Object.freeze(methods),
    });
    this.identity = digestJSON({
      ...this.descriptor,
      methods: methods.map((method) => ({ ...method, description: "" })),
    });
    Object.freeze(this);
  }
}
