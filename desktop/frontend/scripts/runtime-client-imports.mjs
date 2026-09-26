const CLIENT_MODULE = "@flame/runtime-contract/client";

export function importsRuntimeClient(source) {
  const code = source.replace(/\/\*[\s\S]*?\*\/|^\s*\/\/.*$/gm, "");
  const modules = code.matchAll(
    /\b(?:from\s+|import\s*\(\s*|require\s*\(\s*|import\s*)["']([^"']+)["']/g,
  );
  for (const [, specifier] of modules) {
    if (specifier === CLIENT_MODULE || specifier.startsWith(`${CLIENT_MODULE}/`)) return true;
  }
  return false;
}
