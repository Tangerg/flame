import {
  defineFunction,
  evaluate,
  type ASTNode,
  type EvaluationContext,
} from "@openuidev/lang-core";
import {
  equalJSON,
  isJSONNumber,
  parseJSON,
  projectNumber,
  stringifyJSON,
} from "@flame/runtime-client/json";
import { Temporal } from "@js-temporal/polyfill";
import { z } from "zod/v4";

// Captured documents and their derived values must fit history, evaluation and
// display capacity even when one small template expands repeatedly.
export const limits = Object.freeze({
  bytes: 256 * 1024,
  nodes: 256,
  depth: 12,
  collection: 2048,
  text: 16 * 1024,
  html: 64 * 1024,
  states: 128,
  operations: 32,
  actions: 64,
  renderedNodes: 1024,
  tableCells: 8192,
  evaluationSteps: 32768,
  valueNodes: 16384,
  valueDepth: 32,
  valueText: 128 * 1024,
});

export class InvalidPresentation extends Error {
  constructor(message: string, cause?: unknown) {
    super("genui: " + message, cause === undefined ? undefined : { cause });
    this.name = "InvalidPresentation";
  }
}

export type JSONValue =
  null | boolean | number | string | JSONValue[] | { [key: string]: JSONValue };

export const identifier = z.string().regex(/^[A-Za-z][A-Za-z0-9_]{0,63}$/);
export const finiteNumber = z
  .number()
  .finite()
  .min(-Number.MAX_SAFE_INTEGER)
  .max(Number.MAX_SAFE_INTEGER);
export const jsonValueSchema: z.ZodType<JSONValue> = z.lazy(() =>
  z.union([
    z.null(),
    z.boolean(),
    finiteNumber,
    z.string().max(limits.html),
    z.array(jsonValueSchema).max(limits.collection),
    z.record(z.string().max(256), jsonValueSchema),
  ]),
);

const numbers = z.array(finiteNumber).max(limits.collection);
const values = z.array(z.unknown()).max(limits.collection);
const string = z.string().max(limits.html);
const array = z.array(z.unknown()).max(limits.collection);
const path = z.array(z.string().max(256)).max(limits.depth);

export function assertValueBudget(value: unknown): void {
  const pending: Array<{ value: unknown; depth: number }> = [
    { value, depth: 0 },
  ];
  let nodes = 0,
    characters = 0;
  while (pending.length) {
    const entry = pending.pop()!;
    nodes += 1;
    if (nodes > limits.valueNodes || entry.depth > limits.valueDepth)
      throw new InvalidPresentation("value exceeds structural capacity");
    if (typeof entry.value === "string") characters += entry.value.length;
    else if (
      entry.value !== null &&
      typeof entry.value === "object" &&
      !isJSONNumber(entry.value)
    ) {
      const items = Object.entries(entry.value);
      if (items.length > limits.collection)
        throw new InvalidPresentation("value exceeds collection capacity");
      for (const [key, value] of items) {
        characters += key.length;
        pending.push({ value, depth: entry.depth + 1 });
      }
    }
    if (characters > limits.valueText)
      throw new InvalidPresentation("value exceeds text capacity");
  }
}

function boundedJoin(items: readonly string[], separator: string): string {
  const size =
    items.reduce((total, item) => total + item.length, 0) +
    Math.max(0, items.length - 1) * separator.length;
  if (size > limits.html)
    throw new InvalidPresentation("joined text exceeds capacity");
  return items.join(separator);
}

function at(value: unknown, keys: readonly string[]): unknown {
  let current = value;
  for (const key of keys) {
    if (key === "__proto__" || key === "prototype" || key === "constructor") {
      throw new Error("reserved property access");
    }
    if (
      current === null ||
      typeof current !== "object" ||
      !Object.hasOwn(current, key)
    )
      return null;
    current = (current as Record<string, unknown>)[key];
  }
  return current ?? null;
}

function operator<T extends z.ZodType<unknown[]>>(
  parameters: T,
  run: (args: z.infer<T>) => unknown,
  description: string,
  complexity: "linear" | "quadratic" = "linear",
) {
  return { parameters, run, description, complexity };
}

const units = {
  m: { dimension: "length", factor: 1, offset: 0 },
  km: { dimension: "length", factor: 1000, offset: 0 },
  cm: { dimension: "length", factor: 0.01, offset: 0 },
  mm: { dimension: "length", factor: 0.001, offset: 0 },
  inch: { dimension: "length", factor: 0.0254, offset: 0 },
  ft: { dimension: "length", factor: 0.3048, offset: 0 },
  mile: { dimension: "length", factor: 1609.344, offset: 0 },
  kg: { dimension: "mass", factor: 1, offset: 0 },
  g: { dimension: "mass", factor: 0.001, offset: 0 },
  lb: { dimension: "mass", factor: 0.45359237, offset: 0 },
  oz: { dimension: "mass", factor: 0.028349523125, offset: 0 },
  celsius: { dimension: "temperature", factor: 1, offset: 273.15 },
  fahrenheit: {
    dimension: "temperature",
    factor: 5 / 9,
    offset: 255.3722222222222,
  },
  kelvin: { dimension: "temperature", factor: 1, offset: 0 },
  second: { dimension: "time", factor: 1, offset: 0 },
  minute: { dimension: "time", factor: 60, offset: 0 },
  hour: { dimension: "time", factor: 3600, offset: 0 },
  day: { dimension: "time", factor: 86400, offset: 0 },
} as const;
const unitSchema = z.enum(
  Object.keys(units) as [keyof typeof units, ...(keyof typeof units)[]],
);
const date = z.string().regex(/^\d{4}-\d{2}-\d{2}$/u);

function percentile(items: number[], proportion: number): number | null {
  if (!items.length) return null;
  const sorted = [...items].sort((a, b) => a - b);
  const index = (sorted.length - 1) * proportion;
  const lower = Math.floor(index),
    upper = Math.ceil(index);
  return checked(
    sorted[lower]! + (sorted[upper]! - sorted[lower]!) * (index - lower),
  );
}

function checked(value: number): number {
  return finiteNumber.parse(value);
}

export const operators = {
  add: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => checked(a + b),
    "Add two finite display numbers.",
  ),
  subtract: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => checked(a - b),
    "Subtract two finite display numbers.",
  ),
  multiply: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => checked(a * b),
    "Multiply two finite display numbers.",
  ),
  divide: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => (b === 0 ? null : checked(a / b)),
    "Divide; a zero divisor produces null.",
  ),
  modulo: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => (b === 0 ? null : checked(a % b)),
    "Remainder; a zero divisor produces null.",
  ),
  power: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => checked(a ** b),
    "Finite exponentiation.",
  ),
  round: operator(
    z.tuple([finiteNumber, z.number().int().min(0).max(12)]),
    ([value, digits]) => checked(Number(value.toFixed(digits))),
    "Round a display number.",
  ),
  floor: operator(
    z.tuple([finiteNumber]),
    ([value]) => Math.floor(value),
    "Round down.",
  ),
  ceil: operator(
    z.tuple([finiteNumber]),
    ([value]) => Math.ceil(value),
    "Round up.",
  ),
  abs: operator(
    z.tuple([finiteNumber]),
    ([value]) => Math.abs(value),
    "Absolute value.",
  ),
  sin: operator(
    z.tuple([finiteNumber]),
    ([radians]) => checked(Math.sin(radians)),
    "Sine of an angle in radians.",
  ),
  cos: operator(
    z.tuple([finiteNumber]),
    ([radians]) => checked(Math.cos(radians)),
    "Cosine of an angle in radians.",
  ),
  tan: operator(
    z.tuple([finiteNumber]),
    ([radians]) => checked(Math.tan(radians)),
    "Tangent of an angle in radians within display capacity.",
  ),
  sqrt: operator(
    z.tuple([finiteNumber.min(0)]),
    ([value]) => checked(Math.sqrt(value)),
    "Nonnegative square root.",
  ),
  log: operator(
    z.tuple([finiteNumber.positive()]),
    ([value]) => checked(Math.log(value)),
    "Natural logarithm of a positive value.",
  ),
  exp: operator(
    z.tuple([finiteNumber]),
    ([value]) => checked(Math.exp(value)),
    "Natural exponential within finite display capacity.",
  ),
  clamp: operator(
    z.tuple([finiteNumber, finiteNumber, finiteNumber]),
    ([value, minimum, maximum]) => {
      if (minimum > maximum)
        throw new InvalidPresentation("clamp minimum exceeds maximum");
      return Math.min(maximum, Math.max(minimum, value));
    },
    "Clamp a value to the inclusive minimum and maximum.",
  ),
  min: operator(
    z.tuple([numbers]),
    ([items]) => (items.length ? Math.min(...items) : null),
    "Minimum; null for an empty list.",
  ),
  max: operator(
    z.tuple([numbers]),
    ([items]) => (items.length ? Math.max(...items) : null),
    "Maximum; null for an empty list.",
  ),
  sum: operator(
    z.tuple([numbers]),
    ([items]) => checked(items.reduce((sum, value) => sum + value, 0)),
    "Sum finite display numbers.",
  ),
  average: operator(
    z.tuple([numbers]),
    ([items]) =>
      items.length
        ? checked(items.reduce((sum, value) => sum + value, 0) / items.length)
        : null,
    "Arithmetic average.",
  ),
  stats: operator(
    z.tuple([numbers]),
    ([items]) => {
      if (!items.length)
        return { count: 0, sum: 0, min: null, max: null, average: null };
      const sum = checked(items.reduce((sum, value) => sum + value, 0));
      return {
        count: items.length,
        sum,
        min: Math.min(...items),
        max: Math.max(...items),
        average: checked(sum / items.length),
      };
    },
    "Count, sum, minimum, maximum and average.",
  ),
  equal: operator(
    z.tuple([z.unknown(), z.unknown()]),
    ([a, b]) => equalJSON(a, b),
    "Exact JSON equality.",
  ),
  notEqual: operator(
    z.tuple([z.unknown(), z.unknown()]),
    ([a, b]) => !equalJSON(a, b),
    "Exact JSON inequality.",
  ),
  less: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => a < b,
    "Numeric less than.",
  ),
  lessEqual: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => a <= b,
    "Numeric less than or equal.",
  ),
  greater: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => a > b,
    "Numeric greater than.",
  ),
  greaterEqual: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([a, b]) => a >= b,
    "Numeric greater than or equal.",
  ),
  and: operator(
    z.tuple([z.boolean(), z.boolean()]),
    ([a, b]) => a && b,
    "Boolean conjunction.",
  ),
  or: operator(
    z.tuple([z.boolean(), z.boolean()]),
    ([a, b]) => a || b,
    "Boolean disjunction.",
  ),
  not: operator(
    z.tuple([z.boolean()]),
    ([value]) => !value,
    "Boolean negation.",
  ),
  choose: operator(
    z.tuple([z.boolean(), z.unknown(), z.unknown()]),
    ([test, yes, no]) => (test ? yes : no),
    "Choose one value by a boolean.",
  ),
  coalesce: operator(
    z.tuple([values]),
    ([items]) =>
      items.find((item) => item !== null && item !== undefined) ?? null,
    "First non-null value.",
  ),
  string: operator(
    z.tuple([z.unknown()]),
    ([value]) =>
      value === null || value === undefined
        ? ""
        : typeof value === "object"
          ? stringifyJSON(value)
          : String(value),
    "Convert a value to literal text without evaluating markup.",
  ),
  number: operator(
    z.tuple([string]),
    ([value]) => {
      if (!value.trim()) return null;
      try {
        return checked(projectNumber(parseJSON(value)));
      } catch (error) {
        if (
          error instanceof SyntaxError ||
          error instanceof TypeError ||
          error instanceof RangeError ||
          error instanceof z.ZodError
        )
          return null;
        throw error;
      }
    },
    "Read an exactly representable JSON number; incomplete or imprecise input produces null.",
  ),
  concat: operator(
    z.tuple([z.array(string).max(limits.collection)]),
    ([items]) => boundedJoin(items, ""),
    "Concatenate literal text.",
  ),
  join: operator(
    z.tuple([z.array(string).max(limits.collection), string]),
    ([items, separator]) => boundedJoin(items, separator),
    "Join text values.",
  ),
  lower: operator(
    z.tuple([string]),
    ([value]) => value.toLowerCase(),
    "Lowercase text.",
  ),
  upper: operator(
    z.tuple([string]),
    ([value]) => value.toUpperCase(),
    "Uppercase text.",
  ),
  trim: operator(
    z.tuple([string]),
    ([value]) => value.trim(),
    "Trim surrounding whitespace.",
  ),
  contains: operator(
    z.tuple([string, string]),
    ([value, search]) => value.includes(search),
    "Case-sensitive text search.",
  ),
  length: operator(
    z.tuple([z.union([string, array])]),
    ([value]) => value.length,
    "Text or list length.",
  ),
  get: operator(
    z.tuple([z.unknown(), path]),
    ([value, keys]) => at(value, keys),
    "Read own JSON properties; missing paths produce null.",
  ),
  keys: operator(
    z.tuple([z.record(z.string(), z.unknown())]),
    ([value]) => Object.keys(value),
    "Object keys.",
  ),
  values: operator(
    z.tuple([z.record(z.string(), z.unknown())]),
    ([value]) => Object.values(value),
    "Object values.",
  ),
  range: operator(
    z.tuple([z.number().int().min(0).max(limits.collection)]),
    ([count]) => Array.from({ length: count }, (_, index) => index),
    "Bounded zero-based integer range.",
  ),
  slice: operator(
    z.tuple([array, z.number().int(), z.number().int()]),
    ([items, start, end]) => items.slice(start, end),
    "List slice.",
  ),
  append: operator(
    z.tuple([array, z.unknown()]),
    ([items, value]) => values.parse([...items, value]),
    "Append one item to a bounded list.",
  ),
  remove: operator(
    z.tuple([array, z.unknown()]),
    ([items, value]) => items.filter((item) => !equalJSON(item, value)),
    "Remove matching values.",
  ),
  unique: operator(
    z.tuple([array]),
    ([items]) =>
      items.filter(
        (item, index) =>
          items.findIndex((other) => equalJSON(item, other)) === index,
      ),
    "Unique exact JSON values within the aggregate comparison budget.",
    "quadratic",
  ),
  reverse: operator(
    z.tuple([array]),
    ([items]) => [...items].reverse(),
    "Reverse a list.",
  ),
  sort: operator(
    z.tuple([array, path, z.enum(["asc", "desc"])]),
    ([items, keys, direction]) =>
      [...items].sort((a, b) => {
        const left = at(a, keys),
          right = at(b, keys);
        const order =
          typeof left === "number" && typeof right === "number"
            ? left - right
            : String(left ?? "").localeCompare(String(right ?? ""));
        return direction === "asc" ? order : -order;
      }),
    "Stable field-based list sorting.",
  ),
  formatNumber: operator(
    z.tuple([
      finiteNumber,
      z.string().max(32),
      z.number().int().min(0).max(12),
    ]),
    ([value, locale, digits]) =>
      new Intl.NumberFormat(locale, { maximumFractionDigits: digits }).format(
        value,
      ),
    "Locale-aware display formatting.",
  ),
  percent: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([part, total]) => (total === 0 ? null : checked((part / total) * 100)),
    "Percentage of a total; zero total gives null.",
  ),
  percentageChange: operator(
    z.tuple([finiteNumber, finiteNumber]),
    ([before, after]) =>
      before === 0
        ? null
        : checked(((after - before) / Math.abs(before)) * 100),
    "Percentage change relative to the magnitude of the original value.",
  ),
  payment: operator(
    z.tuple([
      finiteNumber.min(0),
      finiteNumber.min(0),
      z.number().int().min(1).max(12000),
    ]),
    ([principal, ratePerPeriod, periods]) =>
      ratePerPeriod === 0
        ? checked(principal / periods)
        : checked(
            (principal * ratePerPeriod) / (1 - (1 + ratePerPeriod) ** -periods),
          ),
    "Equal periodic payment from principal, decimal rate per period and count; display arithmetic only.",
  ),
  convertUnit: operator(
    z.tuple([finiteNumber, unitSchema, unitSchema]),
    ([value, from, to]) => {
      if (units[from].dimension !== units[to].dimension)
        throw new InvalidPresentation("unit dimensions differ");
      return checked(
        (value * units[from].factor + units[from].offset - units[to].offset) /
          units[to].factor,
      );
    },
    "Convert compatible length, mass, temperature or time units.",
  ),
  median: operator(
    z.tuple([numbers]),
    ([items]) => percentile(items, 0.5),
    "Median with linear interpolation.",
  ),
  percentile: operator(
    z.tuple([numbers, finiteNumber.min(0).max(1)]),
    ([items, proportion]) => percentile(items, proportion),
    "Quantile with linear interpolation, using a proportion from zero to one.",
  ),
  cumulative: operator(
    z.tuple([numbers]),
    ([items]) => {
      let total = 0;
      return items.map((value) => (total = checked(total + value)));
    },
    "Cumulative sums in source order.",
  ),
  movingAverage: operator(
    z.tuple([numbers, z.number().int().min(1).max(limits.collection)]),
    ([items, width]) => {
      let total = 0;
      return items.map((value, index) => {
        total += value;
        if (index >= width) total -= items[index - width]!;
        return checked(total / Math.min(width, index + 1));
      });
    },
    "Trailing moving average, using the available prefix before a full window.",
  ),
  top: operator(
    z.tuple([numbers, z.number().int().min(0).max(limits.collection)]),
    ([items, count]) => [...items].sort((a, b) => b - a).slice(0, count),
    "Largest numeric values in descending order.",
  ),
  pivot: operator(
    z.tuple([array, path, path, path, z.enum(["sum", "count", "average"])]),
    ([items, rowPath, columnPath, valuePath, aggregation]) => {
      const rows = new Map<
        string,
        Map<string, { count: number; sum: number }>
      >();
      for (const item of items) {
        const row = at(item, rowPath),
          column = at(item, columnPath);
        if (typeof row !== "string" || typeof column !== "string")
          throw new InvalidPresentation("pivot keys must be strings");
        const cells =
          rows.get(row) ?? new Map<string, { count: number; sum: number }>();
        const cell = cells.get(column) ?? { count: 0, sum: 0 };
        cell.count += 1;
        if (aggregation !== "count")
          cell.sum = checked(
            cell.sum + finiteNumber.parse(at(item, valuePath)),
          );
        cells.set(column, cell);
        rows.set(row, cells);
      }
      return [...rows].map(([key, cells]) => ({
        key,
        values: Object.fromEntries(
          [...cells].map(([column, cell]) => [
            column,
            aggregation === "count"
              ? cell.count
              : aggregation === "average"
                ? checked(cell.sum / cell.count)
                : cell.sum,
          ]),
        ),
      }));
    },
    "Pivot JSON records into rows {key, values:{column:number}}, aggregating sum, count or average.",
  ),
  dateAdd: operator(
    z.tuple([
      date,
      z.enum(["days", "weeks", "months", "years"]),
      z.number().int().min(-100000).max(100000),
    ]),
    ([source, unit, amount]) =>
      Temporal.PlainDate.from(source)
        .add({ [unit]: amount })
        .toString(),
    "Add a calendar duration to an ISO date, constraining month-end overflow.",
  ),
  dateDifference: operator(
    z.tuple([date, date]),
    ([from, to]) =>
      Temporal.PlainDate.from(from).until(Temporal.PlainDate.from(to), {
        largestUnit: "days",
      }).days,
    "Signed calendar days from the first date to the second.",
  ),
  dateParts: operator(
    z.tuple([date]),
    ([source]) => {
      const value = Temporal.PlainDate.from(source);
      return {
        year: value.year,
        month: value.month,
        day: value.day,
        dayOfWeek: value.dayOfWeek,
        daysInMonth: value.daysInMonth,
      };
    },
    "Calendar year, month, day, weekday (Monday=1) and days in month.",
  ),
  formatDate: operator(
    z.tuple([date, z.string().max(32)]),
    ([source, locale]) =>
      Temporal.PlainDate.from(source).toLocaleString(locale, {
        dateStyle: "medium",
      }),
    "Locale-aware calendar date formatting without time-zone conversion.",
  ),
} as const;

export type Operator = keyof typeof operators;
export function functionCatalog() {
  return Object.entries(operators).map(([name, definition]) => ({
    name,
    description: definition.description,
    parameters: z.toJSONSchema(definition.parameters),
  }));
}
export type Expression =
  | { literal: JSONValue }
  | { state: string; path?: string[] }
  | { operation: string; path?: string[] }
  | { item: string; path?: string[] }
  | { call: Operator; args: Expression[] }
  | { object: Record<string, Expression> }
  | { array: Expression[] }
  | { map: { items: Expression; as: string; limit: number; value: Expression } }
  | {
      filter: {
        items: Expression;
        as: string;
        limit: number;
        where: Expression;
      };
    }
  | {
      groupBy: {
        items: Expression;
        as: string;
        limit: number;
        key: Expression;
      };
    };

export const expressionSchema: z.ZodType<Expression> = z.lazy(() =>
  z.union([
    z.strictObject({ literal: jsonValueSchema }),
    z.strictObject({ state: identifier, path: path.optional() }),
    z.strictObject({ operation: identifier, path: path.optional() }),
    z.strictObject({ item: identifier, path: path.optional() }),
    z.strictObject({
      call: z.enum(Object.keys(operators) as [Operator, ...Operator[]]),
      args: z.array(expressionSchema).max(32),
    }),
    z.strictObject({ object: z.record(z.string().max(256), expressionSchema) }),
    z.strictObject({ array: z.array(expressionSchema).max(limits.collection) }),
    z.strictObject({
      map: z.strictObject({
        items: expressionSchema,
        as: identifier,
        limit: z.number().int().min(1).max(limits.collection),
        value: expressionSchema,
      }),
    }),
    z.strictObject({
      filter: z.strictObject({
        items: expressionSchema,
        as: identifier,
        limit: z.number().int().min(1).max(limits.collection),
        where: expressionSchema,
      }),
    }),
    z.strictObject({
      groupBy: z.strictObject({
        items: expressionSchema,
        as: identifier,
        limit: z.number().int().min(1).max(limits.collection),
        key: expressionSchema,
      }),
    }),
  ]),
);

export function isExpression(value: unknown): value is Expression {
  return (
    value !== null &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    [
      "literal",
      "state",
      "operation",
      "item",
      "call",
      "object",
      "array",
      "map",
      "filter",
      "groupBy",
    ].some((key) => Object.hasOwn(value, key))
  );
}

export function expressionCost(expression: Expression): number {
  if ("array" in expression)
    return (
      1 +
      expression.array.reduce((cost, value) => cost + expressionCost(value), 0)
    );
  if ("object" in expression)
    return (
      1 +
      Object.values(expression.object).reduce(
        (cost, value) => cost + expressionCost(value),
        0,
      )
    );
  if ("call" in expression) {
    const definition = operators[expression.call];
    const parameters = z.toJSONSchema(definition.parameters);
    const collections = (parameters.prefixItems ?? []).map(
      (parameter, index) => {
        if (
          typeof parameter !== "object" ||
          parameter === null ||
          (parameter.type !== "array" && parameter.type !== "object")
        )
          return 0;
        const argument = expression.args[index];
        if (!argument) return 0;
        if (
          "literal" in argument &&
          argument.literal !== null &&
          typeof argument.literal === "object"
        )
          return Object.keys(argument.literal).length;
        if ("array" in argument) return argument.array.length;
        if ("object" in argument) return Object.keys(argument.object).length;
        if ("map" in argument) return argument.map.limit;
        if ("filter" in argument) return argument.filter.limit;
        return limits.collection;
      },
    );
    const work = Math.max(1, ...collections);
    return (
      (definition.complexity === "quadratic" ? work * work : work) +
      expression.args.reduce((cost, value) => cost + expressionCost(value), 0)
    );
  }
  if (
    "map" in expression ||
    "filter" in expression ||
    "groupBy" in expression
  ) {
    const iteration =
      "map" in expression
        ? expression.map
        : "filter" in expression
          ? expression.filter
          : expression.groupBy;
    const body =
      "value" in iteration
        ? iteration.value
        : "where" in iteration
          ? iteration.where
          : iteration.key;
    return (
      1 +
      expressionCost(iteration.items) +
      iteration.limit * (1 + expressionCost(body))
    );
  }
  return 1;
}

export function stateKey(id: string): string {
  return "$input_" + id;
}
export function operationKey(id: string): string {
  return "$operation_" + id;
}

const functionName = (name: string) =>
  "GenUI" + name[0]!.toUpperCase() + name.slice(1);
const call = (name: string, args: ASTNode[]): ASTNode => ({
  k: "Comp",
  name,
  args,
  fn: true,
});
const literal = (value: unknown): ASTNode =>
  call("GenUIJSON", [{ k: "Str", v: stringifyJSON(value) }]);

export const expressionFunctions = [
  ...Object.entries(operators).map(([name, definition]) =>
    defineFunction({
      name: functionName(name),
      description: definition.description,
      params: z.object({ args: z.array(z.unknown()).max(32) }),
      fn: ({ args }: { args: unknown[] }) => {
        assertValueBudget(args);
        const input = definition.parameters.parse(args);
        const result = (definition.run as (args: unknown[]) => unknown)(input);
        assertValueBudget(result);
        return result;
      },
    }),
  ),
  defineFunction({
    name: "GenUIInitial",
    description: "Read the declared default before the sole store initializes.",
    params: z.object({ args: z.tuple([z.unknown(), z.unknown()]) }),
    fn: ({ args }) => (args[0] === undefined ? args[1] : args[0]),
  }),
  defineFunction({
    name: "GenUIValue",
    description: "Enforce aggregate value capacity.",
    params: z.object({ value: z.unknown() }),
    fn: ({ value }) => {
      assertValueBudget(value);
      return value;
    },
  }),
  defineFunction({
    name: "GenUIBound",
    description: "Enforce declared iteration capacity.",
    params: z.object({
      items: z.array(z.unknown()).max(limits.collection),
      limit: z.number().int().min(1).max(limits.collection),
    }),
    fn: ({ items, limit }) => {
      if (items.length > limit)
        throw new InvalidPresentation("iteration exceeds its declared limit");
      return items;
    },
  }),
  defineFunction({
    name: "GenUIJSON",
    description: "Decode an owner-encoded literal.",
    params: z.object({ source: z.string() }),
    fn: ({ source }) => parseJSON(source),
  }),
  defineFunction({
    name: "GenUIRecord",
    description: "Build an owner-declared record.",
    params: z.object({
      keys: z.array(z.string()),
      values: z.array(z.unknown()),
    }),
    fn: ({ keys, values }) =>
      Object.fromEntries(
        keys.map((key, index) => [key, values[index] ?? null]),
      ),
  }),
  defineFunction({
    name: "GenUIFilterPairs",
    description: "Select mapped items.",
    params: z.object({
      pairs: z
        .array(z.tuple([z.unknown(), z.boolean()]))
        .max(limits.collection),
    }),
    fn: ({ pairs }) => pairs.filter(([, yes]) => yes).map(([item]) => item),
  }),
  defineFunction({
    name: "GenUIGroupPairs",
    description: "Group mapped items.",
    params: z.object({
      pairs: z
        .array(
          z.tuple([
            z.unknown(),
            z.union([z.string(), finiteNumber, z.boolean(), z.null()]),
          ]),
        )
        .max(limits.collection),
    }),
    fn: ({ pairs }) => {
      const groups = new Map<string, unknown[]>();
      for (const [item, key] of pairs) {
        const name = String(key);
        const group = groups.get(name) ?? [];
        group.push(item);
        groups.set(name, group);
      }
      return Object.fromEntries(groups);
    },
  }),
];

export function compileExpression(
  expression: Expression,
  initial: Record<string, unknown> = {},
): ASTNode {
  const compile = (value: Expression) => compileExpression(value, initial);
  if ("literal" in expression) return literal(expression.literal);
  if ("array" in expression)
    return { k: "Arr", els: expression.array.map(compile) };
  if ("object" in expression)
    return call("GenUIRecord", [
      literal(Object.keys(expression.object)),
      { k: "Arr", els: Object.values(expression.object).map(compile) },
    ]);
  if ("call" in expression)
    return call(functionName(expression.call), [
      { k: "Arr", els: expression.args.map(compile) },
    ]);
  if ("map" in expression)
    return each(
      expression.map.items,
      expression.map.as,
      compile(expression.map.value),
      expression.map.limit,
      initial,
    );
  if ("filter" in expression)
    return call("GenUIFilterPairs", [
      each(
        expression.filter.items,
        expression.filter.as,
        {
          k: "Arr",
          els: [
            { k: "Ref", n: expression.filter.as },
            compile(expression.filter.where),
          ],
        },
        expression.filter.limit,
        initial,
      ),
    ]);
  if ("groupBy" in expression)
    return call("GenUIGroupPairs", [
      each(
        expression.groupBy.items,
        expression.groupBy.as,
        {
          k: "Arr",
          els: [
            { k: "Ref", n: expression.groupBy.as },
            compile(expression.groupBy.key),
          ],
        },
        expression.groupBy.limit,
        initial,
      ),
    ]);
  let reference: ASTNode;
  if ("state" in expression || "operation" in expression) {
    const key =
      "state" in expression
        ? stateKey(expression.state)
        : operationKey(expression.operation);
    reference = { k: "StateRef", n: key };
    if (Object.hasOwn(initial, key))
      reference = call("GenUIInitial", [
        { k: "Arr", els: [reference, literal(initial[key])] },
      ]);
  } else reference = { k: "Ref", n: expression.item };
  return expression.path?.length
    ? call(functionName("get"), [
        { k: "Arr", els: [reference, literal(expression.path)] },
      ])
    : reference;
}

export function each(
  items: Expression,
  name: string,
  template: ASTNode,
  limit: number,
  initial: Record<string, unknown> = {},
): ASTNode {
  const bounded = call("GenUIBound", [
    compileExpression(items, initial),
    { k: "Num", v: limit },
  ]);
  return call("GenUIValue", [
    call("Each", [bounded, { k: "Str", v: name }, template]),
  ]);
}

export function expressionReferences(
  expression: Expression,
  visit: (kind: "state" | "operation" | "item", id: string) => void,
  locals = new Set<string>(),
): void {
  if ("state" in expression) visit("state", expression.state);
  else if ("operation" in expression) visit("operation", expression.operation);
  else if ("item" in expression) {
    if (!locals.has(expression.item)) visit("item", expression.item);
  } else if ("call" in expression) {
    const parameters = z.toJSONSchema(operators[expression.call].parameters);
    if (
      expression.args.length < (parameters.minItems ?? 0) ||
      expression.args.length > (parameters.maxItems ?? Infinity)
    )
      throw new InvalidPresentation(
        "function argument count differs from its declared signature: " +
          expression.call,
      );
    for (const arg of expression.args) expressionReferences(arg, visit, locals);
  } else if ("array" in expression)
    for (const arg of expression.array)
      expressionReferences(arg, visit, locals);
  else if ("object" in expression)
    for (const arg of Object.values(expression.object))
      expressionReferences(arg, visit, locals);
  else if (
    "map" in expression ||
    "filter" in expression ||
    "groupBy" in expression
  ) {
    const value =
      "map" in expression
        ? expression.map
        : "filter" in expression
          ? expression.filter
          : expression.groupBy;
    expressionReferences(value.items, visit, locals);
    const scope = new Set([...locals, value.as]);
    expressionReferences(
      "value" in value
        ? value.value
        : "where" in value
          ? value.where
          : value.key,
      visit,
      scope,
    );
  }
}

export function runtimeFunctions() {
  return Object.fromEntries(
    expressionFunctions.map((fn) => {
      const schema = z.toJSONSchema(fn.params);
      if (schema.type !== "object")
        throw new Error("genui: pure functions require named parameters");
      return [fn.name, { params: { ...schema, type: "object" }, fn: fn.fn }];
    }),
  );
}

export function evaluateExpression(
  expression: Expression,
  readState: (id: string) => unknown,
  readOperation: (id: string) => unknown,
  scope: Record<string, unknown> = {},
): unknown {
  const errors: string[] = [];
  const context: EvaluationContext = {
    getState: (key) =>
      key.startsWith("$input_")
        ? readState(key.slice(7))
        : readOperation(key.slice(11)),
    resolveRef: (name) => (Object.hasOwn(scope, name) ? scope[name] : null),
    functions: runtimeFunctions(),
    reportError: (message) => errors.push(message),
  };
  if (expressionCost(expression) > limits.evaluationSteps)
    throw new InvalidPresentation("expression exceeds evaluation budget");
  const value = evaluate(compileExpression(expression), context);
  assertValueBudget(value);
  if (errors.length)
    throw new InvalidPresentation("expression failed: " + errors.join("; "));
  return value;
}
