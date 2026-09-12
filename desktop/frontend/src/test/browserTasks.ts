export function drainBrowserTasks(): Promise<void> {
  return (window as unknown as HappyDOMWindow).happyDOM.waitUntilComplete();
}
import type { Window as HappyDOMWindow } from "happy-dom";
