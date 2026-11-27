import {
  STRICT_VALIDATION,
  MessageProcessor,
  type ActionPayload,
} from "@a2ui/web_core";
import {
  ActionSchema,
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
import type { PluginCallRoute } from "@flame/runtime-client/wire";
import { z } from "zod/mini";
import {
  A2UI_VERSION,
  CATALOG_ID,
  catalog,
  trustedComponentApis,
} from "./catalog";

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
  z.looseObject({ version: z.literal(A2UI_VERSION) }),
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
function validateContentCapacity(value: unknown): void {
  let nodes = 0;
  function visit(item: unknown, depth: number) {
    if (++nodes > limits.jsonNodes || depth > limits.jsonDepth)
      throw new InvalidPresentation("interface exceeds structural capacity");
    if (Array.isArray(item)) item.forEach((child) => visit(child, depth + 1));
    else if (item && typeof item === "object")
      Object.values(item).forEach((child) => visit(child, depth + 1));
  }
  visit(value, 0);
  if (new TextEncoder().encode(canonicalJSON(value)).byteLength > limits.bytes)
    throw new InvalidPresentation("interface exceeds content capacity");
}

function bounded(value: unknown): unknown {
  validateContentCapacity(value);
  return normalize(value);
}
const bindingKey = (
  value: Pick<CapturedContribution, "installationId" | "contribution">,
) => JSON.stringify([value.installationId, value.contribution]);
function binding(
  request: OperationRequest,
  captured: CapturedContribution,
): PluginCallRoute {
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

function admitRendererFunction(name: string, catalogId: unknown): void {
  if (!catalog.rendererFunctions.has(name))
    throw new InvalidPresentation(`renderer function is not admitted: ${name}`);
  if (catalogId !== undefined && catalogId !== CATALOG_ID)
    throw new InvalidPresentation("catalog is not admitted");
}

function admitExpressions(value: unknown): void {
  if (Array.isArray(value)) {
    value.forEach(admitExpressions);
    return;
  }
  if (!value || typeof value !== "object") return;
  const record = value as Record<string, unknown>;
  const call = record["@call"];
  if (typeof call === "string") admitRendererFunction(call, record.catalogId);
  Object.values(record).forEach(admitExpressions);
}

function admitComponent(
  component: Record<string, unknown>,
  admittedActions: ReadonlySet<string>,
): void {
  if (
    component.component === trustedComponentApis.textField.name ||
    component.component === trustedComponentApis.checkbox.name
  )
    assertDraftBinding(component.value);
  if (component.action !== undefined) {
    const action = ActionSchema.safeParse(component.action);
    if (!action.success)
      throw new InvalidPresentation("invalid component action", action.error);
    if ("event" in action.data && !admittedActions.has(action.data.event.name))
      throw new InvalidPresentation("action is not declared");
    if ("functionCall" in action.data) {
      // A2UI evaluates the unprefixed name within an explicit function action.
      const { "@call": expression, call, catalogId } = action.data.functionCall;
      if (expression === undefined && call !== undefined)
        admitRendererFunction(call, catalogId);
    }
  }
  admitExpressions(component);
}

function admit(document: UiDocument): void {
  const admittedActions = new Set(builtinActionNames);
  for (const { id } of document.operations) {
    if (builtinActionNames.has(id))
      throw new InvalidPresentation("reserved action name");
    if (admittedActions.has(id))
      throw new InvalidPresentation("duplicate interface identity");
    admittedActions.add(id);
  }
  const admitControls = (component: Record<string, unknown>) =>
    admitComponent(component, admittedActions);
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
      surface.components?.forEach(admitControls);
      continue;
    }
    if ("updateComponents" in message) {
      message.updateComponents.components.forEach(admitControls);
      continue;
    }
    if (
      "updateDataModel" in message &&
      !message.updateDataModel.path?.startsWith("/data/")
    )
      throw new InvalidPresentation(
        "remote updates may write only below /data",
      );
  }
  const processor = new MessageProcessor([catalog], undefined, {
    version: A2UI_VERSION,
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
        `expected an A2UI ${A2UI_VERSION} interface; older interface formats must be exported as raw history`,
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
    const document = {
      ...request.snapshot(),
      contributions: captured.data,
    };
    validateContentCapacity(document);
    return new Presentation(document);
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
      `\nInteractive A2UI ${A2UI_VERSION} interface. The accepted protocol messages and captured operation bindings are retained in the structured tool result.`
    );
  }
}
