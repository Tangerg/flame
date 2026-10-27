import {
  createParser,
  evaluateRoot,
  jsonToOpenUI,
  type ASTNode,
  type ElementNode,
  type Library,
  type OpenUIError,
} from "@openuidev/lang-core";
import { pluginMethod } from "@flame/runtime-client";
import {
  equalJSON,
  isJSONNumber,
  parseJSON,
  projectNumber,
  stringifyJSON,
} from "@flame/runtime-client/json";
import type { CallPluginRequest } from "@flame/runtime-client/wire";
import { z } from "zod/v4";
import {
  authoredPropsSchema,
  createDisplayLibrary,
  displayDefinitions,
  validateResolvedProps,
  type DisplayComponentName,
} from "./catalog";
import {
  assertValueBudget,
  compileExpression,
  each,
  evaluateExpression,
  expressionCost,
  expressionReferences,
  expressionSchema,
  identifier,
  InvalidPresentation,
  isExpression,
  jsonValueSchema,
  limits,
  operationKey,
  runtimeFunctions,
  stateKey,
  type Expression,
  type JSONValue,
} from "./expression";
import {
  validateField,
  type FieldKind,
  type FieldRules,
  type ValidationIssue,
} from "./validation";

export {
  InvalidPresentation,
  limits,
  operationKey,
  stateKey,
  type Expression,
  type JSONValue,
} from "./expression";
export {
  isSafeURL,
  safeURL,
  validateField,
  type FieldRules,
  type ValidationIssue,
} from "./validation";

export interface UiNode {
  id: string;
  type: Exclude<DisplayComponentName, "Document"> | "Each";
  props: Record<string, unknown>;
  children?: UiNode[];
}

const nodeSchema: z.ZodType<UiNode> = z.lazy(() => {
  const components = Object.keys(displayDefinitions)
    .filter((name) => name !== "Document")
    .map((name) => {
      const component = name as DisplayComponentName;
      const definition = displayDefinitions[component];
      return z
        .strictObject({
          id: identifier,
          type: z.literal(component),
          props: authoredPropsSchema(component),
          ...(definition.children
            ? { children: z.array(nodeSchema).max(limits.nodes) }
            : {}),
        })
        .describe(definition.description);
    });
  return z.union([
    ...components,
    z.strictObject({
      id: identifier,
      type: z.literal("Each"),
      props: z.strictObject({
        items: expressionSchema,
        as: identifier,
        limit: z.number().int().min(1).max(limits.renderedNodes),
      }),
      children: z.array(nodeSchema).length(1),
    }),
  ]) as z.ZodType<UiNode>;
});

const actionSchema = z.strictObject({
  id: identifier,
  steps: z
    .array(
      z.discriminatedUnion("type", [
        z.strictObject({
          type: z.literal("set"),
          state: identifier,
          value: expressionSchema,
        }),
        z.strictObject({
          type: z.literal("reset"),
          states: z.array(identifier).min(1).max(limits.states),
        }),
        z.strictObject({ type: z.literal("operation"), operation: identifier }),
        z.strictObject({
          type: z.literal("followup"),
          message: expressionSchema,
        }),
        z.strictObject({ type: z.literal("copy"), text: expressionSchema }),
        z.strictObject({ type: z.literal("openUrl"), url: expressionSchema }),
      ]),
    )
    .min(1)
    .max(32),
});
export type Action = z.infer<typeof actionSchema>;

const routeText = z.string().min(1).max(1024);
const targetSchema = z.strictObject({ scope: routeText, reference: routeText });
const operationSchema = z.strictObject({
  id: identifier,
  installationId: routeText,
  contribution: routeText,
  method: routeText,
  target: targetSchema.optional(),
  input: expressionSchema,
  auto: z.boolean().optional(),
});
export type OperationRequest = z.infer<typeof operationSchema>;
export type CapturedBinding = {
  id: string;
  binding: Omit<CallPluginRequest, "input">;
};
const capturedBindingSchema: z.ZodType<CapturedBinding> = z.strictObject({
  id: identifier,
  binding: z.strictObject({
    contract: z.string().min(1).max(limits.html),
    contribution: routeText,
    digest: routeText,
    installationId: routeText,
    method: routeText,
    target: targetSchema,
  }),
});

export const requestSchema = z
  .strictObject({
    title: z.string().min(1).max(256).regex(/\S/u),
    state: z
      .array(z.strictObject({ id: identifier, value: jsonValueSchema }))
      .max(limits.states),
    nodes: z.array(nodeSchema).min(1).max(limits.nodes),
    actions: z.array(actionSchema).max(limits.actions),
    operations: z.array(operationSchema).max(limits.operations),
  })
  .describe(
    "A complete interactive JSON document. State is local input; operation projections are read-only. Values can be pure expressions. Use strings for exact quantities beyond JavaScript numeric capacity. Discover function parameters and available remote operations with ui_catalog. Each/map/filter/groupBy require explicit iteration limits. Data-dependent views must provide valid loading/default values.",
  );
export type UiDocument = z.infer<typeof requestSchema>;
export const documentSchema = requestSchema.extend({
  bindings: z.array(capturedBindingSchema).max(limits.operations),
});
export type AcceptedDocument = z.infer<typeof documentSchema>;

function schemaProjection(schema: z.ZodType): {
  type: "object";
  [keyword: string]: unknown;
} {
  const projected = z.toJSONSchema(schema, { reused: "ref" });
  if (projected.type !== "object")
    throw new Error("genui: document schema requires an object root");
  return { ...projected, type: projected.type };
}
export const requestJSONSchema = schemaProjection(requestSchema);
export const documentJSONSchema = schemaProjection(documentSchema);

function normalized(value: unknown): unknown {
  if (isJSONNumber(value)) {
    try {
      return projectNumber(value);
    } catch (cause) {
      if (cause instanceof RangeError || cause instanceof TypeError)
        throw new InvalidPresentation(
          "numeric literals must be exactly representable; use strings for exact quantities",
          cause,
        );
      throw cause;
    }
  }
  if (Array.isArray(value)) return value.map(normalized);
  if (value !== null && typeof value === "object") {
    if (
      Object.getPrototypeOf(value) !== Object.prototype &&
      Object.getPrototypeOf(value) !== null
    )
      throw new InvalidPresentation(
        "document values must be plain JSON records",
      );
    return Object.fromEntries(
      Object.entries(value).map(([key, value]) => [key, normalized(value)]),
    );
  }
  return value;
}

function checkContent(value: unknown): void {
  assertValueBudget(value);
  let encoded: string;
  try {
    encoded = stringifyJSON(value);
  } catch (cause) {
    if (cause instanceof SyntaxError)
      throw new InvalidPresentation("invalid document JSON", cause);
    throw cause;
  }
  if (new TextEncoder().encode(encoded).length > limits.bytes)
    throw new InvalidPresentation("document exceeds encoded content capacity");
}

function unique(items: readonly string[], kind: string): Set<string> {
  const values = new Set(items);
  if (values.size !== items.length)
    throw new InvalidPresentation("duplicate " + kind + " identifier");
  return values;
}

interface FieldConstraint {
  id: string;
  kind: "string" | "strings" | "boolean" | "number" | "index";
  node: UiNode;
}
function constraints(node: UiNode): FieldConstraint[] {
  const props = node.props;
  const field = (
    key: string,
    kind: FieldConstraint["kind"],
  ): FieldConstraint[] =>
    typeof props[key] === "string" ? [{ id: props[key], kind, node }] : [];
  switch (node.type) {
    case "Input":
    case "TextArea":
    case "Select":
    case "RadioGroup":
    case "DatePicker":
    case "Tabs":
      return field("field", "string");
    case "Checkbox":
    case "Switch":
    case "Disclosure":
    case "Popover":
    case "Flashcard":
      return field("field", "boolean");
    case "MultiSelect":
    case "Accordion":
      return field("field", "strings");
    case "Slider":
      return field("field", "number");
    case "Timer":
      return [...field("field", "number"), ...field("runningState", "boolean")];
    case "Carousel":
      return field("field", "index");
    case "Table":
      return [
        ...field("searchState", "string"),
        ...field("sortState", "string"),
        ...field("pageState", "index"),
        ...field("selectState", "strings"),
      ];
    case "Map":
      return field("selectedState", "string");
    default:
      return [];
  }
}

function checkState(constraint: FieldConstraint, value: unknown): void {
  const valid =
    constraint.kind === "strings"
      ? Array.isArray(value) && value.every((item) => typeof item === "string")
      : constraint.kind === "index"
        ? typeof value === "number" && Number.isSafeInteger(value) && value >= 0
        : typeof value === constraint.kind;
  if (!valid)
    throw new InvalidPresentation(
      "state shape differs from its control: " + constraint.id,
    );
  const { node } = constraint;
  if (
    node.type === "Slider" &&
    ((value as number) < (node.props.min as number) ||
      (value as number) > (node.props.max as number))
  )
    throw new InvalidPresentation("slider state is outside its declared range");
  if (
    node.type === "Timer" &&
    constraint.kind === "number" &&
    (value as number) < 0
  )
    throw new InvalidPresentation("timer seconds cannot be negative");
  if (
    node.type === "Carousel" &&
    (value as number) >= (node.children?.length ?? 0)
  )
    throw new InvalidPresentation("carousel index is outside its children");
  if (
    ["Select", "RadioGroup", "MultiSelect", "Tabs", "Accordion"].includes(
      node.type,
    )
  ) {
    const options = (node.props.options ?? node.props.items) as Array<{
      value: string;
    }>;
    const selected = Array.isArray(value) ? value : value === "" ? [] : [value];
    if (
      selected.some((item) => !options.some((option) => option.value === item))
    )
      throw new InvalidPresentation("selected state is not a declared option");
    if (node.type === "Tabs" && value === "")
      throw new InvalidPresentation("tabs require a selected declared panel");
  }
  if (
    node.type === "Table" &&
    constraint.id === node.props.sortState &&
    value !== ""
  ) {
    const columns = node.props.columns as Array<{ key: string }>;
    if (
      !columns.some(
        (column) =>
          value === column.key + ":asc" || value === column.key + ":desc",
      )
    )
      throw new InvalidPresentation(
        "table sort state is not a declared column and direction",
      );
  }
}

function allNodes(nodes: readonly UiNode[]): UiNode[] {
  return nodes.flatMap((node) => [node, ...allNodes(node.children ?? [])]);
}

function checkDocument(document: UiDocument): void {
  const states = unique(
    document.state.map((value) => value.id),
    "state",
  );
  const actions = unique(
    document.actions.map((value) => value.id),
    "action",
  );
  const operations = unique(
    document.operations.map((value) => value.id),
    "operation",
  );
  const nodes = allNodes(document.nodes);
  unique(
    nodes.map((value) => value.id),
    "node",
  );
  const timers = nodes.filter((node) => node.type === "Timer");
  unique(
    timers.map((node) => node.props.field as string),
    "timer clock state",
  );
  unique(
    timers.map((node) => node.props.runningState as string),
    "timer running state",
  );
  if (nodes.length > limits.nodes)
    throw new InvalidPresentation("document exceeds authored node capacity");
  const expression = (
    value: Expression,
    locals = new Set<string>(),
  ): number => {
    expressionReferences(
      value,
      (kind, id) => {
        if (
          (kind === "state" && !states.has(id)) ||
          (kind === "operation" && !operations.has(id)) ||
          (kind === "item" && !locals.has(id))
        )
          throw new InvalidPresentation(
            "undeclared " + kind + " reference: " + id,
          );
      },
      locals,
    );
    return expressionCost(value);
  };
  const walk = (
    node: UiNode,
    depth: number,
    locals: Set<string>,
    svg: boolean,
    form: boolean,
    copies: number,
  ): { rendered: number; evaluations: number } => {
    if (depth > limits.depth)
      throw new InvalidPresentation("document exceeds nesting capacity");
    const props = node.props;
    if (node.type === "Timer" && copies > 1)
      throw new InvalidPresentation(
        "a timer clock cannot be repeated by an iteration",
      );
    let evaluations = 1;
    let multiplier = 1;
    let childrenScope = locals;
    if (node.type === "Each") {
      evaluations += expression(props.items as Expression, locals);
      multiplier = props.limit as number;
      childrenScope = new Set([...locals, props.as as string]);
    } else {
      for (const value of Object.values(props))
        if (isExpression(value)) evaluations += expression(value, locals);
      if (typeof props.action === "string" && !actions.has(props.action))
        throw new InvalidPresentation("undeclared action reference");
      for (const constraint of constraints(node)) {
        if (!states.has(constraint.id))
          throw new InvalidPresentation(
            "control refers to undeclared local state",
          );
        checkState(
          constraint,
          document.state.find((value) => value.id === constraint.id)!.value,
        );
      }
      if (!Object.values(props).some(isExpression))
        validateResolvedProps(node.type, {
          ...props,
          ...(displayDefinitions[node.type].children
            ? { children: node.children ?? [] }
            : {}),
        });
    }
    const primitive = [
      "G",
      "Circle",
      "Ellipse",
      "Rect",
      "Line",
      "Path",
      "Polyline",
      "Polygon",
      "SvgText",
    ].includes(node.type);
    if (primitive && !svg)
      throw new InvalidPresentation("SVG primitives require an SVG parent");
    if (svg && !primitive && !["Each", "If"].includes(node.type))
      throw new InvalidPresentation("SVG accepts only its primitive children");
    if (form && node.type === "Form")
      throw new InvalidPresentation("forms cannot nest");
    let rendered = node.type === "Each" ? 0 : 1;
    for (const child of node.children ?? []) {
      const cost = walk(
        child,
        depth + 1,
        childrenScope,
        svg || node.type === "Svg",
        form || node.type === "Form",
        copies * multiplier,
      );
      rendered += multiplier * cost.rendered;
      evaluations += multiplier * cost.evaluations;
    }
    return { rendered, evaluations };
  };
  let rendered = 1,
    evaluations = 0;
  for (const node of document.nodes) {
    const cost = walk(node, 1, new Set(), false, false, 1);
    rendered += cost.rendered;
    evaluations += cost.evaluations;
  }
  for (const operation of document.operations)
    evaluations += expression(operation.input);
  for (const action of document.actions)
    for (const step of action.steps) {
      if (step.type === "set") {
        if (!states.has(step.state))
          throw new InvalidPresentation(
            "set may only write declared local state",
          );
        evaluations += expression(step.value);
      } else if (step.type === "reset") {
        if (step.states.some((id) => !states.has(id)))
          throw new InvalidPresentation(
            "reset may only write declared local state",
          );
      } else if (step.type === "operation") {
        if (!operations.has(step.operation))
          throw new InvalidPresentation(
            "action refers to undeclared operation",
          );
      } else
        evaluations += expression(
          step.type === "followup"
            ? step.message
            : step.type === "copy"
              ? step.text
              : step.url,
        );
    }
  if (rendered > limits.renderedNodes || evaluations > limits.evaluationSteps)
    throw new InvalidPresentation(
      "document exceeds expansion or evaluation capacity",
    );
}

export function parseDocument(value: unknown): UiDocument {
  assertValueBudget(value);
  const result = requestSchema.safeParse(normalized(value));
  if (!result.success)
    throw new InvalidPresentation("invalid interactive document", result.error);
  checkContent(result.data);
  checkDocument(result.data);
  return result.data;
}

function freeze<T>(value: T): T {
  if (value !== null && typeof value === "object") {
    for (const child of Object.values(value)) freeze(child);
    Object.freeze(value);
  }
  return value;
}

function capture(document: UiDocument, values: unknown): CapturedBinding[] {
  const result = z
    .array(capturedBindingSchema)
    .max(limits.operations)
    .safeParse(values);
  if (!result.success)
    throw new InvalidPresentation("invalid captured bindings", result.error);
  unique(
    result.data.map((value) => value.id),
    "binding",
  );
  if (result.data.length !== document.operations.length)
    throw new InvalidPresentation(
      "bindings must correspond exactly to declared operations",
    );
  for (const request of document.operations) {
    const captured = result.data.find((value) => value.id === request.id);
    if (!captured)
      throw new InvalidPresentation("missing captured operation binding");
    const binding = captured.binding;
    if (
      binding.installationId !== request.installationId ||
      binding.contribution !== request.contribution ||
      binding.method !== request.method ||
      (request.target !== undefined &&
        !equalJSON(request.target, binding.target))
    )
      throw new InvalidPresentation(
        "captured binding differs from declared operation",
      );
    let method;
    try {
      method = pluginMethod(binding);
    } catch (cause) {
      if (cause instanceof TypeError || cause instanceof SyntaxError)
        throw new InvalidPresentation("captured method is invalid", cause);
      throw cause;
    }
    if (
      method.target !== binding.target.scope ||
      (method.target === "installation" &&
        binding.target.reference !== binding.installationId)
    )
      throw new InvalidPresentation("captured target differs from its method");
    if (request.auto && method.kind !== "query")
      throw new InvalidPresentation("automatic operations must be queries");
  }
  return result.data;
}

export class Presentation {
  readonly #document: AcceptedDocument;
  private constructor(document: AcceptedDocument) {
    this.#document = freeze(document);
  }

  static accept(
    value: unknown,
    bindings: CapturedBinding[] = [],
  ): Presentation {
    const document = parseDocument(value);
    const accepted = { ...document, bindings: capture(document, bindings) };
    checkContent(accepted);
    const presentation = new Presentation(accepted);
    presentation.#validateProjection();
    return presentation;
  }

  static read(source: string): Presentation {
    let value: unknown;
    try {
      value = parseJSON(source);
    } catch (cause) {
      if (cause instanceof SyntaxError)
        throw new InvalidPresentation("invalid document JSON", cause);
      throw cause;
    }
    assertValueBudget(value);
    const accepted = documentSchema.safeParse(normalized(value));
    if (!accepted.success)
      throw new InvalidPresentation(
        "invalid accepted document",
        accepted.error,
      );
    const { bindings, ...document } = accepted.data;
    // Transport indentation is not authored content and does not consume this budget.
    return Presentation.accept(document, bindings);
  }

  snapshot(): AcceptedDocument {
    return structuredClone(this.#document);
  }
  json(): string {
    return stringifyJSON(this.#document, true);
  }
  initialValue(id: string): JSONValue {
    const value = this.#document.state.find((value) => value.id === id);
    if (!value) throw new InvalidPresentation("undeclared local state");
    return structuredClone(value.value);
  }
  initialState(): Record<string, unknown> {
    return Object.fromEntries([
      ...this.#document.state.map(({ id, value }) => [
        stateKey(id),
        { value: structuredClone(value), componentType: "GenUI" },
      ]),
      ...this.#document.operations.map(({ id }) => [
        operationKey(id),
        { value: { status: "idle" }, componentType: "GenUI" },
      ]),
    ]);
  }
  operation(id: string): {
    request: OperationRequest;
    binding: Omit<CallPluginRequest, "input">;
  } {
    const request = this.#document.operations.find((value) => value.id === id);
    const binding = this.#document.bindings.find(
      (value) => value.id === id,
    )?.binding;
    if (!request || !binding)
      throw new InvalidPresentation("undeclared operation");
    return structuredClone({ request, binding });
  }
  operations(): Array<{
    request: OperationRequest;
    binding: Omit<CallPluginRequest, "input">;
  }> {
    return this.#document.operations.map(({ id }) => this.operation(id));
  }
  evaluate(
    expression: Expression,
    readLocal: (id: string) => unknown,
    readOperation: (id: string) => unknown,
    scope: Record<string, unknown> = {},
  ): unknown {
    return evaluateExpression(
      expression,
      (id) => {
        const value = readLocal(id);
        return value === undefined ? this.initialValue(id) : value;
      },
      (id) => readOperation(id) ?? { status: "idle" },
      scope,
    );
  }
  validateState(id: string, value: unknown): JSONValue {
    this.initialValue(id);
    assertValueBudget(value);
    const parsed = jsonValueSchema.safeParse(normalized(value));
    if (!parsed.success)
      throw new InvalidPresentation("invalid local state value", parsed.error);
    for (const node of allNodes(this.#document.nodes))
      for (const constraint of constraints(node))
        if (constraint.id === id) checkState(constraint, parsed.data);
    return parsed.data;
  }
  formStateIds(id: string): string[] {
    const fields = allNodes(this.#document.nodes)
      .filter((node) => node.type === "Form" && node.props.action === id)
      .flatMap((form) =>
        allNodes(form.children ?? []).flatMap((node) =>
          constraints(node).map((constraint) => constraint.id),
        ),
      );
    return [...new Set(fields)];
  }
  validateAction(
    id: string,
    readLocal: (id: string) => unknown,
    readOperation: (id: string) => unknown = () => undefined,
  ): Array<{ field: string; issue: ValidationIssue }> {
    const failures: Array<{ field: string; issue: ValidationIssue }> = [];
    const local = (field: string) => {
      const value = readLocal(field);
      return value === undefined ? this.initialValue(field) : value;
    };
    const visit = (
      node: UiNode,
      associated: boolean,
      scope: Record<string, unknown>,
    ) => {
      if (node.type === "Each") {
        const items = this.evaluate(
          node.props.items as Expression,
          readLocal,
          readOperation,
          scope,
        );
        if (
          !Array.isArray(items) ||
          items.length > (node.props.limit as number)
        )
          throw new InvalidPresentation(
            "form iteration exceeds its declared limit",
          );
        for (const item of items)
          visit(node.children![0]!, associated, {
            ...scope,
            [node.props.as as string]: item,
          });
        return;
      }
      if (node.type === "If") {
        const when = node.props.when;
        const visible = isExpression(when)
          ? this.evaluate(when, readLocal, readOperation, scope)
          : when;
        if (typeof visible !== "boolean")
          throw new InvalidPresentation("form visibility requires a boolean");
        if (!visible) return;
      }
      if (
        ["Disclosure", "Popover"].includes(node.type) &&
        !local(node.props.field as string)
      )
        return;
      const inForm =
        associated || (node.type === "Form" && node.props.action === id);
      const field = node.props.field;
      if (
        inForm &&
        typeof field === "string" &&
        [
          "Input",
          "TextArea",
          "Select",
          "MultiSelect",
          "RadioGroup",
          "DatePicker",
        ].includes(node.type)
      ) {
        const kind: FieldKind =
          node.type === "Input"
            ? ((node.props.type as FieldKind | undefined) ?? "text")
            : node.type === "DatePicker"
              ? "date"
              : "text";
        const issue = validateField(
          local(field),
          node.props.rules as FieldRules | undefined,
          kind,
          node.type === "DatePicker"
            ? (node.props as { min?: string; max?: string })
            : {},
        );
        if (issue && !failures.some((failure) => failure.field === field))
          failures.push({ field, issue });
      }
      const children = node.children ?? [];
      for (const [index, child] of children.entries()) {
        if (node.type === "Tabs" || node.type === "Accordion") {
          const selected = local(node.props.field as string);
          const item = (node.props.items as Array<{ value: string }>)[index]!;
          if (
            node.type === "Tabs"
              ? selected !== item.value
              : !(selected as string[]).includes(item.value)
          )
            continue;
        }
        if (
          node.type === "Carousel" &&
          local(node.props.field as string) !== index
        )
          continue;
        visit(child, inForm, scope);
      }
    };
    for (const node of this.#document.nodes) visit(node, false, {});
    return failures;
  }
  tree(): ElementNode {
    const initial = Object.fromEntries(
      Object.entries(this.initialState()).map(([key, value]) => [
        key,
        (value as { value: unknown }).value,
      ]),
    );
    const nodeAST = (node: UiNode): ASTNode => {
      if (node.type === "Each")
        return each(
          node.props.items as Expression,
          node.props.as as string,
          nodeAST(node.children![0]!),
          node.props.limit as number,
          initial,
        );
      const definition = displayDefinitions[node.type];
      const args = Object.keys(definition.shape).map((key): ASTNode => {
        const value = node.props[key];
        if (value === undefined) return { k: "Null" };
        return compileExpression(
          isExpression(value) ? value : { literal: value as JSONValue },
          initial,
        );
      });
      if (definition.children)
        args.push({ k: "Arr", els: (node.children ?? []).map(nodeAST) });
      return { k: "Comp", name: node.type, args };
    };
    return {
      type: "element",
      typeName: "Document",
      props: {
        title: this.#document.title,
        children: this.#document.nodes.map(nodeAST),
      },
      partial: false,
      hasDynamicProps: true,
    };
  }
  openUI(library: Library): string {
    return jsonToOpenUI(this.tree(), library);
  }
  #validateProjection(): void {
    const renderers = Object.fromEntries(
      Object.keys(displayDefinitions).map((name) => [name, () => null]),
    ) as unknown as Parameters<typeof createDisplayLibrary>[0];
    const library = createDisplayLibrary(renderers);
    const parsed = createParser(library.toJSONSchema()).parse(
      this.openUI(library),
    );
    if (
      !parsed.root ||
      parsed.meta.incomplete ||
      parsed.meta.errors.length ||
      parsed.meta.unresolved.length ||
      parsed.queryStatements.length ||
      parsed.mutationStatements.length
    )
      throw new InvalidPresentation(
        "document does not have a complete display projection",
      );
    const errors: OpenUIError[] = [];
    const root = evaluateRoot(parsed, {
      library,
      store: null,
      errors,
      ctx: {
        getState: (key) =>
          key.startsWith("$input_")
            ? this.initialValue(key.slice(7))
            : { status: "idle" },
        resolveRef: () => null,
        functions: runtimeFunctions(),
      },
    });
    if (errors.length)
      throw new InvalidPresentation(
        "initial expression failed: " +
          errors.map((error) => error.message).join("; "),
      );
    const check = (value: unknown): void => {
      if (Array.isArray(value)) {
        value.forEach(check);
        return;
      }
      if (
        value === null ||
        typeof value !== "object" ||
        !("type" in value) ||
        value.type !== "element" ||
        !("typeName" in value) ||
        !("props" in value)
      )
        throw new InvalidPresentation("display projection omitted an element");
      const name = value.typeName as DisplayComponentName;
      if (!Object.hasOwn(displayDefinitions, name))
        throw new InvalidPresentation(
          "display projection contains an undeclared component",
        );
      const props = validateResolvedProps(name, value.props);
      if ("children" in props) check(props.children);
    };
    check(root);
  }
  text(): string {
    const lines = [this.#document.title];
    let remaining = 4096;
    for (const node of allNodes(this.#document.nodes)) {
      const values = [
        node.props.title,
        node.props.caption,
        node.props.label,
        node.props.text,
        node.props.value,
      ];
      const line = values
        .filter(
          (value) => typeof value === "string" || typeof value === "number",
        )
        .map(String)
        .join(": ");
      if (!line || remaining <= 0) continue;
      const excerpt = Array.from(line)
        .slice(0, Math.min(remaining, 512))
        .join("");
      lines.push(excerpt + (excerpt.length < line.length ? "…" : ""));
      remaining -= excerpt.length;
    }
    if (this.#document.actions.length)
      lines.push(
        "Available actions: " +
          this.#document.actions.map(({ id }) => id).join(", "),
      );
    if (this.#document.operations.length)
      lines.push(
        "Declared operations: " +
          this.#document.operations.map(({ id }) => id).join(", "),
      );
    lines.push(
      "The complete accepted interface is available in the structured result. Interactive controls require a graphical client.",
    );
    return lines.join("\n");
  }
}
