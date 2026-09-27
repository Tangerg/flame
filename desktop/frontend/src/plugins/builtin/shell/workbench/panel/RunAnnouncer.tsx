import { useState } from "react";
import { useT } from "@/lib/i18n";
import { useCurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { runAnnouncement, runAnnouncementKey } from "@/plugins/builtin/agent/public/announcement";

export function RunAnnouncer() {
  const t = useT();
  const material = useCurrentRootMaterial();
  const announcement = runAnnouncement(material.status, material.outcome);
  const [atMount] = useState(announcement);
  const [changed, setChanged] = useState(false);
  if (!changed && announcement !== atMount) setChanged(true);
  const key = changed ? runAnnouncementKey(announcement) : null;

  return (
    <output aria-live="polite" className="sr-only" data-slot="run-announcer">
      {key === null ? "" : t(key)}
    </output>
  );
}
