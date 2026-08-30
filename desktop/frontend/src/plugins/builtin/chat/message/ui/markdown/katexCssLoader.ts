// Its own file so the dynamic-import target is small enough for Vite to split into a chunk,
// while the rest of `katexCss.ts` stays in the main one for synchronous access.
import "katex/dist/katex.min.css";
