import { TextDecoder } from "node:util";
import type { Readable, Writable } from "node:stream";
import type { Transport } from "@modelcontextprotocol/sdk/shared/transport.js";
import {
  JSONRPCMessageSchema,
  type JSONRPCMessage,
} from "@modelcontextprotocol/sdk/types.js";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";

export class StdioTransport implements Transport {
  onclose?: Transport["onclose"];
  onerror?: Transport["onerror"];
  onmessage?: Transport["onmessage"];
  readonly #input: Readable;
  readonly #output: Writable;
  readonly #maximumFrameBytes: number;
  #started = false;
  #closing?: Promise<void>;
  #buffer = Buffer.alloc(0);
  #writing: Promise<void> = Promise.resolve();

  constructor(input: Readable, output: Writable, maximumFrameBytes: number) {
    if (!Number.isSafeInteger(maximumFrameBytes) || maximumFrameBytes < 1)
      throw new TypeError("MCP frame capacity must be a positive safe integer");
    this.#maximumFrameBytes = maximumFrameBytes;
    this.#input = input;
    this.#output = output;
  }
  readonly #read = (bytes: Buffer): void => {
    try {
      let offset = 0;
      while (offset < bytes.length) {
        const end = bytes.indexOf(10, offset);
        const last = end < 0 ? bytes.length : end;
        if (this.#buffer.length + last - offset > this.#maximumFrameBytes)
          throw new Error("plugin: MCP frame capacity exceeded");
        const piece = bytes.subarray(offset, last);
        const frame = this.#buffer.length
          ? Buffer.concat([this.#buffer, piece])
          : piece;
        if (end < 0) {
          this.#buffer = Buffer.from(frame);
          break;
        }
        this.#buffer = Buffer.alloc(0);
        const source = new TextDecoder("utf-8", { fatal: true }).decode(frame);
        const message = JSONRPCMessageSchema.parse(parseJSON(source));
        this.onmessage?.(message);
        if (this.#closing) break;
        offset = end + 1;
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
    if (this.#started || this.#closing)
      throw new Error("plugin: transport already started or closed");
    this.#started = true;
    this.#input.on("data", this.#read);
    this.#input.on("end", this.#end);
    this.#input.on("error", this.#error);
    this.#output.on("error", this.#error);
  }
  async send(message: JSONRPCMessage): Promise<void> {
    if (this.#closing) throw new Error("plugin: transport is closed");
    const encoded = stringifyJSON(message);
    if (Buffer.byteLength(encoded) > this.#maximumFrameBytes)
      throw new Error("plugin: MCP frame capacity exceeded");
    const frame = encoded + "\n";
    const write = this.#writing.then(async () => {
      if (this.#closing) throw new Error("plugin: transport is closed");
      await new Promise<void>((resolve, reject) =>
        this.#output.write(frame, (error) =>
          error ? reject(error) : resolve(),
        ),
      );
    });
    this.#writing = write.catch(() => {});
    return write;
  }
  close(): Promise<void> {
    if (this.#closing) return this.#closing;
    this.#closing = this.#writing.finally(() => {
      this.#output.off("error", this.#error);
    });
    this.#input.off("data", this.#read);
    this.#input.off("end", this.#end);
    this.#input.off("error", this.#error);
    this.#input.pause();
    this.onclose?.();
    return this.#closing;
  }
}
