import stylexPostcss from "@stylexjs/postcss-plugin";
import { stylexBabelConfig } from "./stylex.babel.mjs";

/**
 * StyleX re-parses the sources here to emit the CSS its Babel pass named, and writes it where
 * `@stylex;` sits in `globals.css`. The Babel config is the SAME object Vite hands its own
 * pass — two configs that drift produce class names with no rules behind them.
 *
 * StyleX is the only plugin here, and it is worth saying why the chain stays that way: when
 * Tailwind was still in the project the two fought over one PostCSS chain — with Tailwind
 * first, `@stylex;` was dropped before StyleX could claim it, and with Tailwind on PostCSS the
 * `@import`s in `globals.css` stopped being inlined and every code block lost its padding.
 */
export default {
  plugins: [
    stylexPostcss({
      // The SAME scope Babel's pass has, which is every `.ts(x)` in the project. They had
      // drifted: Babel rewrote `stylex.create` everywhere while this pass only read `src`, so a
      // style defined under `visual/` got a class name and no rule — the fixtures rendered
      // naked, which is why they were still written in utilities. That is the exact divergence
      // `stylex.babel.mjs` warns about, in the pair it warns about.
      include: ["src/**/*.{ts,tsx}", "visual/**/*.{ts,tsx}"],
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
