export const COMPOSITION_SAMPLES = [
  { id: "one", frames: 1 },
  { id: "eight", frames: 8 },
  { id: "thirty-two", frames: 32 },
] as const;

export type CompositionFixtureState = (typeof COMPOSITION_SAMPLES)[number]["id"];
