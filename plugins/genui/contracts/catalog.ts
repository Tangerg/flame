import {
  createLibrary,
  defineComponent,
  type ElementNode,
  type Library,
} from "@openuidev/lang-core";
import type { ComponentRenderProps } from "@openuidev/react-lang";
import type { ReactNode } from "react";
import { z } from "zod/v4";
import {
  assertValueBudget,
  expressionFunctions,
  expressionSchema,
  finiteNumber,
  identifier,
  InvalidPresentation,
  jsonValueSchema,
  limits,
  type Expression,
} from "./expression";
import { fieldRulesSchema, isSafeURL, validateField } from "./validation";

const label = z.string().min(1).max(256).regex(/\S/u);
const text = z.string().max(limits.text);
const choice = z.strictObject({ value: identifier, label });
const choices = z.array(choice).min(1).max(128);
const tone = z.enum([
  "neutral",
  "info",
  "accent",
  "success",
  "warning",
  "negative",
]);
const color = z
  .string()
  .max(128)
  .regex(
    /^(?:none|currentColor|[a-zA-Z]{1,30}|#[0-9a-fA-F]{3,8}|(?:rgb|rgba|hsl|hsla)\([0-9.,% +\-/]+\))$/u,
  );
const paint = {
  fill: color.optional(),
  stroke: color.optional(),
  strokeWidth: finiteNumber.min(0).optional(),
};
const point = finiteNumber;
const state = identifier;
const coordinate = {
  latitude: finiteNumber.min(-85.0511287798066).max(85.0511287798066),
  longitude: finiteNumber.min(-180).max(180),
};

function component<
  Shape extends z.ZodRawShape,
  Children extends boolean = false,
>(
  shape: Shape,
  description: string,
  children: Children = false as Children,
  fixed: readonly (keyof Shape & string)[] = [],
) {
  return { shape, description, children, fixed };
}

// This catalog owns authored props and their evaluated renderer contract.
// Authority-bearing field and action identifiers remain literal.
export const displayDefinitions = {
  Document: component({ title: label }, "A titled interactive document.", true),
  Stack: component(
    {
      direction: z.enum(["vertical", "horizontal"]),
      gap: z.enum(["small", "medium", "large"]).optional(),
    },
    "A row or column of children.",
    true,
  ),
  Grid: component(
    { columns: z.number().int().min(1).max(6) },
    "A responsive grid.",
    true,
  ),
  Card: component(
    { title: label.optional(), description: text.optional() },
    "A card grouping related content.",
    true,
  ),
  Section: component({ title: label }, "A titled section.", true),
  Tabs: component(
    { field: state, items: choices },
    "Tab selection uses a declared string state.",
    true,
    ["field", "items"],
  ),
  Accordion: component(
    { field: state, items: choices },
    "Expanded sections use a declared string-array state.",
    true,
    ["field", "items"],
  ),
  Disclosure: component(
    { field: state, title: label },
    "Visibility uses declared boolean state.",
    true,
    ["field"],
  ),
  Carousel: component(
    { field: state },
    "Visible child uses a declared zero-based index state.",
    true,
    ["field"],
  ),
  Popover: component(
    { field: state, label },
    "An anchored panel using declared boolean state.",
    true,
    ["field"],
  ),
  If: component(
    { when: z.boolean() },
    "Show children when the pure condition is true.",
    true,
  ),
  Divider: component({}, "A thematic divider."),
  Text: component({ text }, "Literal text, including evaluated data."),
  Markdown: component(
    { text },
    "Markdown without raw HTML; destinations follow the document URL policy.",
  ),
  Code: component(
    { code: text, language: z.string().max(32).optional() },
    "Literal source code without execution.",
  ),
  Badge: component(
    { text: label, tone: tone.optional() },
    "A short status label.",
  ),
  Callout: component(
    { title: label.optional(), text, tone: tone.optional() },
    "A highlighted message.",
  ),
  Link: component(
    { label, href: z.string().max(4096) },
    "A link with a permitted destination.",
  ),
  Image: component(
    { src: z.string().max(4096), alt: label, caption: text.optional() },
    "An HTTPS image with alternative text.",
  ),
  Metric: component(
    { label, value: z.union([text, finiteNumber]), detail: text.optional() },
    "A value and optional context. Exact quantities should be strings.",
  ),
  List: component(
    {
      items: z.array(text).max(limits.collection),
      ordered: z.boolean().optional(),
    },
    "A bounded text list.",
  ),
  Progress: component(
    {
      label,
      value: finiteNumber.min(0),
      max: finiteNumber.positive().optional(),
    },
    "A labeled progress indicator.",
  ),
  Table: component(
    {
      caption: label,
      columns: z
        .array(z.strictObject({ key: identifier, label }))
        .min(1)
        .max(32),
      rows: z
        .array(z.record(z.string().max(256), jsonValueSchema))
        .max(limits.collection),
      rowKey: identifier,
      searchState: state.optional(),
      sortState: state.optional(),
      pageState: state.optional(),
      selectState: state.optional(),
      pageSize: z.number().int().min(1).max(100).optional(),
    },
    "A searchable, sortable, pageable table with explicit stable row identifiers.",
    false,
    [
      "rowKey",
      "columns",
      "searchState",
      "sortState",
      "pageState",
      "selectState",
      "pageSize",
    ],
  ),
  Chart: component(
    {
      kind: z.enum(["bar", "line", "area", "pie", "donut", "scatter"]),
      caption: label,
      labels: z.array(label).max(256),
      series: z
        .array(
          z.strictObject({ label, values: z.array(finiteNumber).max(256) }),
        )
        .min(1)
        .max(12),
      xValues: z.array(finiteNumber).max(256).optional(),
    },
    "A chart with an accessible data table and finite numeric values.",
  ),
  Input: component(
    {
      field: state,
      label,
      type: z.enum(["text", "number", "email"]).optional(),
      placeholder: text.optional(),
      rules: fieldRulesSchema.optional(),
    },
    "A text input; number inputs preserve the text draft until explicitly converted.",
    false,
    ["field", "type", "rules"],
  ),
  TextArea: component(
    {
      field: state,
      label,
      placeholder: text.optional(),
      rows: z.number().int().min(1).max(30).optional(),
      rules: fieldRulesSchema.optional(),
    },
    "A multiline input.",
    false,
    ["field", "rules"],
  ),
  Select: component(
    {
      field: state,
      label,
      options: choices,
      rules: fieldRulesSchema.optional(),
    },
    "A single selected string value.",
    false,
    ["field", "options", "rules"],
  ),
  MultiSelect: component(
    {
      field: state,
      label,
      options: choices,
      rules: fieldRulesSchema.optional(),
    },
    "A list of selected string values.",
    false,
    ["field", "options", "rules"],
  ),
  RadioGroup: component(
    {
      field: state,
      label,
      options: choices,
      rules: fieldRulesSchema.optional(),
    },
    "A single choice presented as radio controls.",
    false,
    ["field", "options", "rules"],
  ),
  Checkbox: component({ field: state, label }, "A boolean checkbox.", false, [
    "field",
  ]),
  Switch: component({ field: state, label }, "A boolean switch.", false, [
    "field",
  ]),
  Slider: component(
    {
      field: state,
      label,
      min: finiteNumber,
      max: finiteNumber,
      step: finiteNumber.positive().optional(),
    },
    "A numeric range control.",
    false,
    ["field", "min", "max", "step"],
  ),
  DatePicker: component(
    {
      field: state,
      label,
      min: z
        .string()
        .regex(/^\d{4}-\d{2}-\d{2}$/u)
        .optional(),
      max: z
        .string()
        .regex(/^\d{4}-\d{2}-\d{2}$/u)
        .optional(),
    },
    "A calendar date stored as YYYY-MM-DD text.",
    false,
    ["field", "min", "max"],
  ),
  Form: component(
    {
      title: label.optional(),
      action: identifier,
      submitLabel: label,
      resetLabel: label.optional(),
    },
    "Validate declared fields before running the referenced action.",
    true,
    ["action"],
  ),
  Button: component(
    {
      label,
      action: identifier,
      disabled: z.boolean().optional(),
      tone: z.enum(["negative", "warning", "accent", "success"]).optional(),
    },
    "Run an explicitly declared action after a user click.",
    false,
    ["action"],
  ),
  Math: component(
    { expression: text, display: z.boolean().optional() },
    "Mathematical notation with trusted commands disabled.",
  ),
  Timer: component(
    { field: state, runningState: state, label },
    "A local countdown using numeric seconds and boolean running state.",
    false,
    ["field", "runningState"],
  ),
  Flashcard: component(
    { field: state, front: text, back: text },
    "A question and answer with boolean flip state.",
    false,
    ["field"],
  ),
  YouTube: component(
    {
      videoId: z.string().regex(/^[A-Za-z0-9_-]{11}$/u),
      title: label,
      start: z.number().int().min(0).max(86400).optional(),
    },
    "A YouTube privacy-enhanced player with an explicit video identifier.",
  ),
  Map: component(
    {
      caption: label,
      markers: z
        .array(
          z.strictObject({
            id: identifier,
            label,
            ...coordinate,
            description: text.optional(),
          }),
        )
        .max(256),
      route: z
        .array(z.strictObject(coordinate))
        .max(limits.collection)
        .optional(),
      selectedState: state.optional(),
    },
    "A Web Mercator map, supplied route and accessible marker list; latitude must fit its projection.",
    false,
    ["selectedState"],
  ),
  HtmlView: component(
    {
      title: label,
      html: z.string().max(limits.html),
      height: z.number().int().min(100).max(1600).optional(),
    },
    "An isolated opaque HTML preview with no Runtime bridge.",
  ),
  Svg: component(
    {
      label,
      viewBox: z
        .string()
        .max(256)
        .regex(
          /^-?[0-9.eE+]+[ ,]+-?[0-9.eE+]+[ ,]+[0-9.eE+]+[ ,]+[0-9.eE+]+$/u,
        ),
      width: finiteNumber.positive().max(4096).optional(),
      height: finiteNumber.positive().max(4096).optional(),
    },
    "An accessible SVG made only from the declared primitives.",
    true,
  ),
  G: component(
    {
      transform: z
        .string()
        .max(512)
        .regex(
          /^(?:(?:translate|scale|rotate|skewX|skewY|matrix)\([0-9eE+.,\- ]+\)\s*)*$/u,
        )
        .optional(),
    },
    "An SVG transform group.",
    true,
  ),
  Circle: component(
    { cx: point, cy: point, r: point.min(0), ...paint },
    "An SVG circle.",
  ),
  Ellipse: component(
    { cx: point, cy: point, rx: point.min(0), ry: point.min(0), ...paint },
    "An SVG ellipse.",
  ),
  Rect: component(
    {
      x: point,
      y: point,
      width: point.min(0),
      height: point.min(0),
      rx: point.min(0).optional(),
      ...paint,
    },
    "An SVG rectangle.",
  ),
  Line: component(
    { x1: point, y1: point, x2: point, y2: point, ...paint },
    "An SVG line.",
  ),
  Path: component(
    {
      d: z
        .string()
        .max(limits.text)
        .regex(/^[MmZzLlHhVvCcSsQqTtAa0-9eE+.,\-\s]*$/u),
      ...paint,
    },
    "An SVG path with fixed numeric drawing commands.",
  ),
  Polyline: component(
    {
      points: z
        .string()
        .max(limits.text)
        .regex(/^[0-9eE+.,\-\s]*$/u),
      ...paint,
    },
    "An SVG polyline.",
  ),
  Polygon: component(
    {
      points: z
        .string()
        .max(limits.text)
        .regex(/^[0-9eE+.,\-\s]*$/u),
      ...paint,
    },
    "An SVG polygon.",
  ),
  SvgText: component(
    {
      x: point,
      y: point,
      text,
      fill: color.optional(),
      fontSize: finiteNumber.positive().max(512).optional(),
    },
    "Literal SVG text.",
  ),
} as const;

export type DisplayComponentName = keyof typeof displayDefinitions;
type Shape<Name extends DisplayComponentName> =
  (typeof displayDefinitions)[Name]["shape"];
export type DisplayProps = {
  [Name in DisplayComponentName]: z.infer<z.ZodObject<Shape<Name>>> &
    ((typeof displayDefinitions)[Name]["children"] extends true
      ? { children: ElementNode[] }
      : Record<never, never>);
};
export type AuthoredProps<Name extends DisplayComponentName> = {
  [Key in keyof z.infer<z.ZodObject<Shape<Name>>>]:
    z.infer<z.ZodObject<Shape<Name>>>[Key] | Expression;
};

export function authoredPropsSchema(name: DisplayComponentName) {
  const definition = displayDefinitions[name];
  return z.strictObject(
    Object.fromEntries(
      Object.entries(definition.shape).map(([key, schema]) => {
        if ((definition.fixed as readonly string[]).includes(key))
          return [key, schema];
        const dynamic = z.union([schema, expressionSchema]);
        return [key, schema.isOptional() ? dynamic.optional() : dynamic];
      }),
    ),
  );
}

function distinct(items: readonly string[], message: string): void {
  if (new Set(items).size !== items.length)
    throw new InvalidPresentation(message);
}

export function validateResolvedProps<Name extends DisplayComponentName>(
  name: Name,
  value: unknown,
): DisplayProps[Name] {
  if (name === "Document") assertValueBudget(value);
  const definition = displayDefinitions[name];
  const schema = z.strictObject({
    ...definition.shape,
    ...(definition.children
      ? { children: z.array(z.unknown()).max(limits.renderedNodes) }
      : {}),
  });
  // OpenUI positional arguments encode an omitted optional position as null.
  // The accepted JSON remains strict; only this projection translates omission.
  const propsValue =
    value !== null && typeof value === "object" && !Array.isArray(value)
      ? Object.fromEntries(
          Object.entries(value).filter(
            ([key, value]) =>
              !(
                value === null &&
                Object.hasOwn(definition.shape, key) &&
                (
                  (definition.shape as z.ZodRawShape)[key] as z.ZodType
                ).isOptional()
              ),
          ),
        )
      : value;
  const result = schema.safeParse(propsValue);
  if (!result.success)
    throw new InvalidPresentation(
      "invalid evaluated " + name + " props",
      result.error,
    );
  const props = result.data as Record<string, unknown>;
  if (name === "Link" && !isSafeURL(props.href as string, "link"))
    throw new InvalidPresentation("unsafe link URL");
  if (name === "Image" && !isSafeURL(props.src as string, "image"))
    throw new InvalidPresentation("unsafe image URL");
  if (name === "Table") {
    const table = props as unknown as DisplayProps["Table"];
    distinct(
      table.columns.map((column) => column.key),
      "table column keys must be unique",
    );
    const ids = table.rows.map((row) => row[table.rowKey]);
    if (ids.some((id) => typeof id !== "string" || !id))
      throw new InvalidPresentation(
        "table rows require stable string identifiers",
      );
    distinct(ids as string[], "table row identifiers must be unique");
    if (table.rows.length * table.columns.length > limits.tableCells)
      throw new InvalidPresentation("table exceeds cell budget");
  }
  if (name === "Chart") {
    const chart = props as unknown as DisplayProps["Chart"];
    distinct(
      chart.series.map((series) => series.label),
      "chart series labels must be unique",
    );
    if (
      chart.series.some(
        (series) => series.values.length !== chart.labels.length,
      )
    )
      throw new InvalidPresentation("chart series length differs from labels");
    if (
      (chart.kind === "pie" || chart.kind === "donut") &&
      (chart.series.length !== 1 ||
        chart.series[0]!.values.some((value) => value < 0))
    )
      throw new InvalidPresentation(
        "pie and donut charts require one nonnegative series",
      );
    if (
      chart.kind === "scatter" &&
      chart.xValues?.length !== chart.labels.length
    )
      throw new InvalidPresentation("scatter requires one x value per point");
  }
  if (name === "Slider" && (props.min as number) >= (props.max as number))
    throw new InvalidPresentation("slider minimum must be below maximum");
  if (name === "DatePicker") {
    const bounds = props as { min?: string; max?: string };
    if (
      validateField(bounds.min, {}, "date", { max: bounds.max }) ||
      validateField(bounds.max, {}, "date")
    )
      throw new InvalidPresentation("invalid date bounds");
  }
  if (
    name === "Progress" &&
    (props.value as number) > ((props.max as number | undefined) ?? 100)
  )
    throw new InvalidPresentation("progress exceeds its maximum");
  if (name === "Tabs" || name === "Accordion") {
    const tabs = props as unknown as DisplayProps["Tabs"];
    distinct(
      tabs.items.map((item) => item.value),
      "panel values must be unique",
    );
    if (tabs.children.length !== tabs.items.length)
      throw new InvalidPresentation("each panel requires one child");
  }
  if (name === "Select" || name === "MultiSelect" || name === "RadioGroup")
    distinct(
      (props.options as Array<{ value: string }>).map((option) => option.value),
      "option values must be unique",
    );
  if (name === "Map")
    distinct(
      (props.markers as Array<{ id: string }>).map((marker) => marker.id),
      "map marker identifiers must be unique",
    );
  return result.data as DisplayProps[Name];
}

export function createDisplayLibrary(renderers: {
  [Name in DisplayComponentName]: (
    props: ComponentRenderProps<DisplayProps[Name]>,
  ) => ReactNode;
}): Required<Library<(props: ComponentRenderProps) => ReactNode>> {
  const components = Object.entries(displayDefinitions).map(
    ([name, definition]) =>
      defineComponent({
        name,
        description: definition.description,
        props: z.strictObject({
          ...definition.shape,
          ...(definition.children
            ? { children: z.array(z.unknown()).max(limits.renderedNodes) }
            : {}),
        }),
        component: (view: ComponentRenderProps) => {
          const props = validateResolvedProps(
            name as DisplayComponentName,
            view.props,
          );
          const render = renderers[name as DisplayComponentName] as (
            view: ComponentRenderProps,
          ) => ReactNode;
          return render({ ...view, props });
        },
      }),
  );
  return createLibrary({
    root: "Document",
    components,
    functions: expressionFunctions,
  });
}
