const HIGHLIGHTS_AVAILABLE = typeof CSS !== "undefined" && "highlights" in CSS;
const SVG_NAMESPACE = "http://www.w3.org/2000/svg";
const diagramMarkers = new WeakSet<Node>();
let activeDiagramMarker: SVGPathElement | null = null;

type MatchRect = { left: number; top: number; right: number; bottom: number };

function diagramMatchOutlines(range: Range, label: SVGTextElement, toDiagram: DOMMatrix): string[] {
  const piece = range.cloneRange();
  const texts = label.ownerDocument.createTreeWalker(label, NodeFilter.SHOW_TEXT);
  const fragments: MatchRect[] = [];
  for (let node = texts.nextNode(); node; node = texts.nextNode()) {
    if (!range.intersectsNode(node)) continue;
    const run = node.parentElement;
    if (!(run instanceof SVGTextContentElement)) continue;
    const { top, bottom, height } = run.getBoundingClientRect();
    if (!(height > 0)) continue;

    // WebKit gives SVG ranges the substring width but can put their top below the glyphs.
    piece.setStart(node, node === range.startContainer ? range.startOffset : 0);
    piece.setEnd(node, node === range.endContainer ? range.endOffset : (node as Text).length);
    for (const { left, right, width } of piece.getClientRects()) {
      if (width > 0) fragments.push({ left, right, top, bottom });
    }
  }

  // Stroke joined runs as one outline so styling boundaries inside a match stay invisible.
  fragments.sort((a, b) => a.top - b.top || a.bottom - b.bottom || a.left - b.left);
  const joined: MatchRect[] = [];
  for (const fragment of fragments) {
    const last = joined.at(-1);
    if (
      last &&
      last.top === fragment.top &&
      last.bottom === fragment.bottom &&
      fragment.left <= last.right
    ) {
      last.right = Math.max(last.right, fragment.right);
    } else {
      joined.push(fragment);
    }
  }

  const outlines: string[] = [];
  for (const { left, right, top, bottom } of joined) {
    const corners = [
      { x: left, y: top },
      { x: right, y: top },
      { x: right, y: bottom },
      { x: left, y: bottom },
    ].map((point) => toDiagram.transformPoint(point));
    if (corners.every(({ x, y }) => Number.isFinite(x) && Number.isFinite(y))) {
      outlines.push(`M ${corners.map(({ x, y }) => `${x} ${y}`).join(" L ")} Z`);
    }
  }
  return outlines;
}

function paintDiagramMatch(range: Range): SVGPathElement | null {
  const label = range.startContainer.parentElement?.closest("text");
  if (
    !(label instanceof SVGTextElement) ||
    !label.closest('[data-slot="mermaid-stage"]') ||
    !label.contains(range.endContainer) ||
    !(label.parentElement instanceof SVGGraphicsElement)
  ) {
    return null;
  }

  const toScreen = label.parentElement.getScreenCTM();
  if (!toScreen) return null;
  // WebKit still returns a legacy SVGMatrix at this boundary.
  const toDiagram = DOMMatrix.fromMatrix(toScreen).inverse();
  if (!toDiagram.is2D) return null;
  const outlines = diagramMatchOutlines(range, label, toDiagram);
  if (outlines.length === 0) return null;

  const marker = label.ownerDocument.createElementNS(SVG_NAMESPACE, "path");
  marker.setAttribute("d", outlines.join(" "));
  marker.setAttribute("aria-hidden", "true");
  marker.setAttribute("vector-effect", "non-scaling-stroke");
  marker.style.pointerEvents = "none";
  marker.style.fill = "var(--color-accent-wash)";
  marker.style.stroke = "var(--color-accent-text)";
  marker.style.strokeWidth = "1.5px";
  marker.style.strokeLinejoin = "round";
  diagramMarkers.add(marker);
  // The SVG paints the label after the marker, keeping the glyphs above the wash.
  label.before(marker);
  return marker;
}

export function isChatSearchHighlightMarker(node: Node): boolean {
  return diagramMarkers.has(node);
}

export function paintChatSearchHighlights(ranges: Range[], activeIndex: number): void {
  clearChatSearchHighlights();
  const active = ranges[activeIndex];
  if (active) activeDiagramMarker = paintDiagramMatch(active);
  if (!HIGHLIGHTS_AVAILABLE || ranges.length === 0) return;

  const inactive = ranges.filter((_, index) => index !== activeIndex);
  if (inactive.length > 0) {
    CSS.highlights.set("search-match", new Highlight(...inactive));
  }
  if (active) {
    CSS.highlights.set("search-match-active", new Highlight(active));
  }
}

export function clearChatSearchHighlights(): void {
  activeDiagramMarker?.remove();
  activeDiagramMarker = null;
  if (!HIGHLIGHTS_AVAILABLE) return;

  CSS.highlights.delete("search-match");
  CSS.highlights.delete("search-match-active");
}
