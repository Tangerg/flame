// TypeScript types disappear at the transport boundary. Generated checks consume
// Runtime schemas; this module owns only the JSON Schema keyword semantics.
// Request objects are closed by the Runtime contract; result objects may carry
// fields unknown to this projection, and format/contentEncoding stay annotations.

import {
  ExactNumber,
  compareJSONNumbers,
  equalJSON,
  isJSONInteger,
  isJSONNumber,
  projectNumber,
  type JSONNumber,
} from "@qimu/sdk/json";

export interface WireViolation {
  path: string;
  detail: string;
}

// Union diagnostics need all violations to identify the nearest admissible branch.
export type WireCheck = ((
  value: unknown,
  path: string,
  out: WireViolation[],
  boundary?: "request",
) => void) & { project?: (value: unknown) => unknown };

function withProjection(check: WireCheck, project: (value: unknown) => unknown): WireCheck {
  return Object.assign(check, { project });
}

// Only schema-declared numeric fields promise JS numbers. Opaque JSON retains
// its exact representation, including schema bounds and feature-owned values.
export function projectWireValue(check: WireCheck, value: unknown): unknown {
  return check.project ? check.project(value) : value;
}

// An allOf refinement can repeat its shared shape's bound; reporting that miss
// twice would present one invalid field as two independent failures.
export function distinctViolations(violations: readonly WireViolation[]): WireViolation[] {
  if (violations.length < 2) return [...violations];
  const seen = new Set<string>();
  const out: WireViolation[] = [];
  for (const violation of violations) {
    const key = `${violation.path}\u0000${violation.detail}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(violation);
  }
  return out;
}

export function object(
  properties: Record<string, WireCheck>,
  required: readonly string[],
): WireCheck {
  const stated = fields(properties, required);
  return withProjection(
    (value, path, out, boundary) => {
      if (!isObject(value)) {
        out.push({ path, detail: "expected an object" });
        return;
      }
      if (boundary === "request") {
        for (const name of Object.keys(value)) {
          if (!Object.hasOwn(properties, name))
            out.push({ path: `${path}.${name}`, detail: "must not be present" });
        }
      }
      stated(value, path, out, boundary);
    },
    (value) => projectWireValue(stated, value),
  );
}

// Union refinements inherit the enclosing type check. Reasserting it here would
// report the same wrong type once for every branch.
export function fields(
  properties: Record<string, WireCheck>,
  required: readonly string[],
): WireCheck {
  return withProjection(
    (value, path, out, boundary) => {
      if (!isObject(value)) return;
      for (const name of required) {
        if (value[name] === undefined) out.push({ path: `${path}.${name}`, detail: "is required" });
      }
      for (const [name, check] of Object.entries(properties)) {
        if (value[name] !== undefined) check(value[name], `${path}.${name}`, out, boundary);
      }
    },
    (value) => {
      if (!isObject(value)) return value;
      const result = { ...value };
      for (const [name, check] of Object.entries(properties))
        if (value[name] !== undefined) result[name] = projectWireValue(check, value[name]);
      return result;
    },
  );
}

export function absent(): WireCheck {
  return (_value, path, out) => out.push({ path, detail: "must not be present here" });
}

export function text(): WireCheck {
  return (value, path, out) => {
    if (typeof value !== "string") out.push({ path, detail: "expected a string" });
  };
}

export function integer(): WireCheck {
  return withProjection((value, path, out) => {
    if (!isJSONInteger(value)) {
      out.push({ path, detail: "expected an integer" });
    }
  }, projectNumber);
}

export function numeric(): WireCheck {
  return withProjection((value, path, out) => {
    if (!isJSONNumber(value)) {
      out.push({ path, detail: "expected a number" });
    }
  }, projectNumber);
}

// A schema may refine an inherited string without restating its type.
export function minLength(least: number): WireCheck {
  return (value, path, out) => {
    if (typeof value === "string" && Array.from(value).length < least) {
      out.push({ path, detail: `expected at least ${least} character(s)` });
    }
  };
}

export function maxLength(most: number): WireCheck {
  return (value, path, out) => {
    if (typeof value === "string" && Array.from(value).length > most) {
      out.push({ path, detail: `expected at most ${most} character(s)` });
    }
  };
}

export function pattern(expression: string): WireCheck {
  // JSON Schema patterns use ECMAScript's Unicode-aware regular-expression
  // dialect. The explicit flag makes property escapes such as \p{C} mean the
  // same thing in every generated client runtime.
  const compiled = new RegExp(expression, "u");
  return (value, path, out) => {
    if (typeof value === "string" && !compiled.test(value)) {
      out.push({ path, detail: `expected to match ${expression}` });
    }
  };
}

export function minimum(least: number | string): WireCheck {
  const bound: JSONNumber = typeof least === "string" ? new ExactNumber(least) : least;
  return (value, path, out) => {
    if (isJSONNumber(value) && compareJSONNumbers(value, bound) < 0) {
      out.push({ path, detail: `expected at least ${least}` });
    }
  };
}

export function exclusiveMinimum(boundary: number | string): WireCheck {
  const bound: JSONNumber = typeof boundary === "string" ? new ExactNumber(boundary) : boundary;
  return (value, path, out) => {
    if (isJSONNumber(value) && compareJSONNumbers(value, bound) <= 0) {
      out.push({ path, detail: `expected greater than ${boundary}` });
    }
  };
}

export function maximum(most: number | string): WireCheck {
  const bound: JSONNumber = typeof most === "string" ? new ExactNumber(most) : most;
  return (value, path, out) => {
    if (isJSONNumber(value) && compareJSONNumbers(value, bound) > 0) {
      out.push({ path, detail: `expected at most ${most}` });
    }
  };
}

export function minItems(least: number): WireCheck {
  return (value, path, out) => {
    if (Array.isArray(value) && value.length < least) {
      out.push({ path, detail: `expected at least ${least} item(s)` });
    }
  };
}

export function maxItems(most: number): WireCheck {
  return (value, path, out) => {
    if (Array.isArray(value) && value.length > most) {
      out.push({ path, detail: `expected at most ${most} item(s)` });
    }
  };
}

export function minProperties(least: number): WireCheck {
  return (value, path, out) => {
    if (isObject(value) && Object.keys(value).length < least) {
      const noun = least === 1 ? "property" : "properties";
      out.push({ path, detail: `expected at least ${least} ${noun}` });
    }
  };
}

// JSON value equality ignores object order and number spelling, unlike the
// exact byte identity retained for command recovery.
export function uniqueItems(): WireCheck {
  return (value, path, out) => {
    if (!Array.isArray(value)) return;
    const seen: unknown[] = [];
    for (const element of value) {
      if (seen.some((previous) => equalJSON(element, previous))) {
        out.push({ path, detail: "expected no repeated items" });
        return;
      }
      seen.push(element);
    }
  };
}

export function flag(): WireCheck {
  return (value, path, out) => {
    if (typeof value !== "boolean") out.push({ path, detail: "expected a boolean" });
  };
}

export function literal(expected: string): WireCheck {
  return (value, path, out) => {
    if (value !== expected) out.push({ path, detail: `expected ${JSON.stringify(expected)}` });
  };
}

export function enumOf(values: readonly string[]): WireCheck {
  return (value, path, out) => {
    if (typeof value !== "string" || !values.includes(value)) {
      out.push({
        path,
        detail: `expected one of ${values.map((v) => JSON.stringify(v)).join(", ")}`,
      });
    }
  };
}

export function anything(): WireCheck {
  return () => {};
}

export function nullable(value: WireCheck): WireCheck {
  return withProjection(
    (candidate, path, out, boundary) => {
      if (candidate !== null) value(candidate, path, out, boundary);
    },
    (candidate) => (candidate === null ? null : projectWireValue(value, candidate)),
  );
}

export function array(items: WireCheck): WireCheck {
  return withProjection(
    (value, path, out, boundary) => {
      if (!Array.isArray(value)) {
        out.push({ path, detail: "expected an array" });
        return;
      }
      value.forEach((element, index) => items(element, `${path}[${index}]`, out, boundary));
    },
    (value) => (Array.isArray(value) ? value.map((item) => projectWireValue(items, item)) : value),
  );
}

export function record(values: WireCheck): WireCheck {
  return withProjection(
    (value, path, out, boundary) => {
      if (!isObject(value)) {
        out.push({ path, detail: "expected an object" });
        return;
      }
      for (const [key, member] of Object.entries(value)) {
        values(member, `${path}[${JSON.stringify(key)}]`, out, boundary);
      }
    },
    (value) =>
      isObject(value)
        ? Object.fromEntries(
            Object.entries(value).map(([key, member]) => [key, projectWireValue(values, member)]),
          )
        : value,
  );
}

export function propertyNames(check: WireCheck): WireCheck {
  return (value, path, out, boundary) => {
    if (!isObject(value)) return;
    for (const key of Object.keys(value))
      check(key, `${path}[${JSON.stringify(key)}]`, out, boundary);
  };
}

// Recursive definitions cannot capture a check before the referenced type exists.
export function ref(resolve: () => WireCheck): WireCheck {
  return withProjection(
    (value, path, out, boundary) => resolve()(value, path, out, boundary),
    (value) => projectWireValue(resolve(), value),
  );
}

// A nearest-branch diagnostic explains rejected content without changing the
// exact requirement that only one branch may admit it.
export function oneOf(branches: readonly WireCheck[]): WireCheck {
  return withProjection(
    (value, path, out, boundary) => {
      let matched = 0;
      let nearest: WireViolation[] | undefined;
      let nearestMissesDiscriminator = true;
      for (const branch of branches) {
        const missed: WireViolation[] = [];
        branch(value, path, missed, boundary);
        if (missed.length === 0) {
          matched++;
          continue;
        }
        // Every first-party union is discriminated by `type`. Once that tag matches,
        // report the defects in that variant even when an earlier wrong-tag branch
        // happens to have fewer violations. Otherwise `{type:"resync"}` misleadingly
        // reports `expected "skills.changed"` instead of `topics is required`.
        const missesDiscriminator = missed.some((violation) => violation.path === `${path}.type`);
        if (
          nearest === undefined ||
          (nearestMissesDiscriminator && !missesDiscriminator) ||
          (nearestMissesDiscriminator === missesDiscriminator && missed.length < nearest.length)
        ) {
          nearest = missed;
          nearestMissesDiscriminator = missesDiscriminator;
        }
      }
      if (matched === 1) return;
      if (matched > 1) {
        out.push({
          path,
          detail: `matches ${matched} variants, and exactly one may apply`,
        });
        return;
      }
      out.push({ path, detail: "matches no permitted variant" });
      if (nearest !== undefined) out.push(...nearest);
    },
    (value) => projectAlternative(branches, value),
  );
}

export function anyOf(branches: readonly WireCheck[]): WireCheck {
  return withProjection(
    (value, path, out, boundary) => {
      let nearest: WireViolation[] | undefined;
      for (const branch of branches) {
        const missed: WireViolation[] = [];
        branch(value, path, missed, boundary);
        if (missed.length === 0) return;
        if (nearest === undefined || missed.length < nearest.length) nearest = missed;
      }
      out.push({ path, detail: "matches no permitted alternative" });
      if (nearest !== undefined) out.push(...nearest);
    },
    (value) => projectAlternative(branches, value),
  );
}

function projectAlternative(branches: readonly WireCheck[], value: unknown): unknown {
  for (const branch of branches) {
    const violations: WireViolation[] = [];
    branch(value, "$", violations);
    if (violations.length === 0) return projectWireValue(branch, value);
  }
  throw new TypeError("wire projection requires an admitted alternative");
}

export function allOf(members: readonly WireCheck[]): WireCheck {
  return withProjection(
    (value, path, out, boundary) => {
      for (const member of members) member(value, path, out, boundary);
    },
    (value) => members.reduce((result, member) => projectWireValue(member, result), value),
  );
}

export function ifThen(condition: WireCheck, consequence: WireCheck): WireCheck {
  return withProjection(
    (value, path, out, boundary) => {
      const unmet: WireViolation[] = [];
      condition(value, path, unmet, boundary);
      if (unmet.length === 0) consequence(value, path, out, boundary);
    },
    (value) => {
      const unmet: WireViolation[] = [];
      condition(value, "$", unmet);
      return unmet.length === 0 ? projectWireValue(consequence, value) : value;
    },
  );
}

function isObject(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === "object" &&
    value !== null &&
    !Array.isArray(value) &&
    !(value instanceof ExactNumber)
  );
}
