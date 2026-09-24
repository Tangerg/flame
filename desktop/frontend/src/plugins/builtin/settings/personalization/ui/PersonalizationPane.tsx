import {
  CompletionSoundSection,
  StreamRevealSection,
  SystemNotificationsSection,
} from "./PrefSections";
import { SettingsGroup } from "../../kit";

export function PersonalizationPane() {
  return (
    <SettingsGroup>
      <StreamRevealSection />
      <SystemNotificationsSection />
      <CompletionSoundSection />
    </SettingsGroup>
  );
}
