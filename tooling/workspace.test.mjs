import { execFileSync } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { strict as assert } from "node:assert";
import { test } from "node:test";
import { moduleArtifacts, root } from "./workspace.mjs";

test("workspace coordination loads before npm dependencies are installed", async () => {
  const repository = await mkdtemp(join(tmpdir(), "flame-clean-setup-"));
  try {
    await mkdir(join(repository, "packages/local-runtime"), {
      recursive: true,
    });
    await cp(join(root, "tooling"), join(repository, "tooling"), {
      recursive: true,
    });
    await cp(
      join(root, "packages/local-runtime/node-release.json"),
      join(repository, "packages/local-runtime/node-release.json"),
    );
    const probe = join(repository, "probe.mjs");
    await writeFile(probe, 'import "./tooling/workspace.mjs";\n');
    execFileSync(process.execPath, [probe], {
      cwd: repository,
      env: { ...process.env, NODE_PATH: "" },
      encoding: "utf8",
    });
  } finally {
    await rm(repository, { recursive: true, force: true });
  }
});

test("module artifacts use pinned dependency graphs and frozen tool source", async () => {
  const repository = await mkdtemp(join(tmpdir(), "flame-candidate-closure-"));
  const git = (...args) =>
    execFileSync("git", args, { cwd: repository, encoding: "utf8" }).trim();
  const write = async (path, source) => {
    const target = join(repository, path);
    await mkdir(join(target, ".."), { recursive: true });
    await writeFile(target, source);
  };
  const commit = (message) => {
    git("add", "--all");
    git(
      "-c",
      "user.name=Candidate Test",
      "-c",
      "user.email=candidate@example.invalid",
      "commit",
      "-q",
      "-m",
      message,
    );
    return git("rev-parse", "HEAD");
  };
  try {
    git("init", "-q");
    const tooling = JSON.parse(
      execFileSync("go", ["mod", "edit", "-json"], {
        cwd: join(root, "tooling"),
      }),
    );
    const mod = tooling.Require.find(
      (dependency) => dependency.Path === "golang.org/x/mod",
    );
    await write(
      "go.work",
      `go ${tooling.Go}\nuse (\n ./a\n ./b\n ./tooling\n)\n`,
    );
    await write(
      "a/go.mod",
      `module example.com/a\ngo ${tooling.Go}\nrequire example.com/b v1.0.0\n`,
    );
    await write("a/a.go", "package a\n");
    await write("b/go.mod", `module example.com/b\ngo ${tooling.Go}\n`);
    await write("b/b.go", 'package b\nconst Value = "first"\n');
    await write(
      "tooling/go.mod",
      `module example.com/tooling\ngo ${tooling.Go}\nrequire golang.org/x/mod ${mod.Version}\n`,
    );
    await cp(join(root, "tooling/go.sum"), join(repository, "tooling/go.sum"));
    await cp(
      join(root, "tooling/moduleproxy"),
      join(repository, "tooling/moduleproxy"),
      { recursive: true },
    );
    const first = commit("first immutable module sources");
    await write(
      "a/go.mod",
      `module example.com/a\ngo ${tooling.Go}\nrequire example.com/b v1.1.0\n`,
    );
    await write("b/b.go", 'package b\nconst Value = "second"\n');
    const second = commit("later working graph");
    await write(
      "module-candidates.json",
      JSON.stringify({
        "example.com/a": { "v1.0.0": first },
        "example.com/b": { "v1.0.0": first, "v1.1.0": second },
      }),
    );
    const coordination = commit("freeze candidate coordination");
    await write(
      "tooling/moduleproxy/main.go",
      "this mutable checkout must never be compiled\n",
    );
    await write("b/go.mod", "this mutable manifest must never be read\n");
    const artifacts = await moduleArtifacts({
      repository,
      revision: coordination,
      output: join(repository, "proxy"),
      offline: true,
      targets: [
        { path: "example.com/a", version: "v1.0.0" },
        { path: "example.com/b", version: "v1.1.0" },
      ],
    });
    assert.deepEqual(Object.keys(artifacts.modules).sort(), [
      "example.com/a@v1.0.0",
      "example.com/b@v1.0.0",
      "example.com/b@v1.1.0",
    ]);
    assert.equal(artifacts.modules["example.com/b@v1.0.0"].revision, first);
    assert.equal(artifacts.modules["example.com/b@v1.1.0"].revision, second);
    assert.match(
      await readFile(
        join(repository, "proxy/example.com/a/@v/v1.0.0.mod"),
        "utf8",
      ),
      /example.com\/b v1\.0\.0/,
    );
  } finally {
    await rm(repository, { recursive: true, force: true });
  }
});
