// A cross-cutting CONTRACT, not a styling detail: globals.css hangs the selection scope and
// markdown overrides off it, and chat search walks the same subtree. Named here so consumers
// import the fact — a duplicated selector is a dependency no import guard can see.
export const MESSAGE_CONTENT_CLASS = "msg-content";
export const MESSAGE_CONTENT_SELECTOR = `.${MESSAGE_CONTENT_CLASS}`;
