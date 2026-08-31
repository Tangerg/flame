// What the AGENT decides about a view model, not the view model itself. The shapes
// (`Message`, `ToolCall`, `ContentBlock`, …) are the platform's contract in
// `plugins/sdk/types`, and re-exporting them here gave one fact a second path: sixty-one
// files read as depending on this context while depending on a type below it.
export { toolCategory } from "../domain/toolCategory";
export { isAgentRunFailure } from "../application/view/runOutcome";
