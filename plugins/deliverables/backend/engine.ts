import { constants } from "node:fs";
import {
  access,
  lstat,
  mkdir,
  mkdtemp,
  open,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { isAbsolute, join, relative, resolve, sep } from "node:path";
import { createHash } from "node:crypto";
import { Refusal } from "@qimu/node";
import { LIMITS } from "../contracts/limits";
import type { Format } from "../contracts/capability";
import type {
  Check,
  EngineCatalog,
  EngineRenderReport,
} from "../contracts/evidence";
import {
  confinedPlan,
  DARWIN_SHELL,
  SYSTEM_FILES,
  SYSTEM_ROOTS,
  type PrivateDirectory,
} from "./confinement";
import { runProcess, UnconfirmedProcessExit } from "./process";
import { Office } from "./office";

type Tool = "office" | "pdfinfo" | "pdftoppm" | "pdftotext";
type InstalledTools = Omit<Record<Tool, string>, "office"> & {
  office?: Office;
};
interface Configuration {
  tools: Record<Tool, string>;
  roots: string[];
  issues: string[];
}
interface RenderFile {
  filename: string;
  mime: string;
  bytes: Uint8Array;
}

const SETTINGS: Record<Tool, string> = {
  office: "FLAME_DELIVERABLES_OFFICE",
  pdfinfo: "FLAME_DELIVERABLES_PDFINFO",
  pdftoppm: "FLAME_DELIVERABLES_PDFTOPPM",
  pdftotext: "FLAME_DELIVERABLES_PDFTOTEXT",
};
const GENERAL_ROOTS = new Set([
  "/",
  "/home",
  "/Users",
  "/root",
  "/workspace",
  "/workspace/scratch",
  "/tmp",
  "/var",
  "/private",
  "/opt",
  "/Applications",
  "/etc",
]);
const OFFICE_PROFILE = `<?xml version="1.0" encoding="UTF-8"?>
<oor:items xmlns:oor="http://openoffice.org/2001/registry">
<item oor:path="/org.openoffice.Office.Common/Security/Scripting">
<prop oor:name="DisableMacrosExecution" oor:op="fuse"><value>true</value></prop>
<prop oor:name="DisableActiveContent" oor:op="fuse"><value>true</value></prop>
</item>
<item oor:path="/org.openoffice.Office.Calc/Content/Update">
<prop oor:name="Link" oor:op="fuse"><value>1</value></prop>
</item>
<item oor:path="/org.openoffice.Office.Calc/Formula/Load">
<prop oor:name="OOXMLRecalcMode" oor:op="fuse"><value>0</value></prop>
</item>
</oor:items>`;

function within(root: string, path: string): boolean {
  const suffix = relative(root, path);
  return (
    suffix === "" ||
    (!suffix.startsWith(`..${sep}`) && suffix !== ".." && !isAbsolute(suffix))
  );
}

function absolute(value: string): boolean {
  return (
    isAbsolute(value) &&
    resolve(value) === value &&
    !Array.from(value).some(
      (character) =>
        character.codePointAt(0)! < 32 || character.codePointAt(0) === 127,
    )
  );
}

function configuration(environment: NodeJS.ProcessEnv): Configuration {
  const issues: string[] = [];
  const tools = {} as Record<Tool, string>;
  for (const [tool, setting] of Object.entries(SETTINGS) as Array<
    [Tool, string]
  >) {
    const value = environment[setting] ?? "";
    tools[tool] = value;
    if (value && !absolute(value))
      issues.push(`${setting} must be an absolute normalized executable path.`);
  }
  let roots: string[] = [];
  if (environment.FLAME_DELIVERABLES_TOOL_ROOTS) {
    try {
      const value: unknown = JSON.parse(
        environment.FLAME_DELIVERABLES_TOOL_ROOTS,
      );
      if (
        !Array.isArray(value) ||
        value.length > 16 ||
        value.some(
          (root) =>
            typeof root !== "string" ||
            !absolute(root) ||
            GENERAL_ROOTS.has(root),
        )
      )
        throw new Error("invalid roots");
      roots = [...new Set(value as string[])].sort();
    } catch {
      issues.push(
        "FLAME_DELIVERABLES_TOOL_ROOTS must name at most 16 specific absolute installation directories, never a general host root.",
      );
    }
  }
  return { tools, roots, issues };
}

async function privateDirectory(path: string): Promise<void> {
  await mkdir(path, { mode: 0o700 });
}

async function checkedDirectory(path: string): Promise<void> {
  const info = await lstat(path);
  if (
    !info.isDirectory() ||
    info.isSymbolicLink() ||
    (await realpath(path)) !== path ||
    (info.mode & 0o077) !== 0 ||
    (process.getuid && info.uid !== process.getuid())
  )
    throw new Refusal(
      "engine_private_directory",
      "The rendering directory must be a canonical private directory owned by this process.",
    );
}

async function readOutput(
  path: string,
  maximum: number,
  allowEmpty = false,
): Promise<Uint8Array> {
  let handle;
  try {
    // A FIFO can block open forever after its writer exits. Refuse special
    // files first, and keep the descriptor nonblocking across the final check.
    const entry = await lstat(path);
    if (!entry.isFile() || entry.isSymbolicLink())
      throw new Refusal(
        "engine_output_missing",
        "The rendering process did not produce the required regular output file.",
      );
    handle = await open(
      path,
      constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK,
    );
  } catch (cause) {
    if (
      cause instanceof Error &&
      "code" in cause &&
      (cause.code === "ENOENT" || cause.code === "ELOOP")
    )
      throw new Refusal(
        "engine_output_missing",
        "The rendering process did not produce the required regular output file.",
      );
    throw cause;
  }
  try {
    const info = await handle.stat();
    if (
      !info.isFile() ||
      info.nlink !== 1 ||
      (!allowEmpty && info.size === 0) ||
      info.size > maximum
    )
      throw new Refusal(
        "engine_output_limit",
        "The rendering output is empty, linked, or exceeds its admitted byte limit.",
      );
    const bytes = await handle.readFile();
    if (bytes.byteLength !== info.size || bytes.byteLength > maximum)
      throw new Refusal(
        "engine_output_changed",
        "The rendering output changed after its process exited.",
      );
    return bytes;
  } finally {
    await handle.close();
  }
}

function pdf(bytes: Uint8Array): void {
  if (Buffer.from(bytes.subarray(0, 5)).toString("ascii") !== "%PDF-")
    throw new Refusal(
      "engine_output_invalid",
      "The renderer did not produce a PDF file.",
    );
}

function png(bytes: Uint8Array): void {
  const value = Buffer.from(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  if (
    value.length < 45 ||
    !value
      .subarray(0, 8)
      .equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])) ||
    value.readUInt32BE(8) !== 13 ||
    value.toString("ascii", 12, 16) !== "IHDR" ||
    value.toString("ascii", value.length - 8, value.length - 4) !== "IEND"
  )
    throw new Refusal(
      "engine_preview_invalid",
      "The renderer did not produce a complete PNG preview.",
    );
  const width = value.readUInt32BE(16),
    height = value.readUInt32BE(20);
  if (
    !width ||
    !height ||
    width > LIMITS.previewPixels ||
    height > LIMITS.previewPixels
  )
    throw new Refusal(
      "engine_preview_limit",
      "The page preview exceeds its admitted pixel dimensions.",
    );
}

function digest(bytes: Uint8Array): string {
  return createHash("sha256").update(bytes).digest("hex");
}

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
        if (!["docx", "xlsx", "pptx", "pdf"].includes(format))
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
          await this.#office(directory, tools.office!, source, "pdf", owned);
          pdfBytes = await readOutput(
            join(directory.output, "source.pdf"),
            LIMITS.fileBytes,
          );
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
        const match = /^Pages:\s+([1-9][0-9]*)\s*$/m.exec(info.stdout);
        const count = match ? Number(match[1]) : NaN;
        if (
          !Number.isSafeInteger(count) ||
          count < 1 ||
          count > LIMITS.renderPages
        )
          throw new Refusal(
            "engine_page_limit",
            `The PDF must contain between 1 and ${LIMITS.renderPages} pages.`,
          );
        const files: RenderFile[] = [
          {
            filename: "rendered.pdf",
            mime: "application/pdf",
            bytes: pdfBytes,
          },
        ];
        const pages: EngineRenderReport["pages"] = [];
        let size = pdfBytes.byteLength;
        for (let number = 1; number <= count; number += 1) {
          owned.throwIfAborted();
          const name = `page-${String(number).padStart(4, "0")}`;
          const prefix = join(directory.output, name);
          await this.#command(
            directory,
            tools.pdftoppm,
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
            owned,
            Math.min(LIMITS.previewFileBytes, LIMITS.renderBytes - size),
          );
          const image = await readOutput(
            `${prefix}.png`,
            Math.min(LIMITS.previewFileBytes, LIMITS.renderBytes - size),
          );
          png(image);
          size += image.byteLength;
          const filename = `${name}.png`;
          files.push({ filename, mime: "image/png", bytes: image });
          pages.push({ number, filename });
          await rm(`${prefix}.png`);
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
          const textPath = join(directory.output, "extracted.txt");
          await this.#command(
            directory,
            tools.pdftotext,
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
            owned,
            Math.min(LIMITS.fileBytes, LIMITS.renderBytes - size),
          );
          // Empty text is a valid observation for an image-only PDF.
          const text = await readOutput(
            textPath,
            Math.min(LIMITS.fileBytes, LIMITS.renderBytes - size),
            true,
          );
          const truncated = text.byteLength > LIMITS.renderTextBytes;
          try {
            extractedText = new TextDecoder("utf-8", { fatal: true }).decode(
              text.subarray(0, LIMITS.renderTextBytes),
              { stream: truncated },
            );
          } catch {
            throw new Refusal(
              "engine_text_invalid",
              "The extracted text is not valid UTF-8.",
            );
          }
          if (truncated)
            checks.push({
              code: "text_truncated",
              severity: "warning",
              message:
                "Extracted text exceeds the retained text limit; the page previews remain the complete visual evidence.",
            });
          if (!extractedText.trim())
            checks.push({
              code: "text_empty",
              severity: "warning",
              message:
                "No searchable text was observed; inspect the page previews for image-only or missing content.",
            });
          if (extractedText.includes("\ufffd"))
            checks.push({
              code: "text_replacement_character",
              severity: "warning",
              message:
                "Extracted text contains replacement characters; verify the corresponding glyphs on the page previews.",
            });
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
        await this.#office(directory, tools.office!, source, "xlsx", owned);
        const output = await readOutput(
          join(directory.output, "source.xlsx"),
          LIMITS.fileBytes,
        );
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
            recalculation:
              "OOXMLRecalcMode=0 (always on load) in a private LibreOffice profile",
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
    const data = await realpath(this.#data);
    if (data !== this.#data || !(await lstat(data)).isDirectory())
      throw new Refusal(
        "engine_private_directory",
        "The plugin data directory must be canonical.",
      );
    for (const root of [...system, ...SYSTEM_FILES, ...config.roots]) {
      signal.throwIfAborted();
      const configured = config.roots.includes(root);
      let canonical: string;
      try {
        canonical = await realpath(root);
      } catch (cause) {
        if (
          configured ||
          !(cause instanceof Error) ||
          !("code" in cause) ||
          cause.code !== "ENOENT"
        )
          throw cause;
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
    const tools = { ...config.tools };
    for (const key of Object.keys(tools) as Tool[]) {
      if (!tools[key]) continue;
      signal.throwIfAborted();
      try {
        const path = await realpath(tools[key]);
        const info = await lstat(path);
        if (
          !info.isFile() ||
          info.isSymbolicLink() ||
          ![...system, ...config.roots].some((root) => within(root, path))
        )
          throw new Error("outside installation roots");
        await access(path, constants.X_OK);
        tools[key] = path;
      } catch {
        throw new Refusal(
          "engine_configuration",
          `${SETTINGS[key]} must identify an executable inside an admitted installation root.`,
        );
      }
    }
    const confinement =
      process.platform === "linux"
        ? ["/usr/bin/bwrap", "/usr/bin/prlimit"]
        : ["/usr/bin/sandbox-exec", DARWIN_SHELL, "/bin/ps"];
    for (const path of confinement) {
      try {
        await access(path, constants.X_OK);
      } catch {
        throw new Refusal(
          "engine_unavailable",
          "The required native confinement or resource-limit tool is unavailable.",
        );
      }
    }
    return {
      ...tools,
      office: tools.office
        ? await Office.load(tools.office, [...system, ...config.roots], signal)
        : undefined,
    };
  }

  async #directory(signal: AbortSignal): Promise<PrivateDirectory> {
    signal.throwIfAborted();
    const base = join(this.#data, "engine");
    try {
      await privateDirectory(base);
    } catch (cause) {
      if (!(
        cause instanceof Error &&
        "code" in cause &&
        cause.code === "EEXIST"
      ))
        throw cause;
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
      const profile = join(directory.home, "libreoffice");
      await privateDirectory(profile);
      await privateDirectory(join(profile, "user"));
      await writeFile(
        join(profile, "user", "registrymodifications.xcu"),
        OFFICE_PROFILE,
        { mode: 0o600, flag: "wx" },
      );
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

  async #office(
    directory: PrivateDirectory,
    office: Office,
    source: string,
    format: "pdf" | "xlsx",
    signal: AbortSignal,
  ) {
    const output = await this.#command(
      directory,
      office.executable,
      office.arguments(directory, source, format),
      signal,
    );
    office.verify(output.stdout);
  }
}
