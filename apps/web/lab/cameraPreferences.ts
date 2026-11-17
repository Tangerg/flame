import { z } from "zod";
import type { LabPanel } from "./story";
export interface Size {
  width: number;
  height: number;
}
const checkpoint = z.object({
  positionX: z.number().finite(),
  positionY: z.number().finite(),
  scale: z.number().min(0.01).max(16),
});
export function readCamera(panel: LabPanel): z.infer<typeof checkpoint> | undefined {
  const raw = localStorage.getItem(`flame.lab.viewport.${panel}`);
  if (raw === null) return;
  try {
    const parsed = checkpoint.safeParse(JSON.parse(raw));
    return parsed.success ? parsed.data : undefined;
  } catch (error) {
    if (!(error instanceof SyntaxError)) throw error;
  }
}
export function saveCamera(panel: LabPanel, value: z.infer<typeof checkpoint>) {
  localStorage.setItem(`flame.lab.viewport.${panel}`, JSON.stringify(checkpoint.parse(value)));
}
