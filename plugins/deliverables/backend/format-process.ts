import { mkdir, mkdtemp, realpath, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, isAbsolute, join } from "node:path";
import { Refusal } from "@flame/backend-sdk/mcp";
import { z } from "zod";
import { inspectionSchema } from "../contracts/content";
import {
  documentLayoutSchema,
  slideLayoutSchema,
  workbookLayoutSchema,
  type DocumentLayout,
  type SlideLayout,
  type WorkbookLayout,
} from "../contracts/layouts";
import { LIMITS } from "../contracts/limits";
import {
  createFile,
  editFile,
  inspectFile,
  exportCSV,
  blocksFile,
  layoutFile,
  sheetsFile,
  validateSuggestion,
  type Inspection,
} from "./formats";
import type { Format, OfficeFormat } from "../contracts/content";
import { confinedPlan, type PrivateDirectory } from "./confinement";
import { runProcess, UnconfirmedProcessExit } from "./process";
import { refusal } from "./refusal";

type Request =
  | { operation: "create"; format: Exclude<Format, "pdf">; contentJSON: string }
  | { operation: "inspect"; format: Format; base64: string }
  | { operation: "csv"; base64: string; sheetPart: string }
  | { operation: "slides"; base64: string }
  | { operation: "sheets"; base64: string }
  | { operation: "blocks"; base64: string }
  | {
      operation: "edit";
      format: Format;
      base64: string;
      operationsJSON: string;
    }
  | {
      operation: "validateSuggestion";
      format: OfficeFormat;
      base64: string;
      operationsJSON: string;
    };
const packetBytes =
  Math.ceil(LIMITS.fileBytes / 3) * 4 + LIMITS.inputBytes + LIMITS.reportBytes;
const responseSchema = z.union([
  z.strictObject({ bytes: z.string().min(1).max(packetBytes) }),
  z.strictObject({ inspection: inspectionSchema }),
  z.strictObject({ layout: slideLayoutSchema }),
  z.strictObject({ sheets: workbookLayoutSchema }),
  z.strictObject({ blocks: documentLayoutSchema }),
  z.strictObject({ validated: z.literal(true) }),
  z.strictObject({
    failure: z.strictObject({
      code: z.string().min(1).max(100),
      message: z.string().max(4096),
    }),
  }),
]);
type Response = z.infer<typeof responseSchema>;
const mode = "--deliverable-format";

function unpack(value: string): Uint8Array {
  const bytes = Buffer.from(value, "base64");
  if (
    !bytes.byteLength ||
    bytes.byteLength > LIMITS.fileBytes ||
    bytes.toString("base64") !== value
  )
    throw new Error(
      "deliverables: format process returned invalid native bytes",
    );
  return bytes;
}

export function isFormatProcess(): boolean {
  return process.argv[2] === mode;
}

export async function serveFormatProcess(): Promise<never> {
  const chunks: Buffer[] = [];
  let length = 0;
  try {
    for await (const chunk of process.stdin) {
      const bytes = Buffer.from(chunk as Uint8Array);
      length += bytes.byteLength;
      if (length > packetBytes)
        throw refusal(
          "format_capacity",
          "native format request exceeds its process budget",
        );
      chunks.push(bytes);
    }
    const input = JSON.parse(Buffer.concat(chunks).toString("utf8")) as Request;
    let response: Response;
    if (input.operation === "create")
      response = {
        bytes: Buffer.from(
          await createFile(input.format, input.contentJSON),
        ).toString("base64"),
      };
    else if (input.operation === "inspect")
      response = {
        inspection: await inspectFile(input.format, unpack(input.base64)),
      };
    else if (input.operation === "edit")
      response = {
        bytes: Buffer.from(
          await editFile(
            input.format,
            unpack(input.base64),
            input.operationsJSON,
          ),
        ).toString("base64"),
      };
    else if (input.operation === "validateSuggestion") {
      await validateSuggestion(
        input.format,
        unpack(input.base64),
        input.operationsJSON,
      );
      response = { validated: true };
    } else if (input.operation === "slides")
      response = { layout: layoutFile(unpack(input.base64)) };
    else if (input.operation === "sheets")
      response = { sheets: sheetsFile(unpack(input.base64)) };
    else if (input.operation === "blocks")
      response = { blocks: blocksFile(unpack(input.base64)) };
    else if (input.operation === "csv")
      response = {
        bytes: Buffer.from(
          exportCSV(unpack(input.base64), input.sheetPart),
        ).toString("base64"),
      };
    else throw new Error("deliverables: unknown format process request");
    await write(response);
  } catch (cause) {
    if (cause instanceof Refusal)
      await write({ failure: { code: cause.code, message: cause.message } });
    else if (cause instanceof Error)
      await write({
        failure: {
          code: "invalid_native",
          message: cause.message.slice(0, 4096),
        },
      });
    else {
      process.stderr.write("deliverables: native format operation failed\n");
      process.exit(1);
    }
  }
  process.exit(0);
}

async function write(response: Response): Promise<void> {
  const bytes = Buffer.from(JSON.stringify(response));
  if (bytes.byteLength > packetBytes)
    throw new Error("deliverables: format process output exceeds its budget");
  await new Promise<void>((resolve, reject) =>
    process.stdout.write(bytes, (error) => (error ? reject(error) : resolve())),
  );
}

// Resolves when its turn comes; a canceled wait leaves its place in line.
function turnOf(turn: Promise<void>, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const abort = () => reject(signal.reason);
    signal.addEventListener("abort", abort, { once: true });
    void turn.then(() => {
      signal.removeEventListener("abort", abort);
      resolve();
    });
  });
}

export class Formats {
  readonly #entry: string;
  #active?: { cancel: () => void; done: Promise<unknown> };
  // Operations run one confined process at a time in arrival order, so
  // concurrent readers and edits wait instead of refusing each other.
  #line: Promise<void> = Promise.resolve();
  #admitted = 0;
  #closed = false;
  #cleanupFailure?: unknown;

  constructor(entry: string) {
    if (!isAbsolute(entry))
      throw new Error("deliverables: packaged format entry must be absolute");
    this.#entry = entry;
  }

  async #call(input: Request, signal: AbortSignal): Promise<Response> {
    signal.throwIfAborted();
    if (this.#closed)
      throw refusal("formats_closed", "format processing is closed");
    const data = Buffer.from(JSON.stringify(input));
    if (data.byteLength > packetBytes)
      throw refusal(
        "format_capacity",
        "native format request exceeds its process budget",
      );
    if (this.#admitted >= LIMITS.formatOperations)
      throw refusal(
        "formats_busy",
        "too many native format operations are waiting",
      );
    this.#admitted++;
    const turn = this.#line;
    let leave!: () => void;
    const left = new Promise<void>((resolve) => (leave = resolve));
    this.#line = turn.then(() => left);
    try {
      await turnOf(turn, signal);
      if (this.#closed)
        throw refusal("formats_closed", "format processing is closed");
      return await this.#run(data, signal);
    } finally {
      this.#admitted--;
      leave();
    }
  }

  #run(data: Uint8Array, signal: AbortSignal): Promise<Response> {
    const controller = new AbortController();
    const owned = AbortSignal.any([signal, controller.signal]);
    const active = {
      cancel: () =>
        controller.abort(
          new Error("deliverables: format processing is closing"),
        ),
      done: Promise.resolve() as Promise<unknown>,
    };
    this.#active = active;
    const timeout = setTimeout(
      () =>
        controller.abort(
          refusal(
            "format_timeout",
            "native format processing exceeded its deadline",
          ),
        ),
      LIMITS.formatMs,
    );
    const done = this.#invoke(data, owned).finally(() => {
      clearTimeout(timeout);
      this.#active = undefined;
    });
    active.done = done;
    return done;
  }

  async #invoke(data: Uint8Array, signal: AbortSignal): Promise<Response> {
    const platform = process.platform;
    if (platform !== "linux" && platform !== "darwin")
      throw refusal(
        "format_platform",
        "bounded native format processing requires Linux or macOS",
      );
    const entry = await realpath(this.#entry);
    const node = await realpath(process.execPath);
    signal.throwIfAborted();
    const root = await mkdtemp(
      join(await realpath(tmpdir()), "flame-deliverable-format-"),
    );
    const directory: PrivateDirectory = {
      root,
      input: join(root, "input"),
      output: join(root, "output"),
      home: join(root, "home"),
      temporary: join(root, "tmp"),
    };
    let response!: Response;
    let failure: { cause: unknown } | undefined;
    try {
      for (const path of [
        directory.input,
        directory.output,
        directory.home,
        directory.temporary,
      ])
        await mkdir(path, { mode: 0o700 });
      signal.throwIfAborted();
      const plan = confinedPlan(
        platform,
        directory,
        [...new Set([dirname(entry), dirname(node)])],
        node,
        [
          `--max-old-space-size=${LIMITS.formatHeapMiB}`,
          "--disable-wasm-trap-handler",
          "--permission",
          `--allow-fs-read=${dirname(entry)}`,
          entry,
          mode,
        ],
        {
          memoryBytes: LIMITS.formatMemoryBytes,
          cpuSeconds: LIMITS.formatCpuSeconds,
          maximumFileBytes: 0,
          openFiles: LIMITS.formatOpenFiles,
          writable: false,
        },
      );
      const result = await runProcess(plan, signal, {
        input: data,
        inputBytes: packetBytes,
        stdoutBytes: packetBytes,
        stderrBytes: LIMITS.outputBytes,
      });
      response = responseSchema.parse(JSON.parse(result.stdout));
      if ("failure" in response)
        throw new Refusal(response.failure.code, response.failure.message);
    } catch (cause) {
      if (cause instanceof UnconfirmedProcessExit) {
        this.#cleanupFailure = cause;
        this.#closed = true;
      }
      failure = { cause };
    }
    if (!this.#cleanupFailure) {
      try {
        await rm(root, { recursive: true, force: true });
      } catch (cause) {
        this.#cleanupFailure = cause;
        this.#closed = true;
        failure = {
          cause: failure
            ? new AggregateError(
                [failure.cause, cause],
                "deliverables: format operation and scratch cleanup failed",
              )
            : cause,
        };
      }
    }
    if (failure) throw failure.cause;
    signal.throwIfAborted();
    return response;
  }

  async create(
    format: Exclude<Format, "pdf">,
    contentJSON: string,
    signal: AbortSignal,
  ): Promise<Uint8Array> {
    const result = await this.#call(
      { operation: "create", format, contentJSON },
      signal,
    );
    if (!("bytes" in result))
      throw new Error(
        "deliverables: format creation did not return native bytes",
      );
    return unpack(result.bytes);
  }

  async inspect(
    format: Format,
    bytes: Uint8Array,
    signal: AbortSignal,
  ): Promise<Inspection> {
    const result = await this.#call(
      {
        operation: "inspect",
        format,
        base64: Buffer.from(bytes).toString("base64"),
      },
      signal,
    );
    if (!("inspection" in result))
      throw new Error(
        "deliverables: format inspection did not return observations",
      );
    if (result.inspection.format !== format)
      throw new Error(
        "deliverables: format inspection returned a different source format",
      );
    if (
      Buffer.byteLength(JSON.stringify(result.inspection)) > LIMITS.reportBytes
    )
      throw refusal(
        "report_capacity",
        "format inspection exceeds its evidence budget",
      );
    return result.inspection;
  }

  async edit(
    format: Format,
    bytes: Uint8Array,
    operationsJSON: string,
    signal: AbortSignal,
  ): Promise<Uint8Array> {
    const result = await this.#call(
      {
        operation: "edit",
        format,
        base64: Buffer.from(bytes).toString("base64"),
        operationsJSON,
      },
      signal,
    );
    if (!("bytes" in result))
      throw new Error("deliverables: format edit did not return native bytes");
    return unpack(result.bytes);
  }

  async validateSuggestion(
    format: OfficeFormat,
    bytes: Uint8Array,
    operationsJSON: string,
    signal: AbortSignal,
  ): Promise<void> {
    const result = await this.#call(
      {
        operation: "validateSuggestion",
        format,
        base64: Buffer.from(bytes).toString("base64"),
        operationsJSON,
      },
      signal,
    );
    if (!("validated" in result))
      throw new Error(
        "deliverables: format process did not validate the suggestion",
      );
  }

  async close(): Promise<void> {
    this.#closed = true;
    const active = this.#active;
    if (active) {
      active.cancel();
      try {
        await active.done;
      } catch {
        /* The original caller retains a confirmed operation failure. */
      }
    }
    if (this.#cleanupFailure) throw this.#cleanupFailure;
  }

  async blocks(
    bytes: Uint8Array,
    signal: AbortSignal,
  ): Promise<DocumentLayout> {
    const result = await this.#call(
      { operation: "blocks", base64: Buffer.from(bytes).toString("base64") },
      signal,
    );
    if (!("blocks" in result))
      throw new Error(
        "deliverables: block projection did not return a document layout",
      );
    return result.blocks;
  }

  async sheets(
    bytes: Uint8Array,
    signal: AbortSignal,
  ): Promise<WorkbookLayout> {
    const result = await this.#call(
      { operation: "sheets", base64: Buffer.from(bytes).toString("base64") },
      signal,
    );
    if (!("sheets" in result))
      throw new Error(
        "deliverables: sheet projection did not return a workbook layout",
      );
    return result.sheets;
  }

  async slides(bytes: Uint8Array, signal: AbortSignal): Promise<SlideLayout> {
    const result = await this.#call(
      { operation: "slides", base64: Buffer.from(bytes).toString("base64") },
      signal,
    );
    if (!("layout" in result))
      throw new Error("deliverables: slide projection did not return a layout");
    return result.layout;
  }

  async csv(
    bytes: Uint8Array,
    sheetPart: string,
    signal: AbortSignal,
  ): Promise<Uint8Array> {
    const result = await this.#call(
      {
        operation: "csv",
        base64: Buffer.from(bytes).toString("base64"),
        sheetPart,
      },
      signal,
    );
    if (!("bytes" in result))
      throw new Error("deliverables: CSV export did not return bytes");
    return unpack(result.bytes);
  }
}
