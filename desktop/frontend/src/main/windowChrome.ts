import { getContainer } from "@/main/container";

const CONTROL_GAP_PX = 6;

const GUTTER_PROPERTY = "--window-controls-gutter";
const CENTRE_PROPERTY = "--window-controls-centre";

export async function applyWindowChrome(): Promise<void> {
  const chrome = await getContainer().desktop.windowChrome();
  const root = document.documentElement;
  if (!chrome) {
    root.style.removeProperty(GUTTER_PROPERTY);
    root.style.removeProperty(CENTRE_PROPERTY);
    return;
  }
  const hidden = chrome.controlsInlineEnd === 0;
  root.style.setProperty(
    GUTTER_PROPERTY,
    hidden ? "0px" : `${chrome.controlsInlineEnd + CONTROL_GAP_PX}px`,
  );
  root.style.setProperty(CENTRE_PROPERTY, hidden ? "" : `${chrome.controlsCentreY}px`);
  if (hidden) root.style.removeProperty(CENTRE_PROPERTY);
}

export function watchWindowChrome(): () => void {
  const refresh = () => void applyWindowChrome();
  addEventListener("resize", refresh);
  return () => removeEventListener("resize", refresh);
}
