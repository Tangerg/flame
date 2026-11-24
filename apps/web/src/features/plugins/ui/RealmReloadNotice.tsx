import { useSyncExternalStore } from "react";
import { type InstalledPluginsOwner } from "@/features/plugins/adapters/installedPlugins";
import { useT } from "@/lib/i18n/index";
import { SystemMessage } from "@/ui/index";

export function RealmReloadNotice({
  owner,
}: {
  owner: Pick<
    InstalledPluginsOwner,
    "reload" | "reloadRequired" | "report" | "subscribeReloadRequired"
  >;
}) {
  const t = useT();
  const required = useSyncExternalStore(owner.subscribeReloadRequired, owner.reloadRequired);
  if (!required) return null;
  return (
    <SystemMessage
      variant="warning"
      icon="loop"
      action={{
        label: t("packages.module.reload"),
        onClick: () => void owner.reload().catch((error: unknown) => owner.report(error)),
      }}
    >
      {t("packages.reloadRequired")}
    </SystemMessage>
  );
}
