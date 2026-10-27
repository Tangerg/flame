import { Backend, Refusal } from "@flame/backend-sdk";
import {
  InvalidPresentation,
  Presentation,
  documentJSONSchema,
  requestJSONSchema,
  parseDocument,
} from "../contracts/document";
import manifest from "../package/plugin.json";
import { OperationCatalog } from "./operationCatalog";
import { functionCatalog } from "../contracts/expression";

export const PRESENT_TOOL = "ui_present";
export const CATALOG_TOOL = "ui_catalog";
const INVALID_PRESENTATION = "invalid_presentation";

export function createBackend(): Backend {
  const backend = new Backend({
    name: manifest.name,
    version: manifest.version,
  });
  backend.tool(
    {
      name: PRESENT_TOOL,
      title: "Present an interactive interface",
      description:
        "Present a complete interactive interface using the declared components, local state, " +
        "pure expressions and named actions. Inputs and calculations update local state; " +
        "operation steps use installed plugin contracts discovered with ui_catalog. " +
        "Business commands require an explicit user review. Followup actions continue the " +
        "original conversation. HtmlView runs isolated local HTML/JavaScript without Flame " +
        "or tool access. Keep exact identifiers and quantities as strings. " +
        "The accepted document is retained in the existing Tool result; local drafts reset " +
        "on remount. Use ordinary text when an interface does not help.",
      inputSchema: requestJSONSchema,
      outputSchema: documentJSONSchema,
      annotations: {
        readOnlyHint: true,
        destructiveHint: false,
        idempotentHint: true,
        openWorldHint: false,
      },
    },
    async (context, input) => {
      context.signal.throwIfAborted();
      let presentation: Presentation;
      try {
        const request = parseDocument(input);
        const bindings = request.operations.length
          ? new OperationCatalog(
              (await (await context.runtime()).plugins.list(context.signal))
                .data,
            ).capture(request.operations)
          : [];
        presentation = Presentation.accept(request, bindings);
      } catch (cause) {
        if (cause instanceof InvalidPresentation) {
          const refusal = new Refusal(INVALID_PRESENTATION, cause.message);
          refusal.cause = cause;
          throw refusal;
        }
        throw cause;
      }
      return {
        structuredContent: presentation.snapshot(),
        content: [{ type: "text", text: presentation.text() }],
      };
    },
  );
  backend.tool(
    {
      name: CATALOG_TOOL,
      title: "Discover interface functions and operations",
      description:
        "Read the pure function signatures for a generated interface. Set includeOperations " +
        "to also read currently available installed plugin callable contracts. Use the exact " +
        "installationId, contribution and method in ui_present. " +
        "The contract string contains the authoritative input/output schemas, method kind " +
        "and target scope. Installation targets are derived automatically; other target " +
        "references must be explicit. This does not execute any business operation.",
      inputSchema: {
        type: "object",
        properties: {
          includeOperations: { type: "boolean" },
          installationId: { type: "string", minLength: 1 },
        },
        additionalProperties: false,
      },
      annotations: {
        readOnlyHint: true,
        destructiveHint: false,
        idempotentHint: true,
        openWorldHint: false,
      },
    },
    async (context, input) => {
      const { includeOperations, installationId } = input as {
        includeOperations?: boolean;
        installationId?: string;
      };
      const catalog = {
        functions: functionCatalog(),
        ...(includeOperations || installationId !== undefined
          ? {
              operations: new OperationCatalog(
                (await (await context.runtime()).plugins.list(context.signal))
                  .data,
              ).describe(installationId),
            }
          : {}),
      };
      return {
        structuredContent: catalog,
        content: [{ type: "text", text: JSON.stringify(catalog) }],
      };
    },
  );
  return backend;
}
