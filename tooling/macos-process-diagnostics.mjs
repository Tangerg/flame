import { execFile } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { promisify } from "node:util";
import { setTimeout as delay } from "node:timers/promises";

const execute = promisify(execFile);
const output = process.argv[2];
if (process.platform !== "darwin" || !output)
  throw new Error("macOS process diagnostics require an output directory");
await mkdir(output, { recursive: true });
const sampled = new Set();
for (;;) {
  const { stdout } = await execute("/bin/ps", [
    "-axo",
    "pid,ppid,rss,etime,command",
  ]);
  const processes = stdout
    .split("\n")
    .filter((line) =>
      /\/(?:qimu-runner|dsoffice)(?:\s|$)|\/runtimes\/node\/.+\/bin\/node/.test(
        line,
      ),
    );
  await writeFile(join(output, "latest-processes.txt"), processes.join("\n"));
  for (const line of processes) {
    const [pid, , rss, elapsed] = line.trim().split(/\s+/);
    const seconds = elapsed
      .split(":")
      .reduce((total, part) => total * 60 + Number(part), 0);
    if (sampled.has(pid) || Number(rss) > 1024 || seconds < 2) continue;
    sampled.add(pid);
    await writeFile(join(output, `${pid}-process.txt`), line);
    try {
      await execute(
        "/usr/bin/sample",
        [pid, "2", "-file", join(output, `${pid}-sample.txt`)],
        { timeout: 10_000 },
      );
    } catch (error) {
      await writeFile(join(output, `${pid}-sample-error.txt`), String(error));
    }
  }
  await delay(1000);
}
