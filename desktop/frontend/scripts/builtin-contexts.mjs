// These directories group independent plugin owners; rings are optional and
// never decide whether a feature receives a dependency boundary.
const NAMESPACES = new Set(["chat", "command", "settings", "shell"]);
const PREFIX = "plugins/builtin/";

// The shared settings presentation kit already has an explicit module API.
// Keep its implementation private without adding another forwarding facade.
const PUBLISHED_ENTRIES = new Map([
  ["plugins/builtin/settings/kit", new Set(["index.ts", "panes.ts", "settingStyles.ts"])],
]);

export function isPublishedContextFile(path, context) {
  return (
    path.startsWith(`${context}/public/`) ||
    PUBLISHED_ENTRIES.get(context)?.has(path.slice(context.length + 1)) === true
  );
}

export function builtinContext(path) {
  if (!path.startsWith(PREFIX)) return null;
  const parts = path.slice(PREFIX.length).split("/");
  if (parts.length < 2) return null;
  const depth = NAMESPACES.has(parts[0]) && parts.length > 2 ? 2 : 1;
  return PREFIX + parts.slice(0, depth).join("/");
}

export function contextRootsOf(graph) {
  const roots = new Set();
  for (const [file, dependencies] of Object.entries(graph)) {
    for (const path of [file, ...dependencies]) {
      const context = builtinContext(path);
      if (context) roots.add(context);
    }
  }
  return [...roots].sort();
}

export function atCompositionRoot(file) {
  return (
    file.startsWith("main/") ||
    file === "test/setup.ts" ||
    /^plugins\/builtin\/[^/]+\.test\.tsx?$/.test(file)
  );
}
