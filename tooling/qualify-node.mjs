import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { proxyEnvironment, root, run } from "./workspace.mjs";
import { stageNode } from "./node-companion.mjs";

const directory = await mkdtemp(join(tmpdir(), "flame-node-distribution-"));
const env = {
  ...proxyEnvironment(join(root, "out/module-proxy")),
  GOWORK: "off",
};
try {
  const target = JSON.parse(
    run("go", ["env", "-json", "GOOS", "GOARCH"], root, env, true),
  );
  await stageNode({
    os: target.GOOS,
    arch: target.GOARCH,
    output: directory,
    offline: true,
  });
  run(
    "node",
    [
      "plugins/scripts/build-packages.mjs",
      "--os",
      target.GOOS,
      "--arch",
      target.GOARCH,
      "--output",
      join(directory, "plugins"),
    ],
    root,
    env,
  );
  for (const [name, owner] of [
    ["flame-runtime", "./runtime/cmd/flame-runtime"],
    ["flame", "./apps/cli"],
    ["qualify", "./tooling/nodequalification"],
  ])
    run(
      "go",
      [
        "build",
        "-mod=readonly",
        "-trimpath",
        "-o",
        join(directory, name),
        owner,
      ],
      root,
      env,
    );
  run(
    join(directory, "qualify"),
    ["--distribution", directory],
    directory,
    env,
  );
} finally {
  await rm(directory, { recursive: true, force: true });
}
