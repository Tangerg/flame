import {
  STRICT_VALIDATION,
  MessageProcessor,
  type ActionPayload,
} from "@a2ui/web_core";
import {
  AgentToRendererMessageSchema,
  type AgentToRendererMessage,
} from "@a2ui/web_core/v1_0";
import { pluginMethod } from "@flame/runtime-client";
import {
  canonicalJSON,
  isJSONNumber,
  parseJSON,
  projectNumber,
} from "@frame/sdk/json";
import type { CallPluginRequest } from "@flame/runtime-client/wire";
import { z } from "zod3";
import { CATALOG_ID, catalog, functions } from "./catalog";

export class InvalidPresentation extends Error {
  constructor(message: string, cause?: unknown) {
    super(message, { cause });
  }
}
export const limits = {
  bytes: 512 * 1024,
  nodes: 512,
  operations: 32,
  followupBytes: 16 * 1024,
};
const routeText = z.string().min(1).max(1024);
const operationSchema = z
  .object({
    id: routeText,
    installationId: routeText,
    contribution: routeText,
    method: routeText,
    target: z
      .object({ scope: routeText, reference: routeText })
      .strict()
      .optional(),
  })
  .strict();
export type OperationRequest = z.infer<typeof operationSchema>;
export type CapturedContribution = Pick<
  CallPluginRequest,
  "contract" | "contribution" | "digest" | "installationId"
>;
const capturedContributionSchema = z
  .object({
    contract: z.string().min(1).max(limits.bytes),
    contribution: routeText,
    digest: routeText,
    installationId: routeText,
  })
  .strict();
export const requestSchema = z
  .object({
    title: z.string().min(1).max(256),
    messages: z.array(AgentToRendererMessageSchema).min(1).max(limits.nodes),
    operations: z.array(operationSchema).max(limits.operations),
  })
  .strict();
export type UiDocument = z.infer<typeof requestSchema>;
export const documentSchema = requestSchema.extend({
  contributions: z.array(capturedContributionSchema).max(limits.operations),
});
export type AcceptedDocument = z.infer<typeof documentSchema>;
export const requestJSONSchema = {
  type: "object" as const,
  properties: {
    title: { type: "string", minLength: 1, maxLength: 256 },
    messages: {
      type: "array",
      minItems: 1,
      maxItems: limits.nodes,
      items: {
        type: "object",
        properties: { version: { const: "1.0" } },
        required: ["version"],
      },
      description:
        "Official A2UI 1.0 messages. Get the complete protocol and trusted catalog with ui_catalog.",
    },
    operations: {
      type: "array",
      maxItems: limits.operations,
      items: {
        type: "object",
        properties: {
          id: { type: "string" },
          installationId: { type: "string" },
          contribution: { type: "string" },
          method: { type: "string" },
          target: {
            type: "object",
            properties: {
              scope: { type: "string" },
              reference: { type: "string" },
            },
            required: ["scope", "reference"],
            additionalProperties: false,
          },
        },
        required: ["id", "installationId", "contribution", "method"],
        additionalProperties: false,
      },
    },
  },
  required: ["title", "messages", "operations"],
  additionalProperties: false,
};
export const documentJSONSchema = {
  ...requestJSONSchema,
  properties: {
    ...requestJSONSchema.properties,
    contributions: { type: "array", items: { type: "object" } },
  },
  required: [...requestJSONSchema.required, "contributions"],
};

function normalize(value: unknown): unknown {
  if (isJSONNumber(value)) {
    try {
      return projectNumber(value);
    } catch (cause) {
      throw new InvalidPresentation("use strings for exact quantities", cause);
    }
  }
  if (Array.isArray(value)) return value.map(normalize);
  if (value !== null && typeof value === "object")
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, normalize(item)]),
    );
  return value;
}
function bounded(value: unknown): unknown {
  if (new TextEncoder().encode(canonicalJSON(value)).byteLength > limits.bytes)
    throw new InvalidPresentation("interface exceeds content capacity");
  let nodes = 0;
  function visit(item: unknown, depth: number) {
    if (++nodes > 20000 || depth > 48)
      throw new InvalidPresentation("interface exceeds structural capacity");
    if (Array.isArray(item)) item.forEach((child) => visit(child, depth + 1));
    else if (item && typeof item === "object")
      Object.values(item).forEach((child) => visit(child, depth + 1));
  }
  visit(value, 0);
  return normalize(value);
}
function unique(values: readonly string[]) {
  if (new Set(values).size !== values.length)
    throw new InvalidPresentation("duplicate interface identity");
}
const bindingKey = (
  value: Pick<CapturedContribution, "installationId" | "contribution">,
) => JSON.stringify([value.installationId, value.contribution]);
function binding(
  request: OperationRequest,
  captured: CapturedContribution,
): Omit<CallPluginRequest, "input"> {
  const method = pluginMethod({ ...captured, method: request.method });
  const target =
    method.target === "installation"
      ? { scope: "installation", reference: captured.installationId }
      : request.target;
  if (
    !target ||
    target.scope !== method.target ||
    (request.target &&
      (target.scope !== request.target.scope ||
        target.reference !== request.target.reference))
  )
    throw new InvalidPresentation("captured target differs from its method");
  return { ...captured, method: request.method, target };
}
function admit(document: UiDocument): void {
  unique(document.operations.map(({ id }) => id));
  const names = new Set([
    "followup",
    "resetDraft",
    ...document.operations.map(({ id }) => id),
  ]);
  if (
    document.operations.some(
      ({ id }) => id === "followup" || id === "resetDraft",
    )
  )
    throw new InvalidPresentation("reserved action name");
  const knownFunctions = new Set(functions.map(({ name }) => name));
  function walk(value: unknown) {
    if (Array.isArray(value)) {
      value.forEach(walk);
      return;
    }
    if (!value || typeof value !== "object") return;
    const record = value as Record<string, unknown>;
    const call = record["@call"] ?? record.call;
    if (typeof call === "string" && !knownFunctions.has(call))
      throw new InvalidPresentation(
        "renderer function is not admitted: " + call,
      );
    if (record.catalogId !== undefined && record.catalogId !== CATALOG_ID)
      throw new InvalidPresentation("catalog is not admitted");
    if (
      record.event &&
      typeof record.event === "object" &&
      "name" in record.event &&
      !names.has(String(record.event.name))
    )
      throw new InvalidPresentation("action is not declared");
    if (record.component === "TextField" || record.component === "CheckBox") {
      const value = record.value;
      const path =
        value && typeof value === "object" && "@path" in value
          ? value["@path"]
          : undefined;
      if (typeof path !== "string" || !path.startsWith("/draft/"))
        throw new InvalidPresentation("editable values must bind below /draft");
    }
    Object.values(record).forEach(walk);
  }
  const surfaces = new Set<string>();
  for (const message of document.messages) {
    if ("callRendererFunction" in message || "agentFunctionResponse" in message)
      throw new InvalidPresentation(
        "recorded interfaces cannot execute RPC messages",
      );
    if ("createSurface" in message) {
      const surface = message.createSurface;
      if (
        surface.catalogId !== CATALOG_ID ||
        surface.sendDataModel === true ||
        surfaces.has(surface.surfaceId)
      )
        throw new InvalidPresentation("invalid surface admission");
      surfaces.add(surface.surfaceId);
    }
    if ("updateDataModel" in message) {
      const path = message.updateDataModel.path;
      if (!path?.startsWith("/data/"))
        throw new InvalidPresentation(
          "remote updates may write only below /data",
        );
    }
    if ("createSurface" in message) walk(message.createSurface.components);
    if ("updateComponents" in message)
      walk(message.updateComponents.components);
  }
  const processor = new MessageProcessor([catalog], undefined, {
    version: "1.0",
    validationConfig: STRICT_VALIDATION,
  });
  try {
    processor.processMessages(document.messages);
  } catch (cause) {
    throw new InvalidPresentation("invalid A2UI interface", cause);
  } finally {
    processor.dispose();
  }
}
export function parseDocument(value: unknown): UiDocument {
  const parsed = requestSchema.safeParse(bounded(value));
  if (!parsed.success)
    throw new InvalidPresentation(
      "expected an A2UI 1.0 interface; older interface formats must be exported as raw history",
      parsed.error,
    );
  admit(parsed.data);
  return parsed.data;
}
export class Presentation {
  readonly #document: AcceptedDocument;
  private constructor(document: AcceptedDocument) {
    this.#document = structuredClone(document);
  }
  static accept(value: unknown, contributions: CapturedContribution[] = []) {
    const document = parseDocument(value);
    const captured = z.array(capturedContributionSchema).parse(contributions);
    unique(captured.map(bindingKey));
    const requested = new Set(document.operations.map(bindingKey));
    if (
      captured.length !== requested.size ||
      captured.some((entry) => !requested.has(bindingKey(entry)))
    )
      throw new InvalidPresentation(
        "captured contributions must match declared operations",
      );
    for (const request of document.operations) {
      const entry = captured.find(
        (entry) => bindingKey(entry) === bindingKey(request),
      );
      if (!entry)
        throw new InvalidPresentation("missing captured contribution");
      binding(request, entry);
    }
    return new Presentation({ ...document, contributions: captured });
  }
  static read(source: string) {
    const parsed = documentSchema.safeParse(bounded(parseJSON(source)));
    if (!parsed.success)
      throw new InvalidPresentation(
        "unsupported interface history; export its raw tool result",
        parsed.error,
      );
    const { contributions, ...document } = parsed.data;
    return Presentation.accept(document, contributions);
  }
  snapshot(): AcceptedDocument {
    return structuredClone(this.#document);
  }
  json(): string {
    return canonicalJSON(this.#document);
  }
  messages(): readonly AgentToRendererMessage[] {
    return structuredClone(this.#document.messages);
  }
  operation(id: string) {
    const request = this.#document.operations.find((entry) => entry.id === id);
    const entry =
      request &&
      this.#document.contributions.find(
        (entry) => bindingKey(entry) === bindingKey(request),
      );
    if (!request || !entry)
      throw new InvalidPresentation("operation is not declared");
    return {
      request: structuredClone(request),
      binding: binding(request, entry),
    };
  }
  operations() {
    return this.#document.operations.map(({ id }) => this.operation(id));
  }
  action(action: ActionPayload): void {
    if (
      !this.#document.messages.some(
        (message) =>
          "createSurface" in message &&
          message.createSurface.surfaceId === action.surfaceId,
      )
    )
      throw new InvalidPresentation("action belongs to another surface");
    if (action.name !== "followup" && action.name !== "resetDraft")
      this.operation(action.name);
    bounded(action.context);
  }
  text(): string {
    return (
      this.#document.title +
      "\nInteractive A2UI 1.0 interface. The accepted protocol messages and captured operation bindings are retained in the structured tool result."
    );
  }
}
