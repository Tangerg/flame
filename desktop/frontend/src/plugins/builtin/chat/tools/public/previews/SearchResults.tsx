import * as stylex from "@stylexjs/stylex";
import { ExternalLink } from "@/ui";
import type { WebSearchPreviewResult } from "../../application/specialisedPreviewProjections";
import {
  color,
  leading,
  motion,
  radius,
  space,
  surface,
  type as typeStep,
  weight,
} from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../../chatStyles";
const sr = stylex.create({
  // Auto-fill: the grid decides its own column count from the transcript's width.
  grid: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
    gap: space.s2,
  },
  card: {
    display: "flex",
    flexDirection: "column",
    gap: space.s1_5,
    borderRadius: radius.card,
    backgroundColor: { default: surface.sunken, ":hover": surface.hover },
    paddingInline: space.s3_5,
    paddingBlock: space.s3,
    textDecoration: "none",
    transitionProperty: "background-color",
    transitionDuration: motion.fast,
  },
  domainLine: {
    display: "flex",
    alignItems: "center",
    gap: space.s1_5,
    fontFamily: "var(--font-mono)",
    color: color.fgMuted,
  },
  // The favicon's stand-in: one letter on a plate, at the size the domain line reads at.
  mark: {
    display: "grid",
    height: "calc(var(--spacing) * 3.5)",
    width: "calc(var(--spacing) * 3.5)",
    flexShrink: 0,
    placeItems: "center",
    borderRadius: radius.step2xs,
    backgroundColor: surface.surface3,
    fontFamily: "var(--font-sans)",
    fontWeight: weight.semibold,
    color: color.fgMuted,
  },
  title: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 2,
    overflow: "hidden",
    fontWeight: weight.semibold,
    lineHeight: leading.snug,
    color: color.fg,
  },
  snippet: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 3,
    overflow: "hidden",
    lineHeight: leading.body,
    color: color.fgMuted,
  },
});

export function SearchResults({ results }: { results: WebSearchPreviewResult[] }) {
  return (
    <div {...stylex.props(sr.grid)}>
      {results.map((r) => (
        <ExternalLink
          key={r.url}
          href={r.url}
          title={r.url}
          className={stylex.props(sr.card).className}
        >
          <div {...stylex.props(sr.domainLine, typeStep.uiSm)}>
            <span {...stylex.props(sr.mark, typeStep.ui2xs)}>
              {(r.domain[0] ?? "?").toUpperCase()}
            </span>
            <span {...stylex.props(ct.truncate)}>{r.domain}</span>
          </div>
          <div {...stylex.props(sr.title, typeStep.uiMd)}>{r.title}</div>
          <div {...stylex.props(sr.snippet, typeStep.uiMd)}>{r.snippet}</div>
        </ExternalLink>
      ))}
    </div>
  );
}
