import path from "node:path";
import styleXPlugin from "@stylexjs/babel-plugin";
import typescriptPreset from "@babel/preset-typescript";

/**
 * The one StyleX Babel configuration, read by both halves of the pipeline.
 *
 * StyleX compiles in two passes that must agree: Vite's Babel step rewrites `stylex.create`
 * into class names, and the PostCSS step re-parses the same sources to emit the CSS those
 * names refer to. Configure them separately and they diverge silently — the build succeeds,
 * the JS carries class names, and no rule ever defines them, so the component renders naked.
 *
 * The plugin resolves theme imports itself, because a variable's generated name is derived
 * from the file that defines it. That is why it has to be told about `@/`: the alias is
 * Vite's, and Babel has never heard of it.
 */
const root = path.resolve(import.meta.dirname);

export const stylexBabelConfig = {
  babelrc: false,
  configFile: false,
  presets: [[typescriptPreset, { isTSX: true, allExtensions: true }]],
  plugins: [
    [
      styleXPlugin,
      {
        dev: false,
        runtimeInjection: false,
        unstable_moduleResolution: { type: "commonJS", rootDir: root },
        aliases: { "@/*": [path.resolve(root, "src", "*")] },
      },
    ],
  ],
};
