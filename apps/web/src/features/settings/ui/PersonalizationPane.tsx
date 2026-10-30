import { StreamRevealSection } from "@/features/chat/public/StreamRevealSection";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
export function PersonalizationPane() {
  return (
    <SettingsGroup>
      <StreamRevealSection />
    </SettingsGroup>
  );
}
