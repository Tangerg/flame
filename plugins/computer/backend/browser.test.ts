import {
  chmod,
  cp,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { validateBrowser } from "./browser";

const temporary: string[] = [];
afterEach(async () => {
  for (const path of temporary.splice(0))
    await rm(path, { recursive: true, force: true });
});

async function fixture(platform: "linux" | "darwin" = "linux") {
  const root = await mkdtemp(join(tmpdir(), "flame-browser-"));
  temporary.push(root);
  const resources = join(root, "app/resources");
  const plugin = join(resources, "plugins/openai-bundled/plugins/chrome");
  const node = join(resources, "cua_node/bin/node");
  const nodeRepl = join(resources, "cua_node/bin/node_repl");
  const codex = join(resources, "codex");
  const arch = "arm64" as const;
  const relativeHost = join(
    "extension-host",
    platform === "darwin" ? "macos" : "linux",
    arch,
    platform === "darwin" ? "ChatGPT for Chrome" : "extension-host",
  );
  const host = join(plugin, relativeHost);
  const client = join(plugin, "scripts/browser-client.mjs");
  const home = join(root, "home");
  const registration = join(
    home,
    platform === "darwin"
      ? "Library/Application Support/Google/Chrome"
      : ".config/google-chrome",
    "NativeMessagingHosts/com.openai.codexextension.json",
  );
  const signal = new AbortController().signal;
  async function put(path: string, text: string) {
    await mkdir(dirname(path), { recursive: true });
    await writeFile(path, text);
  }
  for (const path of [node, nodeRepl, codex, host, client])
    await put(path, `original ${path.split("/").at(-1)}`);
  await chmod(host, 0o755);
  const information = {
    extensionHostName: "com.openai.codexextension",
    extensionIds: ["a".repeat(32)],
    browserDiagnostics: [
      {
        browserFamily: "chrome",
        linux: {
          nativeMessagingManifestDirectories: [
            ".config/google-chrome/NativeMessagingHosts",
            ".config/chromium/NativeMessagingHosts",
          ],
        },
        macos: {
          nativeMessagingManifestDirectories: [
            "Library/Application Support/Google/Chrome/NativeMessagingHosts",
          ],
        },
      },
    ],
  };
  await put(
    join(plugin, "scripts/extension-ids.json"),
    JSON.stringify(information),
  );
  const config = {
    schemaVersion: 1,
    channel: "prod",
    browserClientPath: client,
    codexCliPath: codex,
    nodePath: node,
    nodeReplPath: nodeRepl,
    proxyHost: "127.0.0.1",
    proxyPort: 0,
  };
  const manifest = {
    name: information.extensionHostName,
    type: "stdio",
    path: host,
    allowed_origins: [`chrome-extension://${information.extensionIds[0]}/`],
  };
  const configPath = join(dirname(host), "extension-host-config.json");
  await put(configPath, JSON.stringify(config));
  await put(registration, JSON.stringify(manifest));
  const application = { resources, node, nodeRepl, codex, arch, platform };
  const environment = { HOME: home };
  return {
    root,
    plugin,
    application,
    environment,
    config,
    configPath,
    manifest,
    registration,
    information,
    relativeHost,
    signal,
    put,
  };
}

describe("direct CUA browser connector", () => {
  it.each(["linux", "darwin"] as const)(
    "accepts the original %s connector without changing registration or account files",
    async (platform) => {
      const f = await fixture(platform);
      const before = await Promise.all([
        readFile(f.registration),
        readFile(f.configPath),
      ]);
      await expect(
        validateBrowser(f.application, f.environment, f.signal),
      ).resolves.toBeUndefined();
      expect(
        await Promise.all([readFile(f.registration), readFile(f.configPath)]),
      ).toEqual(before);
    },
  );

  it("accepts a complete byte-identical official connector cache with the selected engine paths", async () => {
    const f = await fixture();
    const copy = join(f.root, "official-cache/chrome");
    await cp(f.plugin, copy, { recursive: true });
    const host = join(copy, f.relativeHost);
    await f.put(
      join(dirname(host), "extension-host-config.json"),
      JSON.stringify({
        ...f.config,
        browserClientPath: join(copy, "scripts/browser-client.mjs"),
      }),
    );
    await f.put(f.registration, JSON.stringify({ ...f.manifest, path: host }));
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).resolves.toBeUndefined();
    await f.put(join(copy, "scripts/extra-runtime.mjs"), "unselected code");
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("restore the original connector");
  });

  it("refuses a relay executable even when it reports the original native-host name", async () => {
    const f = await fixture();
    const relay = join(f.root, "other/native-relay");
    await f.put(relay, "relay that changes browser capabilities");
    await f.put(f.registration, JSON.stringify({ ...f.manifest, path: relay }));
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("restore the original connector");
    expect(JSON.parse(await readFile(f.registration, "utf8")).path).toBe(relay);
  });

  it("checks every registered manifest from original metadata instead of accepting the first valid host", async () => {
    const f = await fixture();
    const relay = join(f.root, "other/native-relay");
    await f.put(relay, "relay");
    await f.put(
      join(
        f.environment.HOME,
        ".config/chromium/NativeMessagingHosts/com.openai.codexextension.json",
      ),
      JSON.stringify({ ...f.manifest, path: relay }),
    );
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("restore the original connector");
  });

  it("requires a coherent selected CUA installation in the native host configuration", async () => {
    const f = await fixture();
    const other = join(f.root, "different-engine/node");
    await f.put(other, "another node");
    await f.put(f.configPath, JSON.stringify({ ...f.config, nodePath: other }));
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("restore the original connector");
  });

  it("refuses additional extension origins and unknown host configuration semantics", async () => {
    const f = await fixture();
    await f.put(
      f.registration,
      JSON.stringify({
        ...f.manifest,
        allowed_origins: [
          ...f.manifest.allowed_origins,
          `chrome-extension://${"b".repeat(32)}/`,
        ],
      }),
    );
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("restore the original connector");
    await f.put(f.registration, JSON.stringify(f.manifest));
    await f.put(
      f.configPath,
      JSON.stringify({ ...f.config, skipSitePermissions: true }),
    );
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("restore the original connector");
  });

  it("uses the configured desktop account config root without changing it", async () => {
    const f = await fixture();
    const configRoot = join(f.root, "chrome-config");
    const target = join(
      configRoot,
      "google-chrome/NativeMessagingHosts/com.openai.codexextension.json",
    );
    await f.put(target, JSON.stringify(f.manifest));
    await rm(f.registration);
    await expect(
      validateBrowser(
        f.application,
        { ...f.environment, CHROME_CONFIG_HOME: configRoot },
        f.signal,
      ),
    ).resolves.toBeUndefined();
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("configure the official ChatGPT browser extension");
  });

  it("accepts ordinary official latest links without treating a path spelling as another engine", async () => {
    const f = await fixture();
    const nodeAlias = join(f.root, "selected-node");
    await symlink(f.application.node, nodeAlias);
    await f.put(
      f.configPath,
      JSON.stringify({ ...f.config, nodePath: nodeAlias }),
    );
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).resolves.toBeUndefined();
  });

  it("refuses missing or malformed setup without creating a registration", async () => {
    const f = await fixture();
    await rm(f.registration);
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("configure the official ChatGPT browser extension");
    await expect(readFile(f.registration)).rejects.toMatchObject({
      code: "ENOENT",
    });
    await f.put(f.registration, "not json");
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("could not be inspected");
  });

  it("refuses traversal in original metadata and bounds every inspected JSON resource", async () => {
    const f = await fixture();
    const metadataPath = join(f.plugin, "scripts/extension-ids.json");
    const malicious = structuredClone(f.information);
    malicious.browserDiagnostics[0]!.linux.nativeMessagingManifestDirectories =
      [".config/../../other"];
    await f.put(metadataPath, JSON.stringify(malicious));
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("invalid directory");
    await f.put(metadataPath, JSON.stringify(f.information));
    await f.put(f.registration, " ".repeat(256 * 1024 + 1));
    await expect(
      validateBrowser(f.application, f.environment, f.signal),
    ).rejects.toThrow("could not be inspected");
  });

  it("honors cancellation before accessing browser setup", async () => {
    const f = await fixture();
    const cancelled = AbortSignal.abort(new Error("test cancelled"));
    await expect(
      validateBrowser(f.application, f.environment, cancelled),
    ).rejects.toThrow("missing its original browser connector metadata");
    expect(JSON.parse(await readFile(f.registration, "utf8"))).toEqual(
      f.manifest,
    );
  });
});
