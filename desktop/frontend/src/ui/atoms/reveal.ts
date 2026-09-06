import * as stylex from "@stylexjs/stylex";

/**
 * A control that appears when its row does — the × on a dock tab, the copy on a code block, a
 * message's actions.
 *
 * This was eleven class lists: which group, which pseudo-class, opacity or visibility. The one
 * part already owned was the touch fallback (`[data-reveal]` in `globals.css`), because a device
 * with no pointer can never hover; the mechanism itself was written out at every call site.
 *
 * StyleX has no ancestor selector, so the host publishes its own state as a custom property and
 * the target reads it. Custom properties inherit, so the target may sit at any depth, and the
 * fallback is SHOWN — a target outside any host is simply visible rather than invisible.
 *
 * There are two ways to hide, and the difference is not stylistic: an `opacity` target is still
 * focusable, so `:focus-within` reveals it for a keyboard; a `visibility` target is not, so it is
 * a pointer affordance and the keyboard needs another route. The dock tab's × is the second on
 * purpose — a focusable sibling inside a `tablist` is an unallowed child (axe
 * `aria-required-children`), and Delete/Backspace on the focused tab is the ARIA practice.
 *
 * A device with no pointer can never hover, so the channel carries that fallback itself: what
 * `[data-reveal]` in `globals.css` used to say for everyone, each target now says for itself —
 * it has to, because a generated rule out-specifies that one.
 *
 * The channel owns the VALUES and never the transition. A transition list is one declaration,
 * so an element that fades and also recolours must state all of it in one place — and most of
 * these targets are `Button`s with a list of their own that a stray `transition-property` here
 * would silently replace. Each target keeps the fade it already had.
 */
export const reveal = stylex.create({
  host: {
    "--reveal": { default: "0", ":hover": "1", ":focus-within": "1" },
    "--reveal-events": { default: "none", ":hover": "auto", ":focus-within": "auto" },
    "--rest": { default: "1", ":hover": "0", ":focus-within": "0" },
    "--rest-events": { default: "auto", ":hover": "none", ":focus-within": "none" },
    // A pointer affordance answers the pointer only. It is out of the tab order on purpose,
    // so focus landing anywhere in the host must not bring it back.
    "--reveal-pointer": { default: "0", ":hover": "1" },
    "--reveal-visibility": { default: "hidden", ":hover": "visible" },
  },
  shown: {
    opacity: { default: "var(--reveal, 1)", "@media (hover: none)": 1 },
    pointerEvents: { default: "var(--reveal-events, auto)", "@media (hover: none)": "auto" },
  },
  /** Hidden the other way: out of the tab order entirely, for a control the keyboard reaches
   *  by some other key. Reveal alone would leave it focusable, which `shown` is for. */
  pointerAffordance: {
    opacity: { default: "var(--reveal-pointer, 1)", "@media (hover: none)": 1 },
    visibility: { default: "var(--reveal-visibility, visible)", "@media (hover: none)": "visible" },
  },
  /** What the reveal displaces. It has to give way at the same moment or the two overlap. */
  displaced: {
    opacity: { default: "var(--rest, 1)", "@media (hover: none)": 0 },
    pointerEvents: { default: "var(--rest-events, auto)", "@media (hover: none)": "none" },
  },
});
