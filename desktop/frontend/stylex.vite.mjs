import path from "node:path";
import stylex from "@stylexjs/unplugin";

/**
 * One transform owns both generated class names and their CSS. App, visual
 * fixtures and tests share its resolution policy so theme identities cannot drift.
 */
const root = path.resolve(import.meta.dirname);
const options = {
  dev: false,
  runtimeInjection: false,
  useCSSLayers: false,
  lightningcssOptions: { minify: true },
  unstable_moduleResolution: { type: "commonJS", rootDir: root },
  aliases: { "@/*": [path.resolve(root, "src", "*")] },
};

export function stylexPlugin() {
  return stylex.vite(options);
}

// Tests compile styles without starting a browser CSS/HMR server lifetime.
export function stylexTransform() {
  return stylex.rolldown(options);
}
