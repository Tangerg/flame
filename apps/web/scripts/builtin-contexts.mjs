const PREFIX = "features/";

export function isPublishedContextFile(path, context) {
  return path.startsWith(`${context}/public/`);
}

export function builtinContext(path) {
  if (!path.startsWith(PREFIX)) return null;
  const parts = path.slice(PREFIX.length).split("/");
  if (parts.length < 2) return null;
  return PREFIX + parts[0];
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
    file.startsWith("app/") ||
    file === "main.tsx" ||
    file === "test/setup.ts" ||
    /^features\/[^/]+\.test\.tsx?$/.test(file)
  );
}
