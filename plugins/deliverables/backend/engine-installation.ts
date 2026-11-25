import { constants } from "node:fs";
import { access, lstat, realpath } from "node:fs/promises";
import { isAbsolute, relative, resolve, sep } from "node:path";
import { DARWIN_SHELL } from "./confinement";
import { Office } from "./office";
import { refusal } from "./refusal";

export type Tool = "office" | "pdfinfo" | "pdftoppm" | "pdftotext";

export type InstalledTools = Omit<Record<Tool, string>, "office"> & {
  office?: Office;
};

export interface Configuration {
  tools: Record<Tool, string>;
  roots: string[];
  issues: string[];
}

export const SETTINGS: Record<Tool, string> = {
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

export function within(root: string, path: string): boolean {
  const suffix = relative(root, path);
  return (
    suffix === "" ||
    (!suffix.startsWith(`..${sep}`) && suffix !== ".." && !isAbsolute(suffix))
  );
}

export function absolute(value: string): boolean {
  return (
    isAbsolute(value) &&
    resolve(value) === value &&
    !Array.from(value).some(
      (character) =>
        character.codePointAt(0)! < 32 || character.codePointAt(0) === 127,
    )
  );
}

export function configuration(environment: NodeJS.ProcessEnv): Configuration {
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

export function hasErrorCode(cause: unknown, ...codes: string[]): boolean {
  return (
    cause instanceof Error &&
    "code" in cause &&
    codes.includes(cause.code as string)
  );
}

export async function installedExecutable(
  configured: string,
  roots: readonly string[],
): Promise<string | undefined> {
  try {
    const path = await realpath(configured);
    const info = await lstat(path);
    if (
      !info.isFile() ||
      info.isSymbolicLink() ||
      !roots.some((root) => within(root, path))
    )
      return undefined;
    await access(path, constants.X_OK);
    return path;
  } catch {
    return undefined;
  }
}

export async function requireConfinement(): Promise<void> {
  const confinement =
    process.platform === "linux"
      ? ["/usr/bin/bwrap", "/usr/bin/prlimit"]
      : ["/usr/bin/sandbox-exec", DARWIN_SHELL, "/bin/ps"];
  for (const path of confinement) {
    try {
      await access(path, constants.X_OK);
    } catch {
      throw refusal(
        "engine_unavailable",
        "The required native confinement or resource-limit tool is unavailable.",
      );
    }
  }
}
