import { createHash, randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import { extname, relative, resolve, sep } from "node:path";
import { parse } from "parse5";
import {
  DOCUMENT_POLICY,
  EMBEDDED_DOCUMENT_POLICY,
  NONCE_PLACEHOLDER,
  SANDBOX_FILE,
  SANDBOX_POLICY,
  WEB_POLICY_FILE,
  documentResponse,
} from "./webPolicy.mjs";

const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
const sandbox = new URL("./genui-sandbox.html", import.meta.url);
const within = (root, name) => {
  const target = resolve(root, name);
  const path = relative(root, target);
  if (path === ".." || path.startsWith(`..${sep}`))
    throw new Error("web artifact escapes its root");
  return target;
};

export function webArtifacts() {
  let config;
  const addressed = new Map();
  const policy = {
    version: 1,
    noncePlaceholder: NONCE_PLACEHOLDER,
    documentPolicy: DOCUMENT_POLICY,
    embeddedPolicy: EMBEDDED_DOCUMENT_POLICY,
    sandbox: { file: SANDBOX_FILE, policy: SANDBOX_POLICY },
  };
  async function sendSandbox(request, response) {
    if (request.method !== "GET" && request.method !== "HEAD") {
      response.writeHead(405, { Allow: "GET, HEAD" });
      response.end();
      return;
    }
    response.writeHead(200, {
      "Content-Type": "text/html; charset=utf-8",
      "Content-Security-Policy": SANDBOX_POLICY,
      "Cache-Control": "no-store",
      "X-Content-Type-Options": "nosniff",
    });
    response.end(request.method === "HEAD" ? undefined : await readFile(sandbox));
  }
  return {
    name: "flame-web-artifact-policy",
    enforce: "post",
    config() {
      return {
        html: { cspNonce: NONCE_PLACEHOLDER },
        build: {
          rolldownOptions: {
            output: {
              assetFileNames(asset) {
                const extension = extname(asset.names[0] ?? "");
                // Vite 8.3.7 removes the first marker from split CSS already in
                // generateBundle's snapshot. Shared CSS is emitted inside that
                // hook and retains its marker. Address those actual final bytes.
                const source =
                  config.build.cssCodeSplit &&
                  extension === ".css" &&
                  typeof asset.source === "string"
                    ? asset.source.replace(/\/\*\$vite\$:\d+\*\//, "")
                    : asset.source;
                const sha256 = digest(source);
                const name = `assets/${sha256}${extension}`;
                addressed.set(name, sha256);
                return name;
              },
            },
          },
        },
      };
    },
    configResolved(value) {
      config = value;
    },
    async buildStart() {
      addressed.clear();
      if (config.command !== "build") return;
      this.emitFile({ type: "asset", fileName: SANDBOX_FILE, source: await readFile(sandbox) });
    },
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if (request.url?.split("?")[0] !== `/${SANDBOX_FILE}`) return next();
        void sendSandbox(request, response).catch(next);
      });
    },
    configurePreviewServer(server) {
      const root = resolve(config.root, config.build.outDir);
      server.middlewares.use((request, response, next) => {
        void (async () => {
          const url = new URL(request.url ?? "/", "http://localhost");
          if (request.method !== "GET" && request.method !== "HEAD") return next();
          let name = decodeURIComponent(url.pathname).slice(1);
          if (name.split("/").some((part) => part.startsWith("."))) {
            response.writeHead(404);
            response.end();
            return;
          }
          if (!name || name.endsWith("/")) name += "index.html";
          if (!name.endsWith(".html")) return next();
          const manifest = JSON.parse(await readFile(within(root, WEB_POLICY_FILE), "utf8"));
          const file = manifest.files[name];
          if (!file) {
            response.writeHead(404);
            response.end();
            return;
          }
          const bytes = await readFile(within(root, name));
          if (digest(bytes) !== file.sha256)
            throw new Error("web document differs from its artifact policy");
          if (name === manifest.sandbox.file) {
            response.writeHead(200, {
              "Content-Type": "text/html; charset=utf-8",
              "Content-Security-Policy": manifest.sandbox.policy,
              "Cache-Control": "no-store",
              "X-Content-Type-Options": "nosniff",
            });
            response.end(request.method === "HEAD" ? undefined : bytes);
            return;
          }
          const rendered = documentResponse(
            manifest,
            bytes.toString("utf8"),
            randomBytes(24).toString("base64"),
            file.document,
          );
          response.writeHead(200, rendered.headers);
          response.end(request.method === "HEAD" ? undefined : rendered.body);
        })().catch(next);
      });
    },
    generateBundle: {
      order: "post",
      handler(_output, bundle) {
        const files = {};
        for (const file of Object.values(bundle)) {
          if (file.fileName.split("/").some((part) => part.startsWith("."))) continue;
          const bytes = file.type === "chunk" ? file.code : file.source;
          const sha256 = digest(bytes);
          let fingerprint;
          if (
            file.type === "chunk" &&
            typeof file.preliminaryFileName === "string" &&
            /!~\{[A-Za-z0-9_$]{1,17}\}~/.test(file.preliminaryFileName) &&
            file.preliminaryFileName !== file.fileName
          )
            fingerprint = "bundler";
          if (addressed.has(file.fileName)) {
            if (addressed.get(file.fileName) !== sha256)
              throw new Error(
                `content-addressed web asset was changed after naming: ${file.fileName} (named ${addressed.get(file.fileName)}, final ${sha256})`,
              );
            fingerprint = "sha256";
          }
          let document;
          if (file.fileName.endsWith(".html")) {
            const html = parse(Buffer.from(bytes).toString("utf8")).childNodes.find(
              (node) => node.tagName === "html",
            );
            const role = html?.attrs.find(
              (attribute) => attribute.name === "data-lab-document",
            )?.value;
            document =
              file.fileName === SANDBOX_FILE
                ? "sandbox"
                : role === "component" || role === "fixture"
                  ? "same-origin"
                  : "main";
          }
          files[file.fileName] = {
            sha256,
            ...(fingerprint ? { fingerprint } : {}),
            ...(document ? { document } : {}),
          };
        }
        this.emitFile({
          type: "asset",
          fileName: WEB_POLICY_FILE,
          source: JSON.stringify({ ...policy, files }),
        });
      },
    },
  };
}
