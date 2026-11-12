import { execFile } from "node:child_process";
import { chmod, mkdtemp, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { promisify } from "node:util";
import type { TestProject } from "vitest/node";

declare module "vitest" {
  export interface ProvidedContext {
    runtimeHttpEnvironmentRoot: string;
  }
}

const execFileAsync = promisify(execFile);

export default async function prepareRuntimeHttp(project: TestProject) {
  const environmentRoot = await mkdtemp(join(tmpdir(), "flame-runtime-e2e-"));
  const runtimeDirectory = resolve(project.config.root, "../../runtime");
  const os = process.platform === "win32" ? "windows" : process.platform;
  const arch = process.arch === "x64" ? "amd64" : process.arch;
  const reclaim = async () => {
    const entries = await readdir(environmentRoot, { recursive: true, withFileTypes: true });
    await Promise.all(
      entries
        .filter((entry) => entry.isDirectory())
        .map((entry) => chmod(join(entry.parentPath, entry.name), 0o700)),
    );
    await rm(environmentRoot, { recursive: true, force: true });
  };

  try {
    await execFileAsync(process.execPath, [
      resolve(runtimeDirectory, "../tooling/node-companion.mjs"),
      "--os",
      os,
      "--arch",
      arch,
      "--output",
      environmentRoot,
      "--offline",
    ]);
    await execFileAsync(
      "go",
      ["build", "-o", join(environmentRoot, "flame-e2e"), "./cmd/flame-runtime"],
      { cwd: runtimeDirectory },
    );
    await execFileAsync(
      "go",
      [
        "build",
        "-o",
        join(environmentRoot, "frame-runner"),
        "github.com/Tangerg/frame/cmd/frame-runner",
      ],
      { cwd: runtimeDirectory },
    );
    await execFileAsync("go", ["build", "-o", join(environmentRoot, "flame-cli-e2e"), "."], {
      cwd: resolve(runtimeDirectory, "../apps/cli"),
      env: { ...process.env, GOWORK: resolve(runtimeDirectory, "../go.work") },
    });
    await execFileAsync(process.execPath, [
      resolve(runtimeDirectory, "../plugins/scripts/build-packages.mjs"),
      "--os",
      os,
      "--arch",
      arch,
      "--output",
      join(environmentRoot, "plugins"),
    ]);
    project.provide("runtimeHttpEnvironmentRoot", environmentRoot);
    return reclaim;
  } catch (failure) {
    try {
      await reclaim();
    } catch (cleanupFailure) {
      throw new AggregateError(
        [failure, cleanupFailure],
        "runtime HTTP fixture preparation failed",
      );
    }
    throw failure;
  }
}
