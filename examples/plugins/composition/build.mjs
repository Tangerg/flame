import { rolldown } from "rolldown";
import { buildComponent } from "@flame/plugin-sdk/build";
import { mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const root = import.meta.dirname;
const ns = "io.github.tangerg.flame";
const check = process.argv.includes("--check");
const scratch = await mkdtemp(join(tmpdir(), "flame-composition-build-"));
try {
  for (const name of ["consumer", "provider"]) {
    const target = join(root, name, ns, "components");
    const output = check ? join(scratch, name) : target;
    await buildComponent({
      compiler: rolldown,
      input: join(root, "src", `${name}.tsx`),
      directory: output,
    });
    if (check) {
      const files = await readdir(output);
      if (
        JSON.stringify(files.sort()) !==
        JSON.stringify((await readdir(target)).sort())
      )
        throw new Error(`Component output differs: ${name}`);
      for (const file of files)
        if (
          !(await readFile(join(output, file))).equals(
            await readFile(join(target, file)),
          )
        )
          throw new Error(`Component bytes differ: ${name}/${file}`);
    }
  }
  for (const name of ["panels", "frames", "summary", "details"]) {
    const owner = name === "details" ? "provider" : "consumer";
    const consumer = owner === "consumer" ? "provider" : "consumer";
    const bytes = await readFile(
      join(root, owner, ns, "contracts", `${name}.json`),
    );
    const target = join(root, consumer, ns, "contracts", `${name}.json`);
    if (check) {
      if (!bytes.equals(await readFile(target)))
        throw new Error(`Contract projection differs: ${name}`);
    } else await writeFile(target, bytes);
  }
} finally {
  await rm(scratch, { recursive: true, force: true });
}
