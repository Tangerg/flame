import { service } from "dougong";
export interface WindowService {
  setTitle(text: string): void;
  setBadge(n?: number): void;
  setWorking(on: boolean): void;
}

export const WINDOW = service<WindowService>("flame.shell.window");
