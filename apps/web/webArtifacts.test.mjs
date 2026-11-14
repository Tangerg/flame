import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { extname, join } from "node:path";
import { test } from "node:test";
import { build } from "vite";
import { parse } from "parse5";
import { stylexPlugin } from "./stylex.vite.mjs";
import { webArtifacts } from "./webArtifacts.vite.mjs";
import { NONCE_PLACEHOLDER, WEB_POLICY_FILE, documentResponse } from "./webPolicy.mjs";

const require = createRequire(import.meta.url);
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");

async function fixture(t, color = "#fb6e30", extra = [], cssCodeSplit = true) {
  const root = await mkdtemp(join(tmpdir(), "flame-web-policy-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  await mkdir(join(root, "public"));
  await Promise.all([
    writeFile(
      join(root, "index.html"),
      '<!doctype html><head><script src="/appearance.js"></script><script type="importmap">{"imports":{}}</script></head><body><script type="module" src="/main.jsx"></script></body>',
    ),
    writeFile(
      join(root, "main.jsx"),
      `import * as stylex from '@stylexjs/stylex'; import './base.css'; const styles = stylex.create({ proof: { color: '${color}' } }); document.body.className = stylex.props(styles.proof).className;`,
    ),
    writeFile(join(root, "base.css"), 'body{margin:0;background-image:url("./proof.svg")}'),
    writeFile(
      join(root, "proof.svg"),
      '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="7"><path d="M0 0h12v7H0z"/></svg>',
    ),
    writeFile(join(root, "public/appearance.js"), "document.documentElement.dataset.theme='light'"),
    ...["component", "fixture"].map((role) =>
      writeFile(
        join(root, `${role}.html`),
        `<!doctype html><html data-lab-document="${role}"><head></head><body><script type="module" src="/main.jsx"></script></body></html>`,
      ),
    ),
  ]);
  const output = await build({
    configFile: false,
    root,
    logLevel: "silent",
    plugins: [stylexPlugin(), webArtifacts(), ...extra],
    resolve: { alias: { "@stylexjs/stylex": require.resolve("@stylexjs/stylex") } },
    build: {
      write: false,
      minify: false,
      assetsInlineLimit: 0,
      cssCodeSplit,
      rolldownOptions: {
        input: ["index", "component", "fixture"].map((name) => join(root, `${name}.html`)),
      },
    },
  });
  assert(!Array.isArray(output));
  const files = Object.fromEntries(output.output.map((file) => [file.fileName, file]));
  return { files, policy: JSON.parse(files[WEB_POLICY_FILE].source) };
}

test("the built document and import-map tags use the response nonce placeholder", async (t) => {
  const { files, policy } = await fixture(t);
  const html = files["index.html"].source;
  const elements = [];
  const visit = (node) => {
    if (
      node.tagName === "script" ||
      node.tagName === "style" ||
      (node.tagName === "link" &&
        node.attrs.some(
          (attr) => attr.name === "rel" && ["stylesheet", "modulepreload"].includes(attr.value),
        ))
    )
      elements.push(node);
    for (const child of node.childNodes ?? []) visit(child);
  };
  visit(parse(html));
  assert(elements.length >= 3);
  for (const element of elements)
    assert.equal(element.attrs.find((attr) => attr.name === "nonce")?.value, NONCE_PLACEHOLDER);
  const first = documentResponse(
    policy,
    html,
    "first-response",
    policy.files["index.html"].document,
  );
  const second = documentResponse(
    policy,
    html,
    "second-response",
    policy.files["index.html"].document,
  );
  assert(!first.body.includes(NONCE_PLACEHOLDER));
  assert(first.body.includes('nonce="first-response"'));
  assert(
    first.headers["Content-Security-Policy"].includes("'nonce-first-response' 'strict-dynamic'"),
  );
  assert(!first.headers["Content-Security-Policy"].includes("script-src 'unsafe-inline'"));
  assert(!first.headers["Content-Security-Policy"].includes("'unsafe-eval'"));
  assert.equal(first.headers["X-Frame-Options"], "DENY");
  assert.equal(first.headers["Cache-Control"], "no-store");
  assert.notEqual(first.body, second.body);
  for (const role of ["component", "fixture"]) {
    const kind = policy.files[`${role}.html`].document;
    assert.equal(kind, "same-origin");
    const embedded = documentResponse(
      policy,
      files[`${role}.html`].source,
      "preview-response",
      kind,
    );
    assert.equal(embedded.headers["X-Frame-Options"], "SAMEORIGIN");
    assert(embedded.headers["Content-Security-Policy"].includes("frame-ancestors 'self'"));
    assert(embedded.headers["Content-Security-Policy"].includes("'strict-dynamic'"));
  }
});

test("fingerprints come from emitted asset bytes or actual bundler hash placeholders", async (t) => {
  const { files, policy } = await fixture(t);
  assert.equal(policy.version, 1);
  assert.equal(policy.files["appearance.js"], undefined);
  for (const [name, identity] of Object.entries(policy.files)) {
    const file = files[name];
    const bytes = file.type === "chunk" ? file.code : file.source;
    assert.equal(identity.sha256, hash(bytes));
    if (identity.fingerprint === "sha256")
      assert.equal(name, `assets/${identity.sha256}${extname(name)}`);
    if (identity.fingerprint === "bundler") {
      assert.equal(file.type, "chunk");
      assert.equal(typeof file.preliminaryFileName, "string");
      assert.match(file.preliminaryFileName, /!~\{[A-Za-z0-9_$]{1,17}\}~/);
      assert.notEqual(file.preliminaryFileName, file.fileName);
    }
  }
  assert(Object.values(policy.files).some((file) => file.fingerprint === "bundler"));
  assert(Object.values(policy.files).some((file) => file.fingerprint === "sha256"));
  const image = Object.values(files).find(
    (file) => file.type === "asset" && file.names.includes("proof.svg"),
  );
  const css = Object.values(files).find(
    (file) =>
      file.type === "asset" && file.fileName.endsWith(".css") && !file.names.includes("stylex.css"),
  );
  assert(image && css);
  assert(css.source.includes(image.fileName));
  assert.equal(css.fileName, `assets/${hash(css.source)}.css`);
});

test("StyleX publishes independent complete content-addressed CSS without mutating base CSS", async (t) => {
  const before = await fixture(t, "#fb6e30");
  const after = await fixture(t, "#3b82f6");
  const style = (files) =>
    Object.values(files).find((file) => file.type === "asset" && file.names.includes("stylex.css"));
  const first = style(before.files);
  const second = style(after.files);
  assert(first && second);
  assert.notEqual(first.source, second.source);
  assert.notEqual(first.fileName, second.fileName);
  assert.equal(first.fileName, `assets/${hash(first.source)}.css`);
  assert.equal(second.fileName, `assets/${hash(second.source)}.css`);
  assert(before.files["index.html"].source.includes(first.fileName));
  assert(after.files["index.html"].source.includes(second.fileName));
  for (const result of [before, after]) {
    const base = Object.values(result.files).find(
      (file) =>
        file.type === "asset" &&
        file.fileName.endsWith(".css") &&
        !file.names.includes("stylex.css"),
    );
    assert(base);
    assert(!base.source.includes("fb6e30") && !base.source.includes("3b82f6"));
  }
});

for (const cssCodeSplit of [true, false])
  test(`a late CSS mutation cannot be blessed as a content-addressed artifact (split=${cssCodeSplit})`, async (t) => {
    await assert.rejects(
      fixture(
        t,
        "#fb6e30",
        [
          {
            name: "late-css-change",
            generateBundle: {
              order: "pre",
              handler(_output, bundle) {
                const css = Object.values(bundle).find(
                  (file) => file.type === "asset" && file.fileName.endsWith(".css"),
                );
                assert(css);
                css.source += "body{color:purple}";
              },
            },
          },
        ],
        cssCodeSplit,
      ),
      /content-addressed web asset was changed after naming/,
    );
  });

test("the independent program document keeps its own restrictive policy", async (t) => {
  const { files, policy } = await fixture(t);
  assert(
    Buffer.from(files[policy.sandbox.file].source).toString("utf8").includes("flame-genui-connect"),
  );
  assert(policy.sandbox.policy.includes("connect-src 'none'"));
  assert(policy.sandbox.policy.includes("frame-src 'none'"));
  assert(!policy.sandbox.policy.includes("frame-ancestors"));
  assert(!policy.sandbox.policy.includes("strict-dynamic"));
});

test("shared multi-entry CSS is addressed from the final emitted bytes", async (t) => {
  const { files, policy } = await fixture(t, "#fb6e30", [], false);
  const shared = Object.values(files).find(
    (file) =>
      file.type === "asset" && file.fileName.endsWith(".css") && !file.names.includes("stylex.css"),
  );
  assert(shared);
  for (const document of ["index", "component", "fixture"])
    assert(files[`${document}.html`].source.includes(shared.fileName));
  for (const file of Object.values(files)) {
    if (file.type !== "asset" || !file.fileName.endsWith(".css")) continue;
    assert.equal(file.fileName, `assets/${hash(file.source)}.css`);
    assert.equal(policy.files[file.fileName].sha256, hash(file.source));
    assert.equal(policy.files[file.fileName].fingerprint, "sha256");
  }
});
