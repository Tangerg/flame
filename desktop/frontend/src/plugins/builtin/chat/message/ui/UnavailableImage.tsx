import * as stylex from "@stylexjs/stylex";
import { color, radius, space, surface, type as typeStep } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n";
import { Icon } from "@/ui";

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
  status: { color: color.fgMuted },
});

export function UnavailableImage({ alt = "", title }: { alt?: string; title?: string }) {
  const t = useT();
  const status = t("message.image.unavailable");
  return (
    <span role="img" aria-label={alt || status} title={title} {...stylex.props(styles.frame)}>
      <Icon name="image" size="md" />
      <span {...stylex.props(styles.copy)}>
        {alt && <span>{alt}</span>}
        {alt && " "}
        <span {...stylex.props(styles.status, typeStep.uiSm)}>{status}</span>
      </span>
    </span>
  );
}
