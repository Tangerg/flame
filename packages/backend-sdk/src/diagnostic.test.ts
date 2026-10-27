import { expect, it } from "vitest";
import { failureDiagnostic } from "./diagnostic";

function located(error: Error, source: string): Error {
  const heading = error.message
    ? `${error.name}: ${error.message}`
    : error.name;
  error.stack = `${heading}\n    at operation (${source}:12:3)`;
  return error;
}

it("preserves failure phase, categories, codes, sources and causal relationships", () => {
  const cause = located(new TypeError("private input"), "/author/input.ts");
  const child = located(
    Object.assign(new Error("credential=private"), {
      code: "ERR_TEST_FAILURE",
    }),
    "file:///author/handler.ts?credential=private",
  );
  const failure = located(
    new AggregateError([child], "private aggregate", { cause }),
    "/author/service.ts",
  );
  const diagnostic = failureDiagnostic("service", failure);
  expect(diagnostic).toContain("plugin: backend service failed");
  expect(diagnostic).toContain("error: AggregateError");
  expect(diagnostic).toContain("caused by: TypeError");
  expect(diagnostic).toContain("aggregate[0]: Error [ERR_TEST_FAILURE]");
  expect(diagnostic).toContain("/author/input.ts:12:3");
  expect(diagnostic).toContain("file:///author/handler.ts:12:3");
  expect(diagnostic).not.toContain("private");
  expect(diagnostic).not.toContain("credential");
});

it("never dumps exception messages, arbitrary payloads or inspect hooks", () => {
  const secret = "sensitive-token-and-user-input";
  const failure = located(
    new Error(`${secret}\n    at /do-not-print/${secret}:2:3`, {
      cause: { credential: secret, body: secret },
    }),
    "/author/handler.ts",
  );
  Object.assign(failure, {
    request: { authorization: secret },
    [Symbol.for("nodejs.util.inspect.custom")]: () => {
      throw new Error("inspection must not run");
    },
  });
  const diagnostic = failureDiagnostic("startup", failure);
  expect(diagnostic).toContain("/author/handler.ts:12:3");
  expect(diagnostic).toContain("caused by: non-error thrown value");
  expect(diagnostic).not.toContain(secret);
  expect(diagnostic).not.toContain("do-not-print");
  expect(diagnostic).not.toContain("authorization");
});

it("bounds cyclic and aggregate graphs without changing the original failure", () => {
  const failure = new Error("private cause");
  Object.defineProperty(failure, "cause", { value: failure });
  expect(failureDiagnostic("cleanup", failure)).toContain("[already reported]");
  expect(failure.cause).toBe(failure);
  const aggregate = new AggregateError(
    Array.from({ length: 1000 }, () =>
      located(new Error("private"), "/author/" + "x".repeat(4000)),
    ),
    "private aggregate",
  );
  const diagnostic = failureDiagnostic("cleanup", aggregate);
  expect(Buffer.byteLength(diagnostic)).toBeLessThanOrEqual(8 << 10);
  expect(diagnostic).toContain("[diagnostic truncated]");
});

it("does not invoke cause or aggregate accessors", () => {
  const failure = located(new Error("private"), "/author/handler.ts");
  Object.defineProperty(failure, "cause", {
    get() {
      throw new Error("cause accessor must not run");
    },
  });
  Object.defineProperty(failure, "errors", {
    get() {
      throw new Error("aggregate accessor must not run");
    },
  });
  expect(failureDiagnostic("startup", failure)).toContain("/author/handler.ts");
});

it("reports unreadable error metadata without throwing another exception", () => {
  const failure = new Error();
  Object.defineProperty(failure, "name", {
    get() {
      throw new Error("private value");
    },
  });
  const diagnostic = failureDiagnostic("startup", failure);
  expect(diagnostic).toContain("error metadata could not be read");
  expect(diagnostic).not.toContain("private value");
});

it.each([undefined, null, "secret payload", { token: "secret payload" }])(
  "does not stringify non-Error thrown values",
  (value) => {
    const diagnostic = failureDiagnostic("unhandled rejection", value);
    expect(diagnostic).toContain("non-error thrown value");
    expect(diagnostic).not.toContain("secret payload");
  },
);

it("retains Unicode source paths but never emits terminal control characters", () => {
  const path = "/author/工作区/😀.ts";
  expect(
    failureDiagnostic("startup", located(new Error("private"), path)),
  ).toContain(`${path}:12:3`);
  const diagnostic = failureDiagnostic(
    "startup",
    located(new Error("private"), "/author/\u001b[31mhandler.ts"),
  );
  expect(diagnostic).toContain("[unprintable source]");
  expect(diagnostic).not.toContain("\u001b");
});
