import { execFileSync } from "node:child_process";
import { basename, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { readBuildPlan } from "./build.mjs";

const project = fileURLToPath(new URL("./", import.meta.url));
const { declaration } = await readBuildPlan(project);
const contracts = new Set(
  (declaration.contributions ?? []).map((value) => value.contract),
);
for (const resource of contracts) {
  execFileSync(
    join(
      project,
      "tools/contractgen" + (process.platform === "win32" ? ".exe" : ""),
    ),
    [
      "--feature",
      join(
        project,
        "contracts",
        basename(resource).replace(".generated.json", ".json"),
      ),
      "--out",
      dirname(join(project, "package", resource)),
      ...(declaration.components?.length
        ? ["--ts", join(project, "component")]
        : []),
    ],
    { cwd: project, stdio: "inherit" },
  );
}
