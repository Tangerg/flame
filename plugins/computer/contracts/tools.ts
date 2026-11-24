// Original CUA tools. Flame invokes js and js_reset for its callers; the engine
// must also declare turn_ended, which closes each control turn.
export const ENGINE_TOOLS = ["js", "js_reset"] as const;
export type EngineTool = (typeof ENGINE_TOOLS)[number];
export const TURN_ENDED_TOOL = "turn_ended";
export const REQUIRED_ENGINE_TOOLS: readonly string[] = [
  ...ENGINE_TOOLS,
  TURN_ENDED_TOOL,
];

export function isEngineTool(name: string): name is EngineTool {
  return (ENGINE_TOOLS as readonly string[]).includes(name);
}
