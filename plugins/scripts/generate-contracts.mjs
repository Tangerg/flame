import { access, readdir } from "node:fs/promises";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { generateContracts } from "./contracts.mjs";

export async function generateWorkspaceContracts(root, tool) {
  for (const directory of await readdir(join(root, "plugins"), {
    withFileTypes: true,
  })) {
    if (!directory.isDirectory()) continue;
    const project = join(root, "plugins", directory.name);
    try {
      await access(join(project, "package/plugin.json"));
    } catch (error) {
      if (error.code === "ENOENT") continue;
      throw error;
    }
    await generateContracts(project, tool);
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const root = fileURLToPath(new URL("../../", import.meta.url));
  await generateWorkspaceContracts(root, {
    command: "go",
    args: ["run", "./cmd/qimu"],
    cwd: join(root, "qimu"),
  });
}
