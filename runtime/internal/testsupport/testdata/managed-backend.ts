import { readFileSync } from "node:fs";
import {
  Backend,
  Capability,
  parseJSON,
  type CapabilityDescriptor,
} from "@flame/backend-sdk";

console.info("managed conformance initialized");
const contract = new Capability(
  parseJSON(
    readFileSync(
      new URL("./io.github.tangerg.flame/contract.json", import.meta.url),
      "utf8",
    ),
  ) as CapabilityDescriptor,
);
const backend = new Backend({ name: "managed-conformance", version: "1" });
backend.implement(
  contract.method("fixture.usage"),
  (value) => value as { sinceDays: number },
  async (context, input) => {
    const host = await context.runtime();
    return parseJSON(
      await host.queryJSON("usage.summary", input, { signal: context.signal }),
    );
  },
);
backend.bindCapability("probe", contract);
backend.tool(
  {
    name: "usage_probe",
    description: "Read the calling Runtime accounting evidence.",
    inputSchema: { type: "object", additionalProperties: false },
  },
  async (context) => {
    if (
      context.invocation?.origin !== "model" ||
      context.invocation.target.scope !== "installation" ||
      !context.invocation.parent
    )
      throw new Error("fixture: original tool invocation is required");
    const host = await context.runtime();
    const summary = await host.queryJSON(
      "usage.summary",
      {},
      { signal: context.signal },
    );
    return {
      content: [
        {
          type: "text",
          text:
            "captured-tool-owner:" +
            context.invocation.id +
            ":" +
            context.invocation.target.reference +
            ":" +
            summary,
        },
      ],
    };
  },
);
export default backend;
