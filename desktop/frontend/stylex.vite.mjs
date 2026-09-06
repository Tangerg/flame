import babel from "vite-plugin-babel";
import { stylexBabelConfig } from "./stylex.babel.mjs";

/**
 * The StyleX transform, as one plugin every Vite config takes.
 *
 * There are two configs — the app's and the visual fixtures' — and a plugin list written
 * twice drifts. It already did: the fixtures ran without this and `stylex.defineVars`
 * reached the browser uncompiled, which throws at import time and takes the whole page
 * with it.
 *
 * `include` matches on path, so every file pays Babel whether or not it mentions StyleX;
 * measured at roughly 2x the build on this tree. Nothing here can ask a file what it
 * imports before deciding.
 */
export function stylexBabel() {
  return babel({ include: /\.tsx?$/, babelConfig: stylexBabelConfig });
}
