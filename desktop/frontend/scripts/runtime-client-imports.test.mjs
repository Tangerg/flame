import { strict as assert } from "node:assert";
import { test } from "node:test";
import { importsRuntimeClient } from "./runtime-client-imports.mjs";

test("shared SDK imports retain their outbound-layer edge after package extraction", () => {
  for (const source of [
    'import {\n createFlameClient\n} from "@flame/runtime-contract/client";',
    'import type { Transport } from "@flame/runtime-contract/client/transport";',
    'const client = await import("@flame/runtime-contract/client/sdk");',
    'export { createHttpTransport } from "@flame/runtime-contract/client/transports/http";',
  ])
    assert.equal(importsRuntimeClient(source), true);
});

test("generated value contracts stay available to pure projections without importing transport", () => {
  assert.equal(
    importsRuntimeClient('import type { Item } from "@flame/runtime-contract/wire";'),
    false,
  );
  assert.equal(
    importsRuntimeClient('import type { WireParams } from "@flame/runtime-contract/methods";'),
    false,
  );
  assert.equal(
    importsRuntimeClient('// import { createFlameClient } from "@flame/runtime-contract/client";'),
    false,
  );
});
