import { execFile } from "node:child_process";
import { mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs, promisify } from "node:util";
import { stageNode } from "./node-companion.mjs";

const repository = fileURLToPath(new URL("../", import.meta.url));
const execute = promisify(execFile);

// The Desktop stays a client. Its complete distribution also ships a separately
// started Runtime and the exact companion layout that Runtime already owns.
export async function stageDesktopCompanions({ arch, output }) {
  if (!["amd64", "arm64"].includes(arch))
    throw new Error(
      "Desktop distributions require one explicit amd64 or arm64 architecture; universal companion packages are not supported",
    );
  if (!output) throw new Error("Desktop companion output is required");
  const destination = resolve(output);
  await mkdir(destination, { recursive: true });
  const target = {
    ...process.env,
    GOOS: "darwin",
    GOARCH: arch,
    CGO_ENABLED: "0",
  };
  await stageNode({
    os: "darwin",
    arch,
    output: destination,
    offline: true,
  });
  await execute(
    "go",
    [
      "build",
      "-trimpath",
      "-o",
      join(destination, "qimu-runner"),
      "./runner/cmd/qimu-runner",
    ],
    { cwd: join(repository, "qimu"), env: target, maxBuffer: 32 * 1024 * 1024 },
  );
  await execute(
    "go",
    [
      "build",
      "-trimpath",
      "-o",
      join(destination, "flame-runtime"),
      "./cmd/flame-runtime",
    ],
    {
      cwd: join(repository, "runtime"),
      env: target,
      maxBuffer: 32 * 1024 * 1024,
    },
  );
  await execute(
    process.execPath,
    [
      join(repository, "plugins/scripts/build-packages.mjs"),
      "--os",
      "darwin",
      "--arch",
      arch,
      "--output",
      join(destination, "plugins"),
      "--replace",
    ],
    {
      cwd: repository,
      env: { ...process.env, GOOS: "", GOARCH: "" },
      maxBuffer: 32 * 1024 * 1024,
    },
  );
}

if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const { values } = parseArgs({
    options: { arch: { type: "string" }, output: { type: "string" } },
  });
  await stageDesktopCompanions(values);
}
