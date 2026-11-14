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
  const plugin = stylex.vite(options);
  if (typeof plugin.__stylexCollectCss !== "function")
    throw new Error("stylex CSS collector is unavailable");
  let config;
  let stylesheet;
  return {
    ...plugin,
    configResolved(value) {
      plugin.configResolved?.(value);
      config = value;
    },
    renderStart() {
      stylesheet = undefined;
      const css = plugin.__stylexCollectCss();
      if (css) stylesheet = this.emitFile({ type: "asset", name: "stylex.css", source: css });
    },
    // The upstream generate/write hooks rewrite an already-named CSS asset and
    // its referring JS. Emit complete CSS before hashing instead.
    generateBundle: undefined,
    writeBundle: undefined,
    transformIndexHtml: {
      order: "post",
      handler(html, context) {
        if (config.command !== "build") return plugin.transformIndexHtml?.(html, context);
        if (!stylesheet) return;
        const asset = Object.values(context.bundle).find(
          (file) => file.type === "asset" && file.names.includes("stylex.css"),
        );
        if (!asset) throw new Error("extracted StyleX stylesheet was not emitted");
        return [
          {
            tag: "link",
            attrs: {
              rel: "stylesheet",
              href: `${config.base}${asset.fileName}`,
              ...(config.html.cspNonce ? { nonce: config.html.cspNonce } : {}),
            },
            injectTo: "head",
          },
        ];
      },
    },
  };
}

// Tests compile styles without starting a browser CSS/HMR server lifetime.
export function stylexTransform() {
  return stylex.rolldown(options);
}
