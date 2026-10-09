import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { resolve, extname, sep } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(fileURLToPath(new URL("../dist/", import.meta.url)));
const port = Number(process.env.PORT ?? 8770);
const types = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".svg": "image/svg+xml",
};
createServer(async (request, response) => {
  try {
    const url = new URL(request.url, "http://localhost");
    if (url.pathname === "/") {
      response.writeHead(302, { Location: "/preview/index.html" }).end();
      return;
    }
    const path = resolve(root, `.${decodeURIComponent(url.pathname)}`);
    if (!path.startsWith(root + sep)) {
      response.writeHead(403).end();
      return;
    }
    const body = await readFile(path);
    response
      .writeHead(200, {
        "Content-Type": `${types[extname(path)] ?? "application/octet-stream"}; charset=utf-8`,
      })
      .end(body);
  } catch (error) {
    if (
      error.code === "ENOENT" ||
      error.code === "EISDIR" ||
      error instanceof URIError
    )
      response.writeHead(404).end();
    else {
      console.error(error);
      response.writeHead(500).end();
    }
  }
}).listen(port, "127.0.0.1", () =>
  console.log(`Icon preview: http://127.0.0.1:${port}`),
);
