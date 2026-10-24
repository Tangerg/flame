import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  access,
  mkdir,
  mkdtemp,
  readFile,
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
  const destination = join(output, "runtimes/node", key);
  if (await exists(destination)) {
    if (
      hash(await readFile(join(destination, target.executable))) !==
      target.executableSHA256
    )
      throw new Error(
        "existing Node deployment differs; replace the quiescent distribution as a unit",
      );
    return join(destination, target.executable);
  }
  await mkdir(cache, { recursive: true });
  const archive = join(cache, target.archive);
  if (!(await exists(archive))) {
    if (offline)
      throw new Error(
        `missing prepared Node artifact ${target.archive}; run make setup`,
      );
    const response = await fetch(
      `https://nodejs.org/dist/v${release.version}/${target.archive}`,
      { redirect: "error", signal: AbortSignal.timeout(60_000) },
    );
    if (!response.ok)
      throw new Error(`Node artifact download failed (${response.status})`);
    const bytes = Buffer.from(await response.arrayBuffer());
    if (hash(bytes) !== target.archiveSHA256)
      throw new Error("downloaded Node artifact checksum mismatch");
    await writeFile(archive, bytes, { flag: "wx" });
  }
  if (hash(await readFile(archive)) !== target.archiveSHA256)
    throw new Error("prepared Node artifact checksum mismatch");
  await mkdir(dirname(destination), { recursive: true });
  const stage = await mkdtemp(join(dirname(destination), ".node-stage-"));
  try {
    execFileSync("tar", ["-xzf", archive, "--strip-components=1", "-C", stage]);
    if (
      hash(await readFile(join(stage, target.executable))) !==
      target.executableSHA256
    )
      throw new Error("extracted Node executable checksum mismatch");
    await rename(stage, destination);
  } finally {
    await rm(stage, { recursive: true, force: true });
  }
  return join(destination, target.executable);
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
