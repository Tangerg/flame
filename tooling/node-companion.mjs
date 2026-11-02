import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  access,
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  rename,
  rm,
  writeFile,
} from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

const root = fileURLToPath(new URL("../", import.meta.url));
const release = JSON.parse(
  await readFile(
    new URL("../packages/local-runtime/node-release.json", import.meta.url),
    "utf8",
  ),
);
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
async function exists(path) {
  try {
    await access(path);
    return true;
  } catch (error) {
    if (error.code === "ENOENT") return false;
    throw error;
  }
}
async function verifyNode(directory, target) {
  const executable = join(directory, target.executable);
  const info = await lstat(executable);
  if ((await realpath(executable)) !== executable)
    throw new Error("Node deployment executable path is redirected");
  if (!info.isFile() || (info.mode & 0o111) === 0)
    throw new Error("Node deployment requires a regular executable file");
  if (hash(await readFile(executable)) !== target.executableSHA256)
    throw new Error("Node deployment executable checksum mismatch");
  return executable;
}

async function cacheArchive(archive, bytes) {
  const stage = await mkdtemp(join(dirname(archive), ".node-download-"));
  try {
    const temporary = join(stage, "archive");
    await writeFile(temporary, bytes);
    await rename(temporary, archive);
  } finally {
    await rm(stage, { recursive: true, force: true });
  }
}

async function verifiedArchive(cache, target, offline) {
  await mkdir(cache, { recursive: true });
  const archive = join(cache, target.archive);
  if (await exists(archive)) {
    if (hash(await readFile(archive)) === target.archiveSHA256) return archive;
    if (offline) throw new Error("prepared Node artifact checksum mismatch");
  } else {
    if (offline)
      throw new Error(
        `missing prepared Node artifact ${target.archive}; run make setup`,
      );
  }
  const response = await fetch(
    `https://nodejs.org/dist/v${release.version}/${target.archive}`,
    { redirect: "error", signal: AbortSignal.timeout(60_000) },
  );
  if (!response.ok)
    throw new Error(`Node artifact download failed (${response.status})`);
  const bytes = Buffer.from(await response.arrayBuffer());
  if (hash(bytes) !== target.archiveSHA256)
    throw new Error("downloaded Node artifact checksum mismatch");
  await cacheArchive(archive, bytes);
  return archive;
}

export async function stageNode({
  os,
  arch,
  output,
  offline = false,
  cache = join(root, "out/node-artifacts"),
}) {
  const key = `${os}-${arch}`,
    target = release.targets[key];
  if (!target)
    throw new Error(`managed Node has no selected artifact for ${key}`);
  if (!output || resolve(output) !== output)
    throw new Error("companion output must be absolute");
  await mkdir(output, { recursive: true });
  const distribution = await realpath(output);
  const runtimes = join(distribution, "runtimes");
  for (const directory of [runtimes, join(runtimes, "node")]) {
    await mkdir(directory, { recursive: true });
    if ((await realpath(directory)) !== directory)
      throw new Error("Node deployment directory is redirected");
  }
  const destination = join(runtimes, "node", key);
  if (await exists(destination)) return verifyNode(destination, target);
  const archive = await verifiedArchive(cache, target, offline);
  const stage = await mkdtemp(join(dirname(destination), ".node-stage-"));
  try {
    execFileSync("tar", [
      "-xzf",
      archive,
      "--no-same-owner",
      "--strip-components=1",
      "-C",
      stage,
    ]);
    await verifyNode(stage, target);
    try {
      await rename(stage, destination);
    } catch (error) {
      if (error.code !== "EEXIST" && error.code !== "ENOTEMPTY") throw error;
    }
  } finally {
    await rm(stage, { recursive: true, force: true });
  }
  return verifyNode(destination, target);
}
if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const { values } = parseArgs({
    options: {
      os: { type: "string" },
      arch: { type: "string" },
      output: { type: "string" },
      offline: { type: "boolean" },
    },
  });
  await stageNode(values);
}
