import { execFile } from "node:child_process";
import { mkdir, watch, writeFile } from "node:fs/promises";
import { delimiter, join } from "node:path";
import { promisify } from "node:util";
import type { useRuntimeFixture } from "./fixture";
import { within } from "./observation";

const exec = promisify(execFile);

export function gitIn(cwd: string) {
  return (...args: string[]) =>
    exec(
      "git",
      [
        "-c",
        "user.name=Receipt Fixture",
        "-c",
        "user.email=fixture@example.test",
        "-c",
        "core.hooksPath=/dev/null",
        "-c",
        "commit.gpgSign=false",
        ...args,
      ],
      { cwd },
    );
}

function shellWord(value: string) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

export async function crashAfterGit(
  fixture: ReturnType<typeof useRuntimeFixture>,
  command: "apply" | "remove" | "read-tree",
  invoke: () => Promise<unknown>,
  observeWrite: () => Promise<void>,
  afterExit?: () => Promise<void>,
) {
  const control = join(fixture.root, `${command}-git`);
  await mkdir(control);
  const git = (await exec("/bin/sh", ["-c", "command -v git"])).stdout.trim();
  const continued = join(control, "continue");
  const marker = "git-written";
  await writeFile(
    join(control, "git"),
    `#!/bin/sh
previous=
selected=
for argument do
  case "$argument" in
    apply) selected=apply ;;
    remove) if [ "$previous" = worktree ]; then selected=remove; fi ;;
    --reset) if [ "$previous" = read-tree ]; then selected=read-tree; fi ;;
  esac
  previous=$argument
done
if [ "$selected" != ${shellWord(command)} ]; then
  exec ${shellWord(git)} "$@"
fi
${shellWord(git)} "$@" || exit
: > ${shellWord(join(control, marker))}
while [ ! -f ${shellWord(continued)} ]; do sleep 0.02; done
`,
    { mode: 0o700 },
  );
  await fixture.client.close();
  await fixture.stopRuntimeProcess();
  await fixture.startRuntimeProcess({
    executableSearchPath: `${control}${delimiter}${process.env.PATH ?? ""}`,
  });
  fixture.client = fixture.createRuntimeClient();
  const cut = new AbortController();
  const events = watch(control, { signal: cut.signal });
  const written = (async () => {
    for await (const event of events) if (event.filename === marker) return;
    throw new Error("native Git did not reach the write boundary");
  })();
  const original = invoke().then(
    (response) => {
      throw new Error(`original request answered before termination: ${JSON.stringify(response)}`);
    },
    (cause) => cause,
  );
  try {
    await within(
      Promise.race([
        written,
        original.then((cause) => {
          throw cause;
        }),
      ]),
      "the completed native Git write",
    );
    await observeWrite();
    await fixture.client.close();
    await fixture.killRuntimeProcess();
    await within(original, "the disconnected original request");
    await afterExit?.();
  } finally {
    cut.abort();
    await writeFile(continued, "");
  }
}
