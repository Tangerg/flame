import { applyTheme } from "./theme";

// The carrier grants this port once. Guests receive appearance projections, never preferences.
function connect(event: MessageEvent) {
  if (
    event.source !== parent ||
    event.data?.type !== "flame.view.connect.v3" ||
    event.ports.length !== 1
  )
    return;
  removeEventListener("message", connect);
  applyTheme(document.documentElement, event.data.theme);
  const port = event.ports[0]!;
  port.addEventListener("message", (message) => {
    if (message.data?.type === "theme")
      applyTheme(document.documentElement, message.data.theme);
  });
  port.start();
}
addEventListener("message", connect);
