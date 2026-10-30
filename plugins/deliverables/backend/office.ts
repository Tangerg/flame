import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { lstat, readFile, realpath } from "node:fs/promises";
import { dirname, isAbsolute, join, relative, sep } from "node:path";
import { pathToFileURL } from "node:url";
import { Refusal } from "@qimu/node";
import { LIMITS } from "../contracts/limits";
import type { PrivateDirectory } from "./confinement";
import release from "./office-release.json";

export class Office {
  readonly executable: string;
  readonly identity?: { name: string; version: string };
  readonly #program?: string;

  private constructor(executable: string, program?: string) {
    this.executable = executable;
    this.#program = program;
    if (program)
      this.identity = {
        name: "LibreOffice",
        version: `LibreOfficeKit ${release.coreVersion} (@deepseek-ai/libreoffice-kit ${release.version})`,
      };
  }

  static async load(
    executable: string,
    roots: string[],
    signal: AbortSignal,
  ): Promise<Office> {
    if (process.platform !== "darwin") return new Office(executable);
    const target =
      release.targets[process.arch as keyof typeof release.targets];
    if (!target)
      throw new Refusal(
        "engine_unavailable",
        "The macOS Office toolchain is unavailable for this architecture.",
      );
    const root = dirname(dirname(executable));
    const program = join(root, release.programDirectory);
    try {
      if (
        join(root, release.executable) !== executable ||
        !roots.some((admitted) => {
          const suffix = relative(admitted, root);
          return (
            !isAbsolute(suffix) &&
            suffix !== ".." &&
            !suffix.startsWith(`..${sep}`)
          );
        }) ||
        (await realpath(root)) !== root ||
        (await realpath(program)) !== program ||
        !(await lstat(program)).isDirectory()
      )
        throw new Error("invalid installation layout");
      const manifestPath = join(root, "prebuilds.json");
      const entry = await lstat(manifestPath);
      if (!entry.isFile() || entry.isSymbolicLink())
        throw new Error("invalid release manifest");
      const manifest = await readFile(manifestPath, { signal });
      if (
        createHash("sha256").update(manifest).digest("hex") !==
        target.manifestSHA256
      )
        throw new Error("release manifest mismatch");
      const hash = createHash("sha256");
      for await (const chunk of createReadStream(executable, { signal }))
        hash.update(chunk);
      if (hash.digest("hex") !== target.executableSHA256)
        throw new Error("release executable mismatch");
    } catch {
      signal.throwIfAborted();
      throw new Refusal(
        "engine_configuration",
        "macOS requires the verified LibreOfficeKit installation produced by tooling/office-toolchain.mjs, with its entire directory admitted as a tool installation root.",
      );
    }
    return new Office(executable, program);
  }

  arguments(
    directory: PrivateDirectory,
    source: string,
    format: "pdf" | "xlsx",
  ): string[] {
    const profile = join(directory.home, "libreoffice");
    if (this.#program)
      return [
        "--program-directory",
        this.#program,
        "--input-path",
        source,
        "--output-path",
        join(directory.output, `source.${format}`),
        "--profile-directory",
        profile,
        "--max-output-bytes",
        String(LIMITS.fileBytes),
        // Keep embedded images at print resolution; page previews have their own bounds.
        "--max-image-resolution",
        "300",
        "--format",
        format,
        "--recalculate",
        format === "xlsx" ? "true" : "false",
      ];
    return [
      `-env:UserInstallation=${pathToFileURL(profile).href}`,
      "--headless",
      "--invisible",
      "--nodefault",
      "--nolockcheck",
      "--norestore",
      "--convert-to",
      format === "xlsx" ? "xlsx:Calc MS Excel 2007 XML" : format,
      "--outdir",
      directory.output,
      source,
    ];
  }

  verify(stdout: string): void {
    if (!this.#program) return;
    try {
      const response: unknown = JSON.parse(stdout);
      if (
        !response ||
        typeof response !== "object" ||
        !("ok" in response) ||
        response.ok !== true
      )
        throw new Error("unsuccessful conversion response");
    } catch {
      throw new Refusal(
        "engine_output_invalid",
        "LibreOfficeKit did not confirm the conversion.",
      );
    }
  }
}
