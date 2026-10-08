import { readFile } from "node:fs/promises";

export function pluginCarrier() {
  return {
    name: "plugin-carrier-policy",
    configureServer(server) {
      server.middlewares.use(async (request, response, next) => {
        if (request.url?.split("?")[0] !== "/plugin-carrier.html") {
          next();
          return;
        }
        try {
          const encoded = await readFile(
            new URL("./public/plugin-carrier-policy.txt", import.meta.url),
          );
          const policy = encoded.toString("utf8").trim();
          if (encoded.length >= 256 || !policy || /[\r\n]/.test(policy))
            throw new Error("invalid carrier policy");
          response.setHeader("Connection-Allowlist", policy);
          response.setHeader("Cache-Control", "no-store");
          next();
        } catch (error) {
          next(error);
        }
      });
    },
  };
}
