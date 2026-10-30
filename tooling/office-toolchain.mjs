import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { constants, createReadStream } from "node:fs";
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
    new URL(
      "../plugins/deliverables/backend/office-release.json",
      import.meta.url,
    ),
    "utf8",
  ),
);

async function exists(path) {
  try {
    await access(path);
    return true;
  } catch (error) {
    if (error.code === "ENOENT") return false;
    throw error;
  }
}

async function hash(path, algorithm = "sha256", encoding = "hex") {
  const digest = createHash(algorithm);
  for await (const chunk of createReadStream(path)) digest.update(chunk);
  return digest.digest(encoding);
}

async function verify(directory, target) {
  if ((await realpath(directory)) !== directory)
    throw new Error("Office installation must be canonical");
  const manifest = join(directory, "prebuilds.json");
  if (
    (await lstat(manifest)).isSymbolicLink() ||
    (await hash(manifest)) !== target.manifestSHA256
  )
    throw new Error("Office release manifest checksum mismatch");
  const { files } = JSON.parse(await readFile(manifest, "utf8"));
  for (const [name, checksum] of Object.entries(files)) {
    const file = join(directory, name);
    if (
      (await realpath(file)) !== file ||
      !(await lstat(file)).isFile() ||
      (await hash(file)) !== checksum
    )
      throw new Error(`Office installation checksum mismatch: ${name}`);
  }
  await access(join(directory, release.executable), constants.X_OK);
}

export async function stageOffice({
  arch = process.arch,
  output,
  offline = false,
  cache = join(root, "out/office-artifacts"),
}) {
  const target = release.targets[arch];
  if (!target)
    throw new Error(`macOS Office has no selected artifact for ${arch}`);
  if (!output || resolve(output) !== output)
    throw new Error("Office output must be absolute");
  await mkdir(output, { recursive: true });
  const destination = join(
    await realpath(output),
    `libreoffice-kit-${release.version}-darwin-${arch}`,
  );
  if (await exists(destination)) {
    await verify(destination, target);
    return join(destination, release.executable);
  }
  await mkdir(cache, { recursive: true });
  const archive = join(cache, new URL(target.url).pathname.split("/").at(-1));
  if (!(await exists(archive))) {
    if (offline)
      throw new Error(`missing prepared Office artifact: ${archive}`);
    const response = await fetch(target.url, {
      redirect: "error",
      signal: AbortSignal.timeout(120_000),
    });
    if (!response.ok)
      throw new Error(`Office artifact download failed (${response.status})`);
    const bytes = Buffer.from(await response.arrayBuffer());
    if (
      `sha512-${createHash("sha512").update(bytes).digest("base64")}` !==
      target.integrity
    )
      throw new Error("downloaded Office artifact checksum mismatch");
    const download = await mkdtemp(join(cache, ".office-download-"));
    try {
      await writeFile(join(download, "archive"), bytes, { flag: "wx" });
      await rename(join(download, "archive"), archive);
    } finally {
      await rm(download, { recursive: true, force: true });
    }
  }
  if (`sha512-${await hash(archive, "sha512", "base64")}` !== target.integrity)
    throw new Error("prepared Office artifact checksum mismatch");
  const stage = await mkdtemp(join(dirname(destination), ".office-stage-"));
  try {
    execFileSync("tar", [
      "-xzf",
      archive,
      "--no-same-owner",
      "--strip-components=1",
      "-C",
      stage,
    ]);
    await verify(stage, target);
    await rename(stage, destination);
  } finally {
    await rm(stage, { recursive: true, force: true });
  }
  return join(destination, release.executable);
}

if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const { values } = parseArgs({
    options: {
      arch: { type: "string" },
      output: { type: "string" },
      offline: { type: "boolean" },
    },
  });
  console.log(await stageOffice(values));
}
