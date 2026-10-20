import "@total-typescript/ts-reset";
/// <reference types="vite/client" />

// ts-reset tightens the TS stdlib: `.filter(Boolean)` narrows, and `JSON.parse` /
// `fetch().json()` return `unknown` rather than `any`, which forces a Zod parse or a guard
// at every boundary.
