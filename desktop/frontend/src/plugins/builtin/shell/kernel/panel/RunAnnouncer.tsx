import { useState } from "react";
import { useT } from "@/lib/i18n";
import { useCurrentRootMaterial } from "@/plugins/builtin/agent/public/run";
import { runAnnouncement, runAnnouncementKey } from "@/plugins/builtin/agent/public/announcement";

/**
 * The transcript arrives without a sound: a turn appears in the DOM and a screen reader is
 * told nothing. This says only that the turn changed state — never its text, which a polite
 * region would re-read from the top on every streamed chunk.
 *
 * Rendered outside the transcript and never keyed on the session, so the region is in the
 * document BEFORE its content changes — and it opens EMPTY: a reader that lands on a chat
 * which finished yesterday is told "Response complete" if the text arrives in the same
 * commit as the region, which is an announcement about nothing the reader did.
 */
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
