import * as stylex from "@stylexjs/stylex";

export const reveal = stylex.create({
  host: {
    "--reveal": { default: "0", ":hover": "1", ":focus-within": "1" },
    "--reveal-events": { default: "none", ":hover": "auto", ":focus-within": "auto" },
    "--rest": { default: "1", ":hover": "0", ":focus-within": "0" },
    "--rest-events": { default: "auto", ":hover": "none", ":focus-within": "none" },
    "--reveal-pointer": { default: "0", ":hover": "1" },
    "--reveal-visibility": { default: "hidden", ":hover": "visible" },
  },
  shown: {
    opacity: { default: "var(--reveal, 1)", "@media (hover: none)": 1 },
    pointerEvents: { default: "var(--reveal-events, auto)", "@media (hover: none)": "auto" },
  },
  pointerAffordance: {
    opacity: { default: "var(--reveal-pointer, 1)", "@media (hover: none)": 1 },
    visibility: { default: "var(--reveal-visibility, visible)", "@media (hover: none)": "visible" },
  },
  displaced: {
    opacity: { default: "var(--rest, 1)", "@media (hover: none)": 0 },
    pointerEvents: { default: "var(--rest-events, auto)", "@media (hover: none)": "none" },
  },
});
