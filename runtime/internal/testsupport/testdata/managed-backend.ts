import { readFileSync } from "node:fs";
import { runtime } from "@flame/backend-sdk";
import {
  Backend,
  Capability,
  parseJSON,
  type CapabilityDescriptor,
} from "@qimu/node";

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
    const host = await runtime(context);
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
    const host = await runtime(context);
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
