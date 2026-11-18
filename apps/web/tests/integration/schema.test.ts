import { readFileSync } from "node:fs";
import { join } from "node:path";
import Ajv2020 from "ajv/dist/2020";
import { describe, expect, it } from "vitest";
import { WIRE_SAMPLES } from "@flame/runtime-client/testing";
const CONTRACT = join(import.meta.dirname, "../../../../runtime/contract");

const SAMPLES = join(CONTRACT, "samples");

function read(path: string): unknown {
  return JSON.parse(readFileSync(path, "utf8")) as unknown;
}

const ajv = new Ajv2020({ strict: false, allErrors: true });

const bundle = read(join(CONTRACT, "schema.json")) as { $defs: Record<string, unknown> };

ajv.addSchema(bundle, "schema.json");

const openrpc = read(join(CONTRACT, "openrpc.json")) as {
  methods: { name: string; params: { name: string }[] }[];
};

ajv.addSchema(openrpc, "openrpc.json");

function requestSchema(method: string, param?: string) {
  const index = openrpc.methods.findIndex((entry) => entry.name === method);
  const entry = openrpc.methods[index];
  if (entry === undefined) throw new Error(`OpenRPC defines no ${method}`);
  const fragment =
    param === undefined
      ? `methods/${index}/x-flame-requestFrame`
      : `methods/${index}/params/${entry.params.findIndex((parameter) => parameter.name === param)}/schema`;
  const validate = ajv.getSchema(`openrpc.json#/${fragment}`);
  if (!validate) throw new Error(`OpenRPC defines no ${method}.${param ?? "params"} schema`);
  return validate;
}

describe("the published OpenRPC request schemas", () => {
  it("compiles every whole request and by-name parameter", () => {
    for (const method of openrpc.methods) {
      expect(() => requestSchema(method.name)).not.toThrow();
      for (const param of method.params) {
        expect(() => requestSchema(method.name, param.name)).not.toThrow();
      }
    }
  });
});

describe("the published JSON Schema bundle", () => {
  it("compiles", () => {
    expect(Object.keys(bundle.$defs).length).toBeGreaterThan(0);
    for (const name of Object.keys(bundle.$defs)) {
      expect(() => ajv.getSchema(`schema.json#/$defs/${name}`)).not.toThrow();
    }
  });

  it.each(WIRE_SAMPLES)("accepts $file as $shape", ({ file, shape }) => {
    const validate = ajv.getSchema(`schema.json#/$defs/${shape}`);
    if (!validate) throw new Error(`the bundle defines no ${shape}`);
    validate(read(join(SAMPLES, file)));
    expect(validate.errors ?? []).toEqual([]);
  });
});
