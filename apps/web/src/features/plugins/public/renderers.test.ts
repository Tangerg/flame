import { expect, it } from "vitest";
import { resolveRenderer } from "@/features/plugins/public/renderers";

it("keeps a chosen renderer bound to its installation through reordering and withdrawal", () => {
  const a = { id: "a", title: "Markdown" },
    b = { id: "b", title: "Markdown" };
  expect(resolveRenderer([a, b], null)).toBeUndefined();
  expect(resolveRenderer([b, a], "a")).toBe(a);
  expect(resolveRenderer([b], "a")).toBeUndefined();
  expect(resolveRenderer([b], null)).toBe(b);
});
