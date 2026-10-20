import * as stylex from "@stylexjs/stylex";
import { useId } from "react";
import { Image } from "@flame/icons/react";
import { Icon } from "./font-scale";
import { color, radius, space, surface, type } from "./tokens.stylex";

const styles = stylex.create({
  frame: {
    display: "inline-flex",
    alignItems: "center",
    gap: space.s2,
    minWidth: 0,
    maxWidth: "100%",
    marginBlock: space.s3,
    paddingInline: space.s3,
    paddingBlock: space.s2,
    borderRadius: radius.card,
    backgroundColor: surface.sunken,
    color: color.fgSoft,
    verticalAlign: "top",
  },
  copy: {
    display: "grid",
    gap: space.s0_5,
    minWidth: 0,
    overflowWrap: "anywhere",
  },
  detail: { color: color.fgMuted },
});

export function ImageNotice({
  label,
  detail,
  title,
  styles: callerStyles,
}: {
  label?: string;
  detail: string;
  title?: string;
  styles?: stylex.StyleXArray<stylex.StyleXStyles | null | false>;
}) {
  const labelId = useId();
  const detailId = useId();
  return (
    <span
      role="img"
      aria-labelledby={label ? labelId : detailId}
      aria-describedby={label ? detailId : undefined}
      title={title}
      {...stylex.props(styles.frame, callerStyles)}
    >
      <Icon glyph={Image} size="md" aria-hidden />
      <span {...stylex.props(styles.copy)}>
        {label && <span id={labelId}>{label}</span>}
        {label && " "}
        <span id={detailId} {...stylex.props(styles.detail, type.uiSm)}>
          {detail}
        </span>
      </span>
    </span>
  );
}
