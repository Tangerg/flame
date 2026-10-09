import {
  createElement,
  type ComponentPropsWithRef,
  type CSSProperties,
  type ReactElement,
} from "react";
import { opticalSize } from "./opticalSize.js";

type DrawingAttributes =
  | "width"
  | "height"
  | "viewBox"
  | "stroke"
  | "strokeWidth"
  | "strokeLinecap"
  | "strokeLinejoin"
  | "strokeDasharray"
  | "strokeDashoffset"
  | "vectorEffect"
  | "fill"
  | "preserveAspectRatio";

export type IconProps = Omit<
  ComponentPropsWithRef<"svg">,
  DrawingAttributes | "children" | "dangerouslySetInnerHTML" | "style"
> & {
  size?: number;
  style?: Omit<CSSProperties, DrawingAttributes>;
};

export type IconComponent = (props: IconProps) => ReactElement;

type Path = Readonly<{ d: string; strokeWidth?: number }>;
export type Master = Readonly<{
  grid: 16 | 24;
  strokeWidth: number;
  paths: readonly Path[];
}>;

export function createIcon(
  name: string,
  small: Master,
  large: Master,
): IconComponent {
  function Icon({ size = 16, style, ...props }: IconProps): ReactElement {
    if (!Number.isFinite(size) || size <= 0) {
      throw new RangeError("icon size must be a finite positive number");
    }
    const master = opticalSize(size) === 16 ? small : large;
    const labelled = Boolean(props["aria-label"] || props["aria-labelledby"]);
    return createElement(
      "svg",
      {
        ...props,
        "aria-hidden": props["aria-hidden"] ?? (labelled ? undefined : true),
        role: props.role ?? (labelled ? "img" : undefined),
        focusable: props.focusable ?? false,
        width: size,
        height: size,
        viewBox: `0 0 ${master.grid} ${master.grid}`,
        preserveAspectRatio: "xMidYMid meet",
        fill: "none",
        stroke: "currentColor",
        strokeWidth: master.strokeWidth,
        strokeLinecap: "round",
        strokeLinejoin: "round",
        style: { ...style, width: size, height: size },
      },
      master.paths.map((path, index) =>
        createElement("path", { ...path, key: index }),
      ),
    );
  }
  Object.defineProperty(Icon, "name", { value: name });
  return Icon;
}
