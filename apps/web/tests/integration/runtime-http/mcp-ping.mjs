import { writeFileSync } from "node:fs";
import { createInterface } from "node:readline";

const lines = createInterface({ input: process.stdin, crlfDelay: Infinity });
const holdToolCallMarker = process.argv
  .find((value) => value.startsWith("--hold-tool-call="))
  ?.slice("--hold-tool-call=".length);
const reply = (id, result) => {
  process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id, result }) + "\n");
};

for await (const line of lines) {
  const message = JSON.parse(line);
  if (message.id === undefined) continue;
  if (message.method === "initialize") {
    reply(message.id, {
      protocolVersion: message.params?.protocolVersion ?? "2025-11-25",
      capabilities: { tools: {} },
      serverInfo: { name: "runtime-http-e2e-mcp", version: "1.0.0" },
    });
    continue;
  }
  if (message.method === "tools/list") {
    reply(message.id, {
      tools: [
        {
          name: "ping",
          description: "Returns pong for Runtime HTTP E2E.",
          inputSchema: { type: "object", properties: {}, additionalProperties: false },
        },
      ],
    });
    continue;
  }
  if (message.method === "tools/call") {
    if (holdToolCallMarker) {
      writeFileSync(holdToolCallMarker, "arrived\n");
      continue;
    }
    reply(message.id, { content: [{ type: "text", text: "pong" }] });
    continue;
  }
  reply(message.id, {});
}
