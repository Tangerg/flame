import { mkdirSync, lstatSync, realpathSync } from "node:fs";
import { isAbsolute, join } from "node:path";
import { Backend, Capability, Refusal, canonicalJSON } from "@qimu/node";
import { toolReceipts } from "@flame/backend-sdk";
import descriptor, {
  METHODS,
  schemas,
  definitions,
} from "../contracts/capability";
import { Archive, toolResult } from "./archive";
import { Application, decodeInput } from "./application";
import { Engine } from "./engine";
import { Formats } from "./format-process";
import manifest from "../package/plugin.json";
import { z } from "zod";

export function createBackend(
  environment: NodeJS.ProcessEnv,
  formatEntry: string,
): Backend {
  const data = environment.PLUGIN_DATA;
  if (!data || !isAbsolute(data))
    throw new Error(
      "deliverables: absolute private plugin data directory is required",
    );
  const backend = new Backend({
    name: manifest.name,
    version: manifest.version,
  });
  const contract = new Capability(descriptor);
  let archive: Archive;
  let application: Application;
  backend.onSetup((context) => {
    mkdirSync(data, { recursive: true, mode: 0o700 });
    const info = lstatSync(data);
    if (
      !info.isDirectory() ||
      realpathSync(data) !== data ||
      (info.mode & 0o022) !== 0
    )
      throw new Error(
        "deliverables: plugin data must be a canonical private directory",
      );
    archive = new Archive(join(data, "deliverables.db"));
    context.defer(() => archive.close());
    const engine = new Engine(environment, data);
    const formats = new Formats(formatEntry);
    application = new Application(archive, engine, formats);
    return {
      close: async () => {
        const results = await Promise.allSettled([
          engine.close(),
          formats.close(),
        ]);
        const failures = results.flatMap((result) =>
          result.status === "rejected" ? [result.reason] : [],
        );
        if (failures.length)
          throw new AggregateError(
            failures,
            "deliverables: process shutdown is unconfirmed",
          );
      },
    };
  });
  for (const definition of definitions) {
    const { operation, input } = definition;
    const method = contract.method(METHODS[operation]);
    backend.implement(
      method,
      (value) => decodeInput(input, value) as Record<string, unknown>,
      (context, value) => {
        context.signal.throwIfAborted();
        return method.descriptor.kind === "query"
          ? application.query(operation, value)
          : application.command(context, operation, value);
      },
    );
    backend.tool(
      {
        name: operation,
        title: `Deliverables: ${operation}`,
        description: method.descriptor.description!,
        inputSchema: z.toJSONSchema(input) as { type: "object" },
        annotations: {
          readOnlyHint: method.descriptor.kind === "query",
          destructiveHint: ["remove", "discard"].includes(operation),
          idempotentHint: method.descriptor.kind === "query",
          openWorldHint: false,
        },
      },
      async (context, value) => {
        const decoded = decodeInput(input, value) as Record<string, unknown>;
        try {
          context.signal.throwIfAborted();
          const result =
            method.descriptor.kind === "query"
              ? application.query(operation, decoded)
              : await application.command(context, operation, decoded, "tool");
          return toolResult({ value: result });
        } catch (cause) {
          if (cause instanceof Refusal)
            return toolResult({
              failure: { code: cause.code, message: cause.message },
            });
          throw cause;
        }
      },
    );
  }
  const pageInput = z.strictObject({ assetId: schemas.asset.shape.id });
  backend.tool(
    {
      name: "page",
      title: "Inspect an actual rendered page",
      description:
        "Read one actual retained PNG page from a render assessment as an image. Use its exact asset ID to perform visual review; do not infer layout quality from extracted text. This does not record a review or publish a file.",
      inputSchema: z.toJSONSchema(pageInput) as { type: "object" },
      annotations: {
        readOnlyHint: true,
        destructiveHint: false,
        openWorldHint: false,
      },
    },
    (context, value) => {
      context.signal.throwIfAborted();
      const result = application.page(pageInput.parse(value).assetId);
      return {
        content: [
          { type: "text", text: canonicalJSON(result.metadata) },
          { type: "image", data: result.base64, mimeType: "image/png" },
        ],
        structuredContent: result.metadata,
      };
    },
  );
  backend.receipts((_context, request) => archive.receipt(request, "command"));
  toolReceipts(backend, (_context, request) => {
    const receipt = archive.receipt(request, "tool");
    if (!receipt.found) return { found: false };
    const result = toolResult(receipt.reply!);
    return {
      found: true,
      output: canonicalJSON({
        content: result.content.map((part) => ({
          kind: "text",
          text: part.text,
        })),
        details: result.structuredContent,
      }),
      ...(result.isError ? { isError: true } : {}),
    };
  });
  backend.bindCapability("deliverables", contract);
  return backend;
}
