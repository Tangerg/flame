import { dirname, isAbsolute, join, normalize, relative } from "node:path";

export const CONFIGURATION = "FLAME_COMPUTER_SANDBOX";

function within(path, directory) {
  const difference = relative(directory, path);
  return (
    difference === "" ||
    (difference !== ".." &&
      !difference.startsWith("../") &&
      !isAbsolute(difference))
  );
}

function invalid() {
  throw new Error("computer: invalid native sandbox configuration");
}

export function configuration(environment) {
  const raw = environment[CONFIGURATION];
  if (typeof raw !== "string" || Buffer.byteLength(raw) > 65536) invalid();
  let value;
  try {
    value = JSON.parse(raw);
  } catch {
    invalid();
  }
  if (
    value === null ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    Object.keys(value).sort().join(",") !==
      "codex,filesystem,guardDirectory,launcher,node,runtime,sentinel,tempDirectory,workdir"
  )
    invalid();
  for (const [name, path] of Object.entries(value)) {
    if (name === "filesystem") continue;
    if (
      typeof path !== "string" ||
      !isAbsolute(path) ||
      normalize(path) !== path ||
      /[\p{Cc}\p{Surrogate}]/u.test(path)
    )
      invalid();
  }
  const expected = {
    ":minimal": "read",
    [dirname(value.runtime)]: "read",
    [value.guardDirectory]: "read",
    [dirname(value.launcher)]: "read",
    [value.workdir]: "write",
  };
  if (
    !within(value.node, value.runtime) ||
    value.filesystem === null ||
    typeof value.filesystem !== "object" ||
    Array.isArray(value.filesystem) ||
    Object.keys(expected).length !== 5 ||
    Object.keys(value.filesystem).length !== 5 ||
    Object.entries(expected).some(
      ([path, access]) => value.filesystem[path] !== access,
    ) ||
    Object.values(value).some(
      (path) =>
        typeof path === "string" &&
        path !== value.workdir &&
        within(path, value.workdir),
    ) ||
    [
      dirname(value.runtime),
      value.guardDirectory,
      dirname(value.launcher),
    ].some((directory) => within(value.sentinel, directory)) ||
    within(value.sentinel, value.tempDirectory)
  )
    invalid();
  Object.freeze(value.filesystem);
  return Object.freeze(value);
}

export function trustedServices(environment, config) {
  let services;
  try {
    services = JSON.parse(environment.NODE_REPL_TRUSTED_SERVICES ?? "null");
  } catch {
    invalid();
  }
  const wrapper = join(config.guardDirectory, "native-sky.mjs");
  if (
    services === null ||
    typeof services !== "object" ||
    Array.isArray(services) ||
    Object.keys(services).some(
      (name) => name !== "sky" && name !== "browser",
    ) ||
    (services.sky !== "@oai/sky/service" && services.sky !== wrapper) ||
    (services.browser !== undefined &&
      services.browser !== "@oai/browser-desktop/service")
  )
    invalid();
  return Object.freeze({
    sky: services.sky,
    files: Object.freeze([
      join(config.runtime, "lib/node_modules/@oai/sky/package.json"),
      ...(services.sky === wrapper ? [wrapper] : []),
      ...(services.browser
        ? [
            join(
              config.runtime,
              "lib/node_modules/@oai/browser-desktop/package.json",
            ),
          ]
        : []),
    ]),
  });
}
