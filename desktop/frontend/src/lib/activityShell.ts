/**
 * Lives BELOW both the domain and the design system: a view model naming this fact from
 * `ui/agent` would make the presentation ring import a component ring.
 *
 *   line     Work-narrative activity; disclosed material owns any terminal/diff surface.
 *   card     A composite product with a narrative of its own, such as a delegated Run.
 *   flagged  A composite card whose own boundary needs attention.
 */
export type ActivityShell = "line" | "card" | "flagged";
