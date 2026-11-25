import {
  STRICT_VALIDATION,
  SpecVersion,
  MessageProcessor,
  type ActionPayload,
} from "@a2ui/web_core";
import {
  AgentToRendererMessageSchema,
  type AgentToRendererMessage,
} from "@a2ui/web_core/v1_0";
import { INSTALLATION_SCOPE, pluginMethod } from "@flame/runtime-client";
import {
  canonicalJSON,
  parseJSON,
  projectNumbers,
  type FrozenJSONValue,
} from "@frame/sdk/json";
import type { CallPluginRequest } from "@flame/runtime-client/wire";
import { z } from "zod/mini";
import { CATALOG_ID, catalog, functions } from "./catalog";

export class InvalidPresentation extends Error {
  constructor(message: string, cause?: unknown) {
    super(message, { cause });
  }
}
// Host-handled actions; every other action names a declared operation.
export const BUILTIN_ACTIONS = Object.freeze({
  followup: "followup",
  resetDraft: "resetDraft",
} as const);
const builtinActionNames: ReadonlySet<string> = new Set(
  Object.values(BUILTIN_ACTIONS),
);

export const limits = {
  bytes: 512 * 1024,
  messages: 512,
  operations: 32,
  titleLength: 256,
  routeLength: 1024,
  jsonNodes: 20_000,
  jsonDepth: 48,
  followupBytes: 16 * 1024,
};
const routeText = z
  .string()
  .check(z.minLength(1), z.maxLength(limits.routeLength));
const operationSchema = z.strictObject({
  id: routeText,
  installationId: routeText,
  contribution: routeText,
  method: routeText,
  target: z.optional(
    z.strictObject({ scope: routeText, reference: routeText }),
  ),
});
export type OperationRequest = z.infer<typeof operationSchema>;
const capturedContributions = z
  .array(
    z.strictObject({
      contract: z.string().check(z.minLength(1), z.maxLength(limits.bytes)),
      contribution: routeText,
      digest: routeText,
      installationId: routeText,
    }),
  )
  .check(z.maxLength(limits.operations));
export type CapturedContribution = z.infer<
  typeof capturedContributions
>[number];

// Keep A2UI's protocol discoverable through ui_catalog so each tool listing
// can carry the small envelope while the official parser owns message shape.
const messageSchema = z.pipe(
  z.looseObject({ version: z.literal(SpecVersion.V1_0) }),
  z.transform((message, context) => {
    const parsed = AgentToRendererMessageSchema.safeParse(message);
    if (!parsed.success) {
      context.issues.push({
        code: "custom",
        input: message,
        message: parsed.error.message,
      });
      return z.NEVER;
    }
    return parsed.data;
  }),
);

export const requestSchema = z.strictObject({
  title: z.string().check(z.minLength(1), z.maxLength(limits.titleLength)),
  messages: z
    .array(messageSchema)
    .check(z.minLength(1), z.maxLength(limits.messages)),
  operations: z.array(operationSchema).check(z.maxLength(limits.operations)),
});
type UiDocument = z.infer<typeof requestSchema>;
export const documentSchema = z.extend(requestSchema, {
  contributions: capturedContributions,
});
type AcceptedDocument = z.infer<typeof documentSchema>;

function normalize(value: unknown): unknown {
  try {
    return projectNumbers(value as FrozenJSONValue);
  } catch (cause) {
    throw new InvalidPresentation("use strings for exact quantities", cause);
  }
}
function bounded(value: unknown): unknown {
  if (new TextEncoder().encode(canonicalJSON(value)).byteLength > limits.bytes)
    throw new InvalidPresentation("interface exceeds content capacity");
  let nodes = 0;
  function visit(item: unknown, depth: number) {
    if (++nodes > limits.jsonNodes || depth > limits.jsonDepth)
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
    method.target === INSTALLATION_SCOPE
      ? { scope: INSTALLATION_SCOPE, reference: captured.installationId }
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
// Editable controls write only below /draft, the surface's own state.
function assertDraftBinding(value: unknown): void {
  const path =
    value && typeof value === "object" && "@path" in value
      ? value["@path"]
      : undefined;
  if (typeof path !== "string" || !path.startsWith("/draft/"))
    throw new InvalidPresentation("editable values must bind below /draft");
}

function admit(document: UiDocument): void {
  unique(document.operations.map(({ id }) => id));
  const names = new Set([
    ...builtinActionNames,
    ...document.operations.map(({ id }) => id),
  ]);
  if (document.operations.some(({ id }) => builtinActionNames.has(id)))
    throw new InvalidPresentation("reserved action name");
  const knownFunctions = new Set(functions.map(({ name }) => name));
  function admitNode(record: Record<string, unknown>) {
    const call = record["@call"] ?? record.call;
    if (typeof call === "string" && !knownFunctions.has(call))
      throw new InvalidPresentation(
        "renderer function is not admitted: " + call,
      );
    if (record.catalogId !== undefined && record.catalogId !== CATALOG_ID)
      throw new InvalidPresentation("catalog is not admitted");
    const event = record.event;
    if (
      event &&
      typeof event === "object" &&
      "name" in event &&
      !names.has(String(event.name))
    )
      throw new InvalidPresentation("action is not declared");
    if (record.component === "TextField" || record.component === "CheckBox")
      assertDraftBinding(record.value);
  }
  function walk(value: unknown) {
    if (Array.isArray(value)) {
      value.forEach(walk);
      return;
    }
    if (!value || typeof value !== "object") return;
    const record = value as Record<string, unknown>;
    admitNode(record);
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
    version: SpecVersion.V1_0,
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
export class PresentationRequest {
  readonly #document: UiDocument;
  private constructor(document: UiDocument) {
    this.#document = document;
  }

  static parse(value: unknown): PresentationRequest {
    const parsed = requestSchema.safeParse(bounded(value));
    if (!parsed.success)
      throw new InvalidPresentation(
        "expected an A2UI 1.0 interface; older interface formats must be exported as raw history",
        parsed.error,
      );
    admit(parsed.data);
    return new PresentationRequest(parsed.data);
  }

  operations(): readonly OperationRequest[] {
    return structuredClone(this.#document.operations);
  }

  snapshot(): UiDocument {
    return structuredClone(this.#document);
  }
}

function validateBindings(document: AcceptedDocument): void {
  const captured = new Map(
    document.contributions.map((entry) => [bindingKey(entry), entry]),
  );
  if (captured.size !== document.contributions.length)
    throw new InvalidPresentation("duplicate interface identity");
  const requested = new Set(document.operations.map(bindingKey));
  if (captured.size !== requested.size)
    throw new InvalidPresentation(
      "captured contributions must match declared operations",
    );
  for (const request of document.operations) {
    const entry = captured.get(bindingKey(request));
    if (!entry) throw new InvalidPresentation("missing captured contribution");
    binding(request, entry);
  }
}

export class Presentation {
  readonly #document: AcceptedDocument;

  private constructor(document: AcceptedDocument) {
    validateBindings(document);
    this.#document = document;
  }
  static accept(
    request: PresentationRequest,
    contributions: CapturedContribution[],
  ) {
    const captured = capturedContributions.safeParse(contributions);
    if (!captured.success)
      throw new InvalidPresentation(
        "invalid captured contributions",
        captured.error,
      );
    return new Presentation({
      ...request.snapshot(),
      contributions: captured.data,
    });
  }
  static read(source: string) {
    const parsed = documentSchema.safeParse(bounded(parseJSON(source)));
    if (!parsed.success)
      throw new InvalidPresentation(
        "unsupported interface history; export its raw tool result",
        parsed.error,
      );
    admit(parsed.data);
    return new Presentation(parsed.data);
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
    if (!builtinActionNames.has(action.name)) this.operation(action.name);
    bounded(action.context);
  }
  text(): string {
    return (
      this.#document.title +
      "\nInteractive A2UI 1.0 interface. The accepted protocol messages and captured operation bindings are retained in the structured tool result."
    );
  }
}
