import { createElement, createRef } from "react";
import { BellOff, type IconProps } from "@flame/icons/react";

createElement(BellOff, {
  size: 24,
  ref: createRef<SVGSVGElement>(),
  "aria-label": "Muted",
  className: "glyph",
  color: "red",
  style: { opacity: 0.5 },
});

// Geometry belongs to the drawing; callers choose its actual size and inherited color.
// @ts-expect-error
const geometry: IconProps = { strokeWidth: 2 };
// @ts-expect-error
const dimensions: IconProps = { width: 32 };
// @ts-expect-error
const content: IconProps = { children: "replacement" };
// @ts-expect-error
const token: IconProps = { size: "sm" };
// @ts-expect-error
const strokeStyle: IconProps = { style: { strokeWidth: 2 } };
void [geometry, dimensions, content, token, strokeStyle];
