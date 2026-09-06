import stylexPostcss from "@stylexjs/postcss-plugin";
import { stylexBabelConfig } from "./stylex.babel.mjs";

/**
 * StyleX re-parses the sources here to emit the CSS its Babel pass named, and writes it where
 * `@stylex;` sits in `globals.css`. The Babel config is the SAME object Vite hands its own
 * pass — two configs that drift produce class names with no rules behind them.
 *
 * Tailwind is NOT here. It keeps its own Vite plugin and its own sheet: run through one
 * PostCSS chain the two fight — with Tailwind first `@stylex;` was dropped before StyleX
 * could claim it, and with Tailwind on PostCSS the `@import`s in `globals.css` stopped being
 * inlined and every code block lost its padding.
 */
export default {
  plugins: [
    stylexPostcss({
      include: ["src/**/*.{ts,tsx}"],
      babelConfig: stylexBabelConfig,
      // NOT layers. An unlayered rule beats every layer regardless of specificity, and this
      // sheet is full of them — the first migrated component came out `display: block`
      // because a global span rule outranked StyleX's own `inline-block`. Unlayered, StyleX
      // competes on source order like Tailwind's utilities do, and its sheet is imported
      // after `globals.css`.
      useCSSLayers: false,
    }),
  ],
};
