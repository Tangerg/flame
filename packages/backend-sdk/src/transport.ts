import { TextDecoder } from "node:util";
import type { Readable, Writable } from "node:stream";
import type { Transport } from "@modelcontextprotocol/sdk/shared/transport.js";
import {
  JSONRPCMessageSchema,
  type JSONRPCMessage,
} from "@modelcontextprotocol/sdk/types.js";
import { parseJSON, stringifyJSON } from "@flame/runtime-client";

export class StdioTransport implements Transport {
  onclose?: Transport["onclose"];
  onerror?: Transport["onerror"];
  onmessage?: Transport["onmessage"];
  readonly #input: Readable;
  readonly #output: Writable;
  #started = false;
  #closed = false;
  #buffer = Buffer.alloc(0);
  #writing: Promise<void> = Promise.resolve();

  constructor(input: Readable, output: Writable) {
    this.#input = input;
    this.#output = output;
  }
  readonly #read = (bytes: Buffer): void => {
    try {
      this.#buffer = Buffer.concat([this.#buffer, bytes]);
      for (
        let end = this.#buffer.indexOf(10);
        end >= 0;
        end = this.#buffer.indexOf(10)
      ) {
        const source = new TextDecoder("utf-8", { fatal: true }).decode(
          this.#buffer.subarray(0, end),
        );
        this.#buffer = this.#buffer.subarray(end + 1);
        const message = JSONRPCMessageSchema.parse(parseJSON(source));
        this.onmessage?.(message);
      }
    } catch (cause) {
      this.onerror?.(new Error("plugin: invalid MCP frame", { cause }));
      void this.close();
    }
  };
  readonly #end = (): void => {
    if (this.#buffer.length !== 0)
      this.onerror?.(new Error("plugin: incomplete MCP frame"));
    void this.close();
  };
  readonly #error = (cause: Error): void => {
    this.onerror?.(new Error("plugin: stdio failed", { cause }));
    void this.close();
  };
  async start(): Promise<void> {
    if (this.#started || this.#closed)
      throw new Error("plugin: transport already started or closed");
    this.#started = true;
    this.#input.on("data", this.#read);
    this.#input.on("end", this.#end);
    this.#input.on("error", this.#error);
    this.#output.on("error", this.#error);
  }
  async send(message: JSONRPCMessage): Promise<void> {
    if (this.#closed) throw new Error("plugin: transport is closed");
    const frame = stringifyJSON(message) + "\n";
    const write = this.#writing.then(async () => {
      if (this.#closed) throw new Error("plugin: transport is closed");
      await new Promise<void>((resolve, reject) =>
        this.#output.write(frame, (error) =>
          error ? reject(error) : resolve(),
        ),
      );
    });
    this.#writing = write.catch(() => {});
    return write;
  }
  async close(): Promise<void> {
    if (this.#closed) return;
    this.#closed = true;
    this.#input.off("data", this.#read);
    this.#input.off("end", this.#end);
    this.#input.off("error", this.#error);
    this.#output.off("error", this.#error);
    this.#input.pause();
    this.onclose?.();
    await this.#writing;
  }
}
