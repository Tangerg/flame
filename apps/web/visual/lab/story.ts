import type { ComponentType } from "react";
import type { VisualAgentState } from "../agentSessionSnapshots";
import type { VisualWorkIndexState } from "../shellFixtureStates";
import type { VisualSettingsPane, VisualWorkspaceState } from "../workspaceFixtureStates";

export type LabLocale = "zh-CN" | "en";
export type LabText = Readonly<Record<LabLocale, string>>;
export type LabPanel = "foundation" | "business";
export type LabTheme = "light" | "dark";
export type LabMotion = "off" | "full";
export type ObservationValue = string | number | boolean | null;

export interface LabExampleProps {
  readonly variant: string;
  readonly locale: LabLocale;
  readonly observe: (name: string, value?: ObservationValue) => void;
}

export interface LabVariant {
  readonly id: string;
  readonly title: LabText;
}

export type LabFixtureTarget =
  | { readonly fixture: "agent"; readonly state: VisualAgentState }
  | { readonly fixture: "shell"; readonly state: VisualWorkIndexState }
  | {
      readonly fixture: "workspace";
      readonly state: VisualWorkspaceState;
      readonly pane?: VisualSettingsPane;
    };

interface StoryDescription<V extends LabVariant> {
  readonly id: string;
  readonly panel: LabPanel;
  readonly title: LabText;
  readonly summary: LabText;
  readonly group: LabText;
  readonly source: string;
  readonly width: number;
  readonly height: number;
  readonly variants: readonly [V, ...V[]];
}

export interface ComponentStory extends StoryDescription<LabVariant> {
  readonly kind: "component";
  readonly load: () => Promise<{ default: ComponentType<LabExampleProps> }>;
}

export interface FixtureVariant extends LabVariant {
  readonly target: LabFixtureTarget;
}

export interface FixtureStory extends StoryDescription<FixtureVariant> {
  readonly kind: "fixture";
}

export type LabStory = ComponentStory | FixtureStory;

export function localize(text: LabText, locale: LabLocale): string {
  return text[locale];
}
