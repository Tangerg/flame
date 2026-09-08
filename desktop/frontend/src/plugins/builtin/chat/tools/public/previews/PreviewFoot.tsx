import * as stylex from "@stylexjs/stylex";
import { Button, Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import { space } from "@/styles/tokens.stylex";

const pf = stylex.create({
  foot: { marginTop: space.s2, paddingTop: space.s1_5, textAlign: "right" },
});

export function PreviewFoot({ label, onClick }: { label: string; onClick?: () => void }) {
  const t = useT();
  if (!onClick) return null;
  return (
    <div {...stylex.props(pf.foot)}>
      <Button variant="ghost" size="xs" onClick={onClick}>
        {t(label)} <Icon name="share" size="xs" />
      </Button>
    </div>
  );
}
