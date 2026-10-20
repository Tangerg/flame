import type { MethodInputs, MethodOutputs } from "./methods.generated";

type UpdateInput =
  MethodInputs["io.github.tangerg.flame.examples/reviews.update"];
type UpdateOutput =
  MethodOutputs["io.github.tangerg.flame.examples/reviews.update"];
type ListOutput =
  MethodOutputs["io.github.tangerg.flame.examples/reviews.list"];

export const update: UpdateInput = {
  id: "example-review",
  expectedRevision: 1,
  status: "resolved",
};

export function currentRevision(
  result: ListOutput,
): ListOutput["reviews"][number]["revision"] | undefined {
  return result.reviews[0]?.revision;
}

export function changedReview(
  result: UpdateOutput,
): ListOutput["reviews"][number] | undefined {
  return result.review;
}

// @ts-expect-error The Go-owned Status vocabulary is closed.
export const invalidStatus: UpdateInput["status"] = "pending";

// @ts-expect-error Every producer and consumer requires the inspected revision.
export const missingRevision: UpdateInput = {
  id: "example-review",
  status: "open",
};

// @ts-expect-error Revision conflict is a typed business outcome, not an arbitrary string.
export const unknownOutcome: UpdateOutput["type"] = "failed";
