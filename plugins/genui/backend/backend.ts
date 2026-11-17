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
import { catalog as trustedCatalog } from "../contracts/catalog";
import { Schemas } from "@a2ui/web_core/v1_0";

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
        "Present a bounded official A2UI 1.0 interface. Discover the Flame trusted catalog and protocol with ui_catalog. " +
        "The initial createSurface must use the Flame catalog and sendDataModel:false. Editable controls bind below /draft; " +
        "subsequent data updates write only below /data. Events call declared captured operations with explicit context. " +
        "Business commands require user review; followup sends only {message,payload} to the original conversation. " +
        "Recorded rendering never replays operations. No older interface format is accepted.",
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
        "kind=catalog returns the official A2UI 1.0 protocol and complete Flame trusted catalog. kind=operations lists available " +
        "method identities and routing metadata; optionally filter by installationId and contribution. " +
        "Set installationId, contribution and method to read that exact methodJSON with its " +
        "input/output schemas. Index pages contain at most 20 entries; continue using nextOffset. " +
        "Use the exact installationId, contribution and method in ui_present. Installation targets " +
        "are derived automatically; other targets require an explicit reference. Discovery does not " +
        "execute business operations or authorize a command.",
      inputSchema: {
        type: "object",
        properties: {
          kind: { type: "string", enum: ["catalog", "operations"] },
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
      const { kind, installationId, contribution, method, offset } = input as {
        kind: "catalog" | "operations";
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
      if (kind === "catalog") {
        if (
          installationId !== undefined ||
          contribution !== undefined ||
          method !== undefined ||
          offset !== undefined
        )
          throw new Refusal(
            "invalid_catalog_request",
            "Trusted catalog discovery accepts only kind.",
          );
        catalog = {
          protocol: Schemas.A2uiMessageSchemaRaw,
          catalog: trustedCatalog.toJson(),
        };
      } else {
        if (
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
