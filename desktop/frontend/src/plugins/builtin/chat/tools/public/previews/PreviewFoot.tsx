import { Button, Icon } from "@/ui";
import { useT } from "@/lib/i18n";

export function PreviewFoot({ label, onClick }: { label: string; onClick?: () => void }) {
  const t = useT();
  if (!onClick) return null;
  return (
    <div className="mt-2 pt-1.5 text-right">
      <Button variant="ghost" size="xs" onClick={onClick}>
        {t(label)} <Icon name="share" size="xs" />
      </Button>
    </div>
  );
}
