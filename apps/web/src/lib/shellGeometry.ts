const DOCK_MIN_WIDTH_PX = 320;
const CONVERSATION_READING_MIN_PX = 440;
export function canPresentDock(rowWidth: number): boolean {
  return rowWidth >= DOCK_MIN_WIDTH_PX + CONVERSATION_READING_MIN_PX;
}
