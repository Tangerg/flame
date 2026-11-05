import { runtime } from "@flame/backend-sdk";
import { Backend, Refusal } from "@flame/backend-sdk/mcp";
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
        "original conversation with only their explicit payload expression (message and JSON " +
        "payload together must fit 16 KiB). HtmlView runs isolated local HTML/JavaScript without Flame " +
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
        const contributions = request.operations.length
          ? new OperationCatalog(
              (await (await runtime(context)).plugins.list(context.signal))
                .data,
            ).capture(request.operations)
          : [];
        presentation = Presentation.accept(request, contributions);
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
        "Discover interface capabilities on demand. kind=functions lists brief function entries; " +
        "set name to read one function's complete parameters. kind=operations lists available " +
        "method identities and routing metadata; optionally filter by installationId and contribution. " +
        "Set installationId, contribution and method to read that exact methodJSON with its " +
        "input/output schemas. Index pages contain at most 20 entries; continue using nextOffset. " +
        "Use the exact installationId, contribution and method in ui_present. Installation targets " +
        "are derived automatically; other targets require an explicit reference. Discovery does not " +
        "execute business operations or authorize a command.",
      inputSchema: {
        type: "object",
        properties: {
          kind: { type: "string", enum: ["functions", "operations"] },
          name: { type: "string", minLength: 1 },
          installationId: { type: "string", minLength: 1 },
          contribution: { type: "string", minLength: 1 },
          method: { type: "string", minLength: 1 },
          offset: { type: "integer", minimum: 0 },
        },
        required: ["kind"],
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
      const { kind, name, installationId, contribution, method, offset } =
        input as {
          kind: "functions" | "operations";
          name?: string;
          installationId?: string;
          contribution?: string;
          method?: string;
          offset?: number;
        };
      const page = <T>(entries: T[]) => {
        const start = offset ?? 0;
        return {
          entries: entries.slice(start, start + 20),
          ...(start + 20 < entries.length ? { nextOffset: start + 20 } : {}),
        };
      };
      let catalog: Record<string, unknown>;
      if (kind === "functions") {
        if (
          installationId !== undefined ||
          contribution !== undefined ||
          method !== undefined ||
          (name !== undefined && offset !== undefined)
        )
          throw new Refusal(
            "invalid_catalog_request",
            "Function discovery accepts only name or offset.",
          );
        const functions = functionCatalog();
        if (name === undefined) {
          catalog = {
            functions: page(
              functions.map(({ name, description }) => ({ name, description })),
            ),
          };
        } else {
          const selected = functions.find((value) => value.name === name);
          if (!selected)
            throw new Refusal(
              "unknown_function",
              "No such interface function: " + name,
            );
          catalog = { function: selected };
        }
      } else {
        if (
          name !== undefined ||
          (contribution !== undefined && installationId === undefined) ||
          (method !== undefined &&
            (installationId === undefined ||
              contribution === undefined ||
              offset !== undefined))
        )
          throw new Refusal(
            "invalid_catalog_request",
            "Method discovery requires installationId and contribution; offset only pages an index.",
          );
        const operations = new OperationCatalog(
          (await (await runtime(context)).plugins.list(context.signal)).data,
        );
        try {
          catalog =
            method === undefined
              ? {
                  operations: page(
                    operations.describe(installationId, contribution),
                  ),
                }
              : {
                  operation: operations.method(
                    installationId!,
                    contribution!,
                    method,
                  ),
                };
        } catch (cause) {
          if (cause instanceof InvalidPresentation)
            throw new Refusal("invalid_catalog_request", cause.message);
          throw cause;
        }
      }
      return {
        structuredContent: catalog,
        content: [{ type: "text", text: JSON.stringify(catalog) }],
      };
    },
  );
  return backend;
}
