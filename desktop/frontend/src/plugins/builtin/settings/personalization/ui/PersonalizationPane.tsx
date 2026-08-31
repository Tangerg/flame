import { CompletionSoundSection, StreamRevealSection } from "./PrefSections";
import { SettingsGroup } from "../../kit";

export function PersonalizationPane() {
  return (
    <SettingsGroup>
      <StreamRevealSection />
      <CompletionSoundSection />
    </SettingsGroup>
  );
}
