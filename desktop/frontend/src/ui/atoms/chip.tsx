import type { ReactNode } from "react";
import { Icon, type IconName } from "@/ui/icons";
import { useT } from "@/lib/i18n";
import { ButtonPrimitive } from "@/ui/primitives";
import { Tooltip } from "./tooltip";

interface Props {
  icon?: IconName;
  children: ReactNode;
  title?: string;
  onClose?: () => void;
}

export function Chip({ icon, children, title, onClose }: Props) {
  const t = useT();
  return (
    <Tooltip label={title}>
      <span className="group inline-flex h-[var(--control-height-sm)] items-center gap-1.5 rounded-pill border-[length:var(--control-edge-width)] border-field bg-accent-badge pl-2.5 pr-1 text-ui-sm font-normal text-fg-soft">
        {icon && <Icon name={icon} size="xs" />}
        <span className="max-w-[220px] truncate font-mono">{children}</span>
        {onClose && (
          <ButtonPrimitive
            data-reveal="hover"
            type="button"
            className="grid h-5 w-5 place-items-center rounded-pill border-0 bg-transparent text-fg-faint pointer-events-none opacity-0 scale-[0.96] transition-[opacity,scale,background-color,color] group-hover:pointer-events-auto group-hover:opacity-100 group-hover:scale-100 focus-visible:pointer-events-auto focus-visible:opacity-100 hover:bg-hover hover:text-fg active:scale-[var(--press-scale)]"
            onClick={onClose}
            aria-label={t("common.remove")}
          >
            <Icon name="x" size="xs" />
          </ButtonPrimitive>
        )}
      </span>
    </Tooltip>
  );
}
