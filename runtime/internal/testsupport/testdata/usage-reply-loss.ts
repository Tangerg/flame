import { existsSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { parseJSON } from "@qimu/node";

const marker = join(process.env.PLUGIN_DATA!, "response-lost");
const write = process.stdout.write.bind(process.stdout);
process.stdout.write = ((chunk: string | Uint8Array, ...args: unknown[]) => {
  if (!existsSync(marker)) {
    const message = parseJSON(Buffer.from(chunk).toString("utf8")) as {
      result?: { value?: { id?: string; period?: string } };
    };
    const value = message.result?.value;
    if (
      typeof value?.id === "string" &&
      /^[a-f0-9]{64}$/.test(value.id) &&
      value.period === "daily"
    ) {
      writeFileSync(marker, "committed-before-reply");
      process.exit(0);
    }
  }
  return Reflect.apply(write, process.stdout, [chunk, ...args]);
}) as typeof process.stdout.write;
const { default: backend } =
  await import("../../../../plugins/usagereports/backend/start");
export default backend;
