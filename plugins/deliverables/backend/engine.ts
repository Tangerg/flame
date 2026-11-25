import { lstat, mkdtemp, realpath, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { Refusal } from "@flame/backend-sdk/mcp";
import { LIMITS } from "../contracts/limits";
import { FORMATS, type Format } from "../contracts/content";
import type {
  Check,
  EngineCatalog,
  EngineRenderReport,
} from "../contracts/evidence";
import {
  confinedPlan,
  SYSTEM_FILES,
  SYSTEM_ROOTS,
  type PrivateDirectory,
} from "./confinement";
import { runProcess, UnconfirmedProcessExit } from "./process";
import { Office } from "./office";
import { ASSET_MIME } from "./document";
import {
  type Configuration,
  type InstalledTools,
  SETTINGS,
  type Tool,
  absolute,
  configuration,
  hasErrorCode,
  installedExecutable,
  requireConfinement,
  within,
} from "./engine-installation";
import {
  type RenderFile,
  checkedDirectory,
  digest,
  pageCount,
  pdf,
  png,
  privateDirectory,
  readOutput,
  textChecks,
} from "./engine-output";

export class Engine {
  readonly #configuration: Configuration;
  readonly #data: string;
  #active?: { controller: AbortController; done: Promise<unknown> };
  #closed = false;
  #cleanupFailure?: unknown;

  constructor(environment: NodeJS.ProcessEnv, privateData: string) {
    this.#configuration = configuration(environment);
    this.#data = privateData;
    if (!absolute(privateData))
      this.#configuration.issues.push(
        "The plugin data directory must be an absolute canonical path.",
      );
  }

  catalog(): EngineCatalog {
    const config = this.#configuration;
    return {
      platform: process.platform,
      configured:
        !config.issues.length &&
        (process.platform === "linux" || process.platform === "darwin") &&
        !!config.tools.office &&
        !!config.tools.pdfinfo &&
        !!config.tools.pdftoppm,
      configuration: Object.fromEntries([
        ...Object.entries(SETTINGS).map(([tool, name]) => [
          name,
          config.tools[tool as Tool],
        ]),
        ["FLAME_DELIVERABLES_TOOL_ROOTS", JSON.stringify(config.roots)],
      ]),
      limitations: [
        ...config.issues,
        "Configured paths do not establish availability; each invocation verifies executable files and native confinement.",
        "LibreOffice can change Microsoft Office features, layout and formula behavior. Recalculation creates a new revision and needs formula-result inspection.",
        "Font substitution and visual quality remain unverified until the actual page previews are reviewed.",
        "Only explicitly mounted system fonts and installation assets are readable; user font directories are not exposed.",
        "macOS requires actual binaries that work with process-fork denied. Launchers or tools requiring helper processes are refused.",
        ...(!config.tools.pdftotext
          ? [
              "PDF text extraction is unavailable until FLAME_DELIVERABLES_PDFTOTEXT is configured.",
            ]
          : []),
      ],
    };
  }

  render(
    format: Format,
    bytes: Uint8Array,
    signal: AbortSignal,
  ): Promise<{ report: EngineRenderReport; files: RenderFile[] }> {
    return this.#admit(
      bytes,
      signal,
      async (input, directory, tools, owned) => {
        if (!FORMATS.includes(format))
          throw new Refusal(
            "engine_format",
            "The requested rendering format is unsupported.",
          );
        this.#required(
          tools,
          format === "pdf"
            ? ["pdfinfo", "pdftoppm"]
            : ["office", "pdfinfo", "pdftoppm"],
        );
        const source = join(directory.input, `source.${format}`);
        await writeFile(source, input, { flag: "wx", mode: 0o600 });
        const engines: EngineRenderReport["engines"] = [];
        if (format !== "pdf")
          engines.push(await this.#version("office", tools, directory, owned));
        engines.push(await this.#version("pdfinfo", tools, directory, owned));
        engines.push(await this.#version("pdftoppm", tools, directory, owned));
        let pdfBytes = input;
        if (format !== "pdf") {
          const outputPath = await this.#office(
            directory,
            tools.office!,
            source,
            "pdf",
            owned,
          );
          pdfBytes = await readOutput(outputPath, LIMITS.fileBytes);
        }
        pdf(pdfBytes);
        const inputPDF =
          format === "pdf" ? source : join(directory.input, "rendered.pdf");
        if (format !== "pdf")
          await writeFile(inputPDF, pdfBytes, { flag: "wx", mode: 0o600 });
        const info = await this.#command(
          directory,
          tools.pdfinfo,
          [inputPDF],
          owned,
        );
        const count = pageCount(info.stdout);
        const files: RenderFile[] = [
          {
            filename: "rendered.pdf",
            mime: ASSET_MIME.pdf,
            bytes: pdfBytes,
          },
        ];
        const pages: EngineRenderReport["pages"] = [];
        let size = pdfBytes.byteLength;
        for (let number = 1; number <= count; number += 1) {
          const preview = await this.#pagePreview(
            directory,
            tools.pdftoppm,
            inputPDF,
            number,
            LIMITS.renderBytes - size,
            owned,
          );
          size += preview.bytes.byteLength;
          files.push({
            filename: preview.filename,
            mime: ASSET_MIME.png,
            bytes: preview.bytes,
          });
          pages.push({ number, filename: preview.filename });
        }
        const checks: Check[] = [
          {
            code: "rendered_pages",
            severity: "info",
            message: `Poppler produced ${count} page previews from the actual rendered PDF.`,
          },
          {
            code: "visual_review_required",
            severity: "warning",
            message:
              "Page appearance, clipping, font substitution and semantic correctness require explicit review of these exact previews.",
          },
        ];
        let extractedText = "";
        if (tools.pdftotext) {
          engines.push(
            await this.#version("pdftotext", tools, directory, owned),
          );
          const extraction = await this.#extractText(
            directory,
            tools.pdftotext,
            inputPDF,
            count,
            LIMITS.renderBytes - size,
            owned,
          );
          extractedText = extraction.text;
          checks.push(...textChecks(extraction.text, extraction.truncated));
        } else
          checks.push({
            code: "text_unavailable",
            severity: "warning",
            message:
              "Text extraction was not performed because its tool is not configured.",
          });
        return {
          report: {
            platform: process.platform,
            engines,
            pages,
            checks,
            extractedText,
            fontStatus: "unverified",
          },
          files,
        };
      },
    );
  }

  recalculate(
    bytes: Uint8Array,
    signal: AbortSignal,
  ): Promise<{ bytes: Uint8Array; report: Record<string, unknown> }> {
    return this.#admit(
      bytes,
      signal,
      async (input, directory, tools, owned) => {
        this.#required(tools, ["office"]);
        const engine = await this.#version("office", tools, directory, owned);
        const source = join(directory.input, "source.xlsx");
        await writeFile(source, input, { flag: "wx", mode: 0o600 });
        const outputPath = await this.#office(
          directory,
          tools.office!,
          source,
          "xlsx",
          owned,
        );
        const output = await readOutput(outputPath, LIMITS.fileBytes);
        if (output[0] !== 0x50 || output[1] !== 0x4b)
          throw new Refusal(
            "engine_output_invalid",
            "LibreOffice did not produce an XLSX package.",
          );
        return {
          bytes: output,
          report: {
            platform: process.platform,
            engines: [engine],
            sourceSha256: digest(input),
            outputSha256: digest(output),
            recalculation: tools.office!.recalculation,
            externalLinks: "disabled",
            checks: [
              {
                code: "formula_inspection_required",
                severity: "warning",
                message:
                  "Inspect the resulting workbook for missing caches and formula errors. LibreOffice conversion can change unsupported workbook features.",
              },
            ],
          },
        };
      },
    );
  }

  async close(): Promise<void> {
    this.#closed = true;
    const active = this.#active;
    active?.controller.abort(
      new DOMException("The rendering owner is closing.", "AbortError"),
    );
    await active?.done.catch(() => {});
    if (this.#cleanupFailure) throw this.#cleanupFailure;
  }

  #required(tools: InstalledTools, names: Tool[]): void {
    if (names.some((name) => !tools[name]))
      throw new Refusal(
        "engine_not_configured",
        "The required rendering tools are not configured.",
      );
  }

  #admit<T>(
    bytes: Uint8Array,
    signal: AbortSignal,
    operation: (
      bytes: Uint8Array,
      directory: PrivateDirectory,
      tools: InstalledTools,
      signal: AbortSignal,
    ) => Promise<T>,
  ): Promise<T> {
    signal.throwIfAborted();
    if (this.#closed || this.#cleanupFailure)
      throw new Refusal(
        "engine_closed",
        "The rendering owner is closed or its cleanup remains unconfirmed.",
      );
    if (this.#active)
      throw new Refusal(
        "engine_busy",
        "A rendering operation is already active; no second operation was queued.",
      );
    if (
      !(bytes instanceof Uint8Array) ||
      !bytes.byteLength ||
      bytes.byteLength > LIMITS.fileBytes
    )
      throw new Refusal(
        "engine_input_limit",
        "The native source bytes exceed the rendering input limit.",
      );
    if (this.#configuration.issues.length)
      throw new Refusal(
        "engine_configuration",
        this.#configuration.issues.join(" "),
      );
    if (process.platform !== "linux" && process.platform !== "darwin")
      throw new Refusal(
        "engine_unavailable",
        "Native rendering confinement is unsupported on this platform.",
      );
    const input = new Uint8Array(bytes);
    const controller = new AbortController();
    const deadline = setTimeout(
      () =>
        controller.abort(
          new DOMException("The rendering deadline expired.", "TimeoutError"),
        ),
      LIMITS.processMs,
    );
    const owned = AbortSignal.any([signal, controller.signal]);
    const active = { controller, done: Promise.resolve() as Promise<unknown> };
    this.#active = active;
    const done = this.#invoke(input, owned, operation).finally(() => {
      clearTimeout(deadline);
      if (this.#active === active) this.#active = undefined;
    });
    active.done = done;
    return done;
  }

  async #invoke<T>(
    input: Uint8Array,
    signal: AbortSignal,
    operation: (
      bytes: Uint8Array,
      directory: PrivateDirectory,
      tools: InstalledTools,
      signal: AbortSignal,
    ) => Promise<T>,
  ): Promise<T> {
    let directory: PrivateDirectory | undefined;
    let result!: T;
    let failure: { cause: unknown } | undefined;
    try {
      const tools = await this.#tools(signal);
      directory = await this.#directory(signal);
      signal.throwIfAborted();
      result = await operation(input, directory, tools, signal);
    } catch (cause) {
      if (cause instanceof UnconfirmedProcessExit) this.#cleanupFailure = cause;
      failure = { cause };
    }
    if (directory && !this.#cleanupFailure) {
      try {
        await rm(directory.root, { recursive: true, force: true });
      } catch (cause) {
        this.#cleanupFailure = cause;
        failure = {
          cause: failure
            ? new AggregateError(
                [failure.cause, cause],
                "deliverables: invocation and scratch cleanup failed",
              )
            : cause,
        };
      }
    }
    if (failure) throw failure.cause;
    signal.throwIfAborted();
    return result;
  }

  async #tools(signal: AbortSignal): Promise<InstalledTools> {
    const config = this.#configuration;
    const system =
      process.platform === "linux" ? SYSTEM_ROOTS.linux : SYSTEM_ROOTS.darwin;
    await this.#admitReadRoots(system, signal);
    const roots = [...system, ...config.roots];
    const tools = { ...config.tools };
    for (const key of Object.keys(tools) as Tool[]) {
      if (!tools[key]) continue;
      signal.throwIfAborted();
      const path = await installedExecutable(tools[key], roots);
      if (!path)
        throw new Refusal(
          "engine_configuration",
          `${SETTINGS[key]} must identify an executable inside an admitted installation root.`,
        );
      tools[key] = path;
    }
    await requireConfinement();
    return {
      ...tools,
      office: tools.office
        ? await Office.load(tools.office, roots, signal)
        : undefined,
    };
  }

  // Every read root is mounted into the renderer, so none may expose private plugin data.
  async #admitReadRoots(
    system: readonly string[],
    signal: AbortSignal,
  ): Promise<void> {
    const configuredRoots = this.#configuration.roots;
    const data = await realpath(this.#data);
    if (data !== this.#data || !(await lstat(data)).isDirectory())
      throw new Refusal(
        "engine_private_directory",
        "The plugin data directory must be canonical.",
      );
    for (const root of [...system, ...SYSTEM_FILES, ...configuredRoots]) {
      signal.throwIfAborted();
      const configured = configuredRoots.includes(root);
      let canonical: string;
      try {
        canonical = await realpath(root);
      } catch (cause) {
        if (configured || !hasErrorCode(cause, "ENOENT")) throw cause;
        canonical = root;
      }
      if (
        within(canonical, data) ||
        within(data, canonical) ||
        (configured &&
          (canonical !== root || !(await lstat(root)).isDirectory()))
      )
        throw new Refusal(
          "engine_configuration",
          "An installation read root is noncanonical or overlaps private plugin data.",
        );
    }
  }

  async #directory(signal: AbortSignal): Promise<PrivateDirectory> {
    signal.throwIfAborted();
    const base = join(this.#data, "engine");
    try {
      await privateDirectory(base);
    } catch (cause) {
      if (!hasErrorCode(cause, "EEXIST")) throw cause;
    }
    await checkedDirectory(base);
    const root = await mkdtemp(join(base, "invocation-"));
    try {
      await checkedDirectory(root);
      const directory = {
        root,
        input: join(root, "input"),
        output: join(root, "output"),
        home: join(root, "home"),
        temporary: join(root, "tmp"),
      };
      for (const child of [
        directory.input,
        directory.output,
        directory.home,
        directory.temporary,
      ])
        await privateDirectory(child);
      for (const child of ["cache", "config", "data"])
        await privateDirectory(join(directory.home, child));
      await privateDirectory(join(directory.home, "cache", "fontconfig"));
      return directory;
    } catch (cause) {
      try {
        await rm(root, { recursive: true, force: true });
      } catch (cleanup) {
        this.#cleanupFailure = cleanup;
        throw cleanup;
      }
      throw cause;
    }
  }

  #command(
    directory: PrivateDirectory,
    executable: string,
    args: string[],
    signal: AbortSignal,
    maximumFileBytes: number = LIMITS.fileBytes,
  ) {
    if (maximumFileBytes < 1)
      throw new Refusal(
        "engine_output_limit",
        "The rendered assets exceeded their aggregate byte limit.",
      );
    const platform = process.platform;
    if (platform !== "linux" && platform !== "darwin")
      throw new Refusal(
        "engine_unavailable",
        "Native rendering confinement is unsupported.",
      );
    return runProcess(
      confinedPlan(
        platform,
        directory,
        this.#configuration.roots,
        executable,
        args,
        {
          memoryBytes: LIMITS.processMemoryBytes,
          cpuSeconds: Math.ceil(LIMITS.processMs / 1000),
          maximumFileBytes,
          writable: true,
        },
        {
          SAL_USE_VCLPLUGIN: "svp",
          SAL_DISABLE_OPENCL: "1",
          // The shared pool must not grow with the host's CPU count.
          MAX_CONCURRENCY: String(LIMITS.processConcurrency),
        },
      ),
      signal,
    );
  }

  async #version(
    tool: Tool,
    tools: InstalledTools,
    directory: PrivateDirectory,
    signal: AbortSignal,
  ): Promise<{ name: string; version: string }> {
    if (tool === "office" && tools.office!.identity)
      return tools.office!.identity;
    const executable =
      tool === "office" ? tools.office!.executable : tools[tool];
    const output = await this.#command(
      directory,
      executable,
      tool === "office" ? ["--version"] : ["-v"],
      signal,
    );
    const version = `${output.stdout}\n${output.stderr}`
      .split(/\r?\n/u)
      .map((line) => line.trim())
      .find(Boolean);
    if (!version || version.length > 512)
      throw new Refusal(
        "engine_version",
        "The confined tool did not report a bounded version identity.",
      );
    return {
      name: tool === "office" ? "LibreOffice" : `Poppler ${tool}`,
      version,
    };
  }

  async #pagePreview(
    directory: PrivateDirectory,
    pdftoppm: string,
    inputPDF: string,
    number: number,
    remainingBytes: number,
    signal: AbortSignal,
  ): Promise<{ filename: string; bytes: Uint8Array }> {
    signal.throwIfAborted();
    const name = `page-${String(number).padStart(4, "0")}`;
    const prefix = join(directory.output, name);
    const maximum = Math.min(LIMITS.previewFileBytes, remainingBytes);
    await this.#command(
      directory,
      pdftoppm,
      [
        "-f",
        String(number),
        "-l",
        String(number),
        "-singlefile",
        "-scale-to",
        String(LIMITS.previewPixels),
        "-png",
        inputPDF,
        prefix,
      ],
      signal,
      maximum,
    );
    const bytes = await readOutput(`${prefix}.png`, maximum);
    png(bytes);
    await rm(`${prefix}.png`);
    return { filename: `${name}.png`, bytes };
  }

  async #extractText(
    directory: PrivateDirectory,
    pdftotext: string,
    inputPDF: string,
    count: number,
    remainingBytes: number,
    signal: AbortSignal,
  ): Promise<{ text: string; truncated: boolean }> {
    const textPath = join(directory.output, "extracted.txt");
    const maximum = Math.min(LIMITS.fileBytes, remainingBytes);
    await this.#command(
      directory,
      pdftotext,
      [
        "-f",
        "1",
        "-l",
        String(count),
        "-layout",
        "-enc",
        "UTF-8",
        inputPDF,
        textPath,
      ],
      signal,
      maximum,
    );
    // Empty text is a valid observation for an image-only PDF.
    const bytes = await readOutput(textPath, maximum, "maybeEmpty");
    const truncated = bytes.byteLength > LIMITS.renderTextBytes;
    try {
      const text = new TextDecoder("utf-8", { fatal: true }).decode(
        bytes.subarray(0, LIMITS.renderTextBytes),
        { stream: truncated },
      );
      return { text, truncated };
    } catch {
      throw new Refusal(
        "engine_text_invalid",
        "The extracted text is not valid UTF-8.",
      );
    }
  }

  async #office(
    directory: PrivateDirectory,
    office: Office,
    source: string,
    format: "pdf" | "xlsx",
    signal: AbortSignal,
  ) {
    const conversion = await office.prepareConversion(
      directory,
      source,
      format,
    );
    const output = await this.#command(
      directory,
      office.executable,
      conversion.arguments,
      signal,
    );
    office.verify(output.stdout);
    return conversion.outputPath;
  }
}
