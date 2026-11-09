const TEXT_STYLE = [
  "direction",
  "font-family",
  "font-size",
  "font-stretch",
  "font-style",
  "font-variant",
  "font-weight",
  "font-feature-settings",
  "font-kerning",
  "font-variation-settings",
  "letter-spacing",
  "line-height",
  "overflow-wrap",
  "tab-size",
  "text-align",
  "text-indent",
  "text-transform",
  "word-break",
  "word-spacing",
] as const;

function px(style: CSSStyleDeclaration, property: string) {
  return Number.parseFloat(style.getPropertyValue(property)) || 0;
}

// Measures the textarea's text in an inert offscreen copy of its content box,
// so the live element's layout is never disturbed.
function measureMirror<T>(
  area: HTMLTextAreaElement,
  style: CSSStyleDeclaration,
  measure: (text: Text, mirror: HTMLElement) => T,
) {
  const document = area.ownerDocument;
  const padding = px(style, "padding-left") + px(style, "padding-right");
  const border =
    px(style, "border-left-width") + px(style, "border-right-width");
  const usedWidth = Number.parseFloat(style.width);
  const width = Number.isFinite(usedWidth)
    ? usedWidth - (style.boxSizing === "border-box" ? padding + border : 0)
    : area.getBoundingClientRect().width - padding - border;
  const scrollbar = Math.max(
    0,
    area.offsetWidth - area.clientWidth - Math.round(border),
  );
  if (width <= scrollbar) return null;
  const mirror = document.createElement("div");
  mirror.setAttribute("aria-hidden", "true");
  mirror.style.cssText =
    "position:fixed;top:0;left:0;visibility:hidden;pointer-events:none;contain:layout style paint;box-sizing:content-box;margin:0;border:0;padding:0;";
  for (const property of TEXT_STYLE)
    mirror.style.setProperty(property, style.getPropertyValue(property));
  mirror.style.whiteSpace = area.wrap === "off" ? "pre" : "pre-wrap";
  mirror.style.width = `${width - scrollbar}px`;
  // The trailing break opportunity keeps an empty or newline-ended last line.
  const text = document.createTextNode(`${area.value}\u200b`);
  mirror.append(text);
  document.body.append(mirror);
  try {
    return measure(text, mirror);
  } finally {
    mirror.remove();
  }
}

// Natural content height (excluding padding and border), at least `rows` lines.
export function textAreaContentHeight(
  area: HTMLTextAreaElement,
  style = getComputedStyle(area),
) {
  return measureMirror(area, style, (text, mirror) => {
    const content = mirror.getBoundingClientRect().height;
    text.data = "\u200b";
    const line = mirror.getBoundingClientRect().height;
    return Math.ceil(Math.max(content, line * area.rows));
  });
}

function caretFrame(area: HTMLTextAreaElement) {
  const document = area.ownerDocument;
  const style = getComputedStyle(area);
  // Native selection owns the offsets; this transient text only measures its line.
  return measureMirror(area, style, (text, mirror) => {
    const range = document.createRange();
    const offset =
      area.selectionDirection === "backward"
        ? area.selectionStart
        : area.selectionEnd;
    range.setStart(text, offset);
    range.collapse(true);
    const rect = range.getBoundingClientRect();
    if (!(rect.height > 0)) return null;
    const origin = mirror.getBoundingClientRect().top;
    const paddingTop = px(style, "padding-top");
    return {
      top: rect.top - origin + paddingTop,
      bottom: rect.bottom - origin + paddingTop,
      paddingTop,
      paddingBottom: px(style, "padding-bottom"),
    };
  });
}

function frame(area: HTMLTextAreaElement) {
  const bounds = area.getBoundingClientRect();
  return `${bounds.width}:${bounds.height}:${area.scrollHeight}`;
}

function selection(area: HTMLTextAreaElement) {
  return `${area.selectionStart}:${area.selectionEnd}:${area.selectionDirection}`;
}

export function followTextAreaCaret(area: HTMLTextAreaElement) {
  const document = area.ownerDocument;
  const window = document.defaultView!;
  let active = true;
  let focused = document.activeElement === area;
  let following = focused;
  let composing = false;
  let pressed = false;
  let pending = 0;
  let position = area.scrollTop;
  let layout = frame(area);
  let selected = selection(area);
  const ownsFocus = () =>
    active && area.isConnected && document.activeElement === area;
  const reveal = () => {
    pending = 0;
    layout = frame(area);
    position = area.scrollTop;
    if (
      !ownsFocus() ||
      !following ||
      composing ||
      pressed ||
      area.clientWidth <= 0 ||
      area.clientHeight <= 0 ||
      area.scrollHeight <= area.clientHeight
    )
      return;
    const caret = caretFrame(area);
    if (!caret) return;
    if (caret.top < position + caret.paddingTop)
      area.scrollTop = caret.top - caret.paddingTop;
    else if (caret.bottom > position + area.clientHeight - caret.paddingBottom)
      area.scrollTop = caret.bottom - area.clientHeight + caret.paddingBottom;
    position = area.scrollTop;
  };
  const schedule = () => {
    if (active && !pending) pending = window.requestAnimationFrame(reveal);
  };
  const onFocus = () => {
    // Window reactivation is not a new reading intent.
    if (focused) return;
    focused = true;
    following = true;
    selected = selection(area);
    schedule();
  };
  const onBlur = () => {
    if (document.activeElement === area) return;
    focused = false;
    following = false;
    composing = false;
    pressed = false;
    window.cancelAnimationFrame(pending);
    pending = 0;
  };
  const onSelect = () => {
    const next = selection(area);
    if (next === selected) return;
    selected = next;
    if (!ownsFocus()) return;
    following = true;
    position = area.scrollTop;
    schedule();
  };
  const onInput = () => {
    if (!ownsFocus()) return;
    following = true;
    selected = selection(area);
    position = area.scrollTop;
    schedule();
  };
  const onWheel = () => {
    following = false;
  };
  const onScroll = () => {
    if (area.scrollTop === position) return;
    position = area.scrollTop;
    // Native anchoring can precede ResizeObserver delivery during a reflow.
    if (frame(area) !== layout) schedule();
    else following = false;
  };
  const onPointerDown = () => {
    pressed = true;
  };
  const release = () => {
    if (!pressed) return;
    pressed = false;
    schedule();
  };
  const onCompositionStart = () => {
    composing = true;
  };
  const onCompositionEnd = () => {
    composing = false;
    onInput();
  };
  const observer = new ResizeObserver(() => {
    if (frame(area) !== layout) schedule();
  });
  observer.observe(area, { box: "border-box" });
  area.addEventListener("focus", onFocus);
  area.addEventListener("blur", onBlur);
  area.addEventListener("select", onSelect);
  area.addEventListener("input", onInput);
  area.addEventListener("wheel", onWheel, { passive: true });
  area.addEventListener("scroll", onScroll, { passive: true });
  area.addEventListener("pointerdown", onPointerDown);
  area.addEventListener("compositionstart", onCompositionStart);
  area.addEventListener("compositionend", onCompositionEnd);
  document.addEventListener("pointerup", release, true);
  document.addEventListener("pointercancel", release, true);
  window.addEventListener("resize", schedule);
  return () => {
    active = false;
    observer.disconnect();
    window.cancelAnimationFrame(pending);
    area.removeEventListener("focus", onFocus);
    area.removeEventListener("blur", onBlur);
    area.removeEventListener("select", onSelect);
    area.removeEventListener("input", onInput);
    area.removeEventListener("wheel", onWheel);
    area.removeEventListener("scroll", onScroll);
    area.removeEventListener("pointerdown", onPointerDown);
    area.removeEventListener("compositionstart", onCompositionStart);
    area.removeEventListener("compositionend", onCompositionEnd);
    document.removeEventListener("pointerup", release, true);
    document.removeEventListener("pointercancel", release, true);
    window.removeEventListener("resize", schedule);
  };
}
