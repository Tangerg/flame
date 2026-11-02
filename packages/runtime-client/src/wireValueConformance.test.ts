import { readFile } from "node:fs/promises";
import { expect, it } from "vitest";
import { validateWire, type WireTypeName } from "@flame/runtime-client/validation";

it("enforces the same value boundaries as the Go protocol decoder", async () => {
  const source = new URL(
    "../../../runtime/internal/arch/testdata/wire-values.json",
    import.meta.url,
  );
  const vectors = JSON.parse(await readFile(source, "utf8")) as {
    shape: WireTypeName;
    value: unknown;
    valid: boolean;
  }[];
  for (const vector of vectors)
    expect(validateWire(vector.shape, vector.value).length === 0, JSON.stringify(vector)).toBe(
      vector.valid,
    );
});
