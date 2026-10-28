import {
  chmodSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { Installation } from "./installation";

const directories: string[] = [];
afterEach(() => {
  for (const path of directories.splice(0))
    rmSync(path, { recursive: true, force: true });
});

function archive(value: unknown): Buffer {
  const body = Buffer.from(JSON.stringify(value));
  const header = Buffer.from(
    JSON.stringify({
      files: { "package.json": { offset: "0", size: body.length } },
    }),
  );
  const padding = (4 - (header.length % 4)) % 4;
  const prefix = Buffer.alloc(16);
  prefix.writeUInt32LE(4, 0);
  prefix.writeUInt32LE(8 + header.length + padding, 4);
  prefix.writeUInt32LE(4 + header.length + padding, 8);
  prefix.writeUInt32LE(header.length, 12);
  return Buffer.concat([prefix, header, Buffer.alloc(padding), body]);
}

function fixture() {
  const path = mkdtempSync(join(tmpdir(), "flame-cua-installation-"));
  directories.push(path);
  const write = (
    name: string,
    value: string | Buffer = "fixture",
    mode = 0o644,
  ) => {
    const file = join(path, name);
    mkdirSync(dirname(file), { recursive: true, mode: 0o755 });
    writeFileSync(file, value, { mode });
    return file;
  };
  for (const name of [
    "resources/cua_node/bin/node",
    "resources/cua_node/bin/node_repl",
    "resources/codex",
    "resources/codex-code-mode-host",
  ])
    write(name, "fixture", 0o755);
  const manifest = write(
    "resources/cua_node/manifest.json",
    JSON.stringify({
      platform: "linux",
      arch: process.arch,
      runtime_archive_version: "fixture/1",
      node_path: "bin/node",
      node_repl_path: "bin/node_repl",
      node_modules: "lib/node_modules",
    }),
  );
  const entry = write(
    "resources/cua_node/lib/node_modules/@oai/cua-repl/bin/cua-repl.mjs",
  );
  write(
    "resources/cua_node/lib/node_modules/@oai/sky/dist/project/cua/sky_js/src/service.js",
  );
  write(
    "resources/plugins/openai-bundled/plugins/unified-computer-use/.mcp.json",
    JSON.stringify({
      mcpServers: {
        cua_repl: { enabled_tools: ["js", "js_reset", "turn_ended"] },
      },
    }),
  );
  write(
    "resources/app.asar",
    archive({ name: "openai-codex-electron", version: "fixture" }),
  );
  return { path, write, manifest, entry };
}

describe.skipIf(process.platform !== "linux")(
  "installed Linux CUA dependency",
  () => {
    it("selects the original CUA entry and captures its exact installed generation", async () => {
      const { path } = fixture();
      const selected = await Installation.inspect(
        path,
        new AbortController().signal,
      );
      expect(selected.version).toBe("fixture");
      expect(selected.runtimeVersion).toBe("fixture/1");
      expect(selected.entry).toBe(
        join(
          path,
          "resources/cua_node/lib/node_modules/@oai/cua-repl/bin/cua-repl.mjs",
        ),
      );
      expect(() => selected.assertCurrent()).not.toThrow();
    });

    it.each(["replace", "add", "remove"])(
      "retires a generation after an application %s",
      async (change) => {
        const { path, entry, write } = fixture();
        const selected = await Installation.inspect(
          path,
          new AbortController().signal,
        );
        if (change === "replace") writeFileSync(entry, "updated application");
        else if (change === "add")
          write("resources/cua_node/lib/node_modules/added.js");
        else rmSync(entry);
        expect(() => selected.assertCurrent()).toThrow(
          "CUA application changed",
        );
      },
    );

    it("refuses mismatched architecture and executable layout ambiguity", async () => {
      const { path, manifest, write } = fixture();
      const original = JSON.parse(readFileSync(manifest, "utf8"));
      writeFileSync(manifest, JSON.stringify({ ...original, arch: "wrong" }));
      await expect(
        Installation.inspect(path, new AbortController().signal),
      ).rejects.toThrow("compatible CUA installation");
      writeFileSync(manifest, JSON.stringify(original));
      write("resources/codex-cli/bin/codex", "fixture", 0o755);
      write("resources/codex-cli/bin/codex-code-mode-host", "fixture", 0o755);
      await expect(
        Installation.inspect(path, new AbortController().signal),
      ).rejects.toThrow("one complete Codex tool layout");
    });

    it("refuses dependencies writable by other accounts and links outside the application", async () => {
      const { path, entry } = fixture();
      chmodSync(entry, 0o666);
      await expect(
        Installation.inspect(path, new AbortController().signal),
      ).rejects.toThrow("writable by other accounts");
      chmodSync(entry, 0o644);
      symlinkSync(process.execPath, join(dirname(entry), "outside"));
      await expect(
        Installation.inspect(path, new AbortController().signal),
      ).rejects.toThrow("outside the selected installation");
    });

    it("rejects a truncated archive before executing dependency code", async () => {
      const { path, write } = fixture();
      write("resources/app.asar", Buffer.alloc(4));
      await expect(
        Installation.inspect(path, new AbortController().signal),
      ).rejects.toThrow("archive is truncated");
    });

    it("honors cancellation before inspecting a dependency", async () => {
      const abort = new AbortController();
      abort.abort(new Error("cancelled before dependency inspection"));
      await expect(
        Installation.inspect("/unavailable", abort.signal),
      ).rejects.toThrow("cancelled before dependency inspection");
    });
  },
);
