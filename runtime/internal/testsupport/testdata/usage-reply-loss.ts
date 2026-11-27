import { existsSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { parseJSON } from "@flame/backend-sdk/mcp";

const write = process.stdout.write.bind(process.stdout);
process.stdout.write = ((chunk: string | Uint8Array, ...args: unknown[]) => {
  const message = parseJSON(Buffer.from(chunk).toString("utf8")) as {
    result?: { value?: { id?: string; period?: string } };
  };
  const value = message.result?.value;
  if (typeof value?.id === "string" && /^[a-f0-9]{64}$/.test(value.id)) {
    const operation =
      value.period === "daily"
        ? "capture"
        : Object.keys(value).length === 1
          ? "remove"
          : undefined;
    if (operation) {
      const marker = join(
        process.env.PLUGIN_DATA!,
        `response-lost-${operation}`,
      );
      if (!existsSync(marker)) {
        writeFileSync(marker, "committed-before-reply");
        process.exit(0);
      }
    }
  }
  return Reflect.apply(write, process.stdout, [chunk, ...args]);
}) as typeof process.stdout.write;
const { default: backend } =
  await import("../../../../plugins/usagereports/backend/start");
export default backend;
