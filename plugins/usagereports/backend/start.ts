import { runtime } from "@flame/backend-sdk";
import { readFileSync } from "node:fs";
import { isAbsolute, join } from "node:path";
import {
  Backend,
  Capability,
  parseJSON,
  type CapabilityDescriptor,
} from "@qimu/node";
import { Archive } from "./archive";

const root = process.env.PLUGIN_ROOT;
const data = process.env.PLUGIN_DATA;
if (!root || !isAbsolute(root) || !data || !isAbsolute(data))
  throw new Error(
    "usage reports: absolute plugin resource and data directories are required",
  );
const contract = new Capability(
  parseJSON(
    readFileSync(
      join(root, "io.github.tangerg.flame/contracts/capability.generated.json"),
      "utf8",
    ),
  ) as CapabilityDescriptor,
);
const backend = new Backend({ name: "flame.usagereports", version: "1.0.0" });
let archive: Archive;
backend.onSetup((context) => {
  archive = new Archive(join(data, "reports.db"), contract);
  context.defer(() => archive.close());
});
backend.implement(
  contract.method("flame.usage-reports/capture.v1"),
  (value) => value,
  async (context) =>
    archive.capture(context, async (days) => {
      const host = await runtime(context);
      return host.queryJSON(
        "usage.summary",
        { sinceDays: days },
        { signal: context.signal },
      );
    }),
);
backend.implement(
  contract.method("flame.usage-reports/list.v1"),
  (value) => value,
  () => archive.list(),
);
backend.implement(
  contract.method("flame.usage-reports/get.v1"),
  (value) => value as { id: string },
  (_context, input) => archive.read(input.id),
);
backend.implement(
  contract.method("flame.usage-reports/delete.v1"),
  (value) => value,
  (context) => archive.remove(context),
);
backend.receipts((_context, request) => archive.receipt(request));
backend.bindCapability("reports", contract);
export default backend;
