export const HOST_MODULES = Object.freeze([
  "react",
  "react-dom",
  "react-dom/client",
  "react/jsx-runtime",
  "react/jsx-dev-runtime",
  "@flame/component-sdk",
  "@flame/ui",
]);

// Private dependencies are bundled; these modules are supplied by the actual
// workbench renderer. The format is identical for released and development code.
export async function buildComponent({ input, directory, compiler }) {
  if (typeof compiler !== "function")
    throw new TypeError("component compiler is required");
  const bundle = await compiler({
    input,
    platform: "browser",
    external: (id) => HOST_MODULES.includes(id) || id.startsWith("@flame/ui/"),
    transform: {
      jsx: { runtime: "automatic", development: false },
      define: { "process.env.NODE_ENV": JSON.stringify("production") },
    },
    onwarn(warning, report) {
      if (
        warning.code === "UNRESOLVED_IMPORT" ||
        warning.code === "UNRESOLVED_ENTRY"
      )
        throw new Error(warning.message);
      if (
        warning.code === "MODULE_LEVEL_DIRECTIVE" &&
        warning.message.includes("use client")
      )
        return;
      report(warning);
    },
  });
  try {
    await bundle.write({
      dir: directory,
      format: "esm",
      entryFileNames: "component.js",
      chunkFileNames: "[name]-[hash].js",
      assetFileNames: "[name]-[hash][extname]",
      minify: true,
    });
  } finally {
    await bundle.close();
  }
}
