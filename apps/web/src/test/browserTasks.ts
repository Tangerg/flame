import { type Window as HappyDOMWindow } from "happy-dom";
export function drainBrowserTasks(): Promise<void> {
  return (window as unknown as HappyDOMWindow).happyDOM.waitUntilComplete();
}

export function preventBrowserNavigation(): () => void {
  const navigation = (window as unknown as HappyDOMWindow).happyDOM.settings.navigation;
  const original = { ...navigation };
  Object.assign(navigation, {
    disableMainFrameNavigation: true,
    disableChildFrameNavigation: true,
    disableChildPageNavigation: true,
    disableFallbackToSetURL: true,
  });
  return () => Object.assign(navigation, original);
}
