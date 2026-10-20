import {
  CompletionSoundSection,
  SystemNotificationsSection,
} from "@/features/notifications/public/PreferencesSections";
import { StreamRevealSection } from "@/features/chat/public/StreamRevealSection";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
export function PersonalizationPane() {
  return (
    <SettingsGroup>
      <StreamRevealSection />
      <SystemNotificationsSection />
      <CompletionSoundSection />
    </SettingsGroup>
  );
}
