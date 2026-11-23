import {
  $createLineBreakNode,
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  $isElementNode,
  $isLineBreakNode,
  $isTextNode,
  TextNode,
  type EditorState,
  type LexicalNode,
  type NodeKey,
  type SerializedTextNode,
  type Spread,
} from "lexical";
import {
  DraftContent,
  formatFileReference,
  type DraftSegment,
} from "@/features/composer/domain/draft";

type SerializedFileReference = Spread<
  { filePath: string; type: "file-reference"; version: 1 },
  SerializedTextNode
>;

class FileReferenceNode extends TextNode {
  readonly __filePath: string;
  static override getType() {
    return "file-reference";
  }
  static override clone(node: FileReferenceNode) {
    return new FileReferenceNode(node.__filePath, node.__key);
  }
  constructor(path: string, key?: NodeKey) {
    super(formatFileReference(path), key);
    this.__filePath = path;
  }
  static override importJSON(value: SerializedFileReference) {
    if (typeof value.filePath !== "string" || !value.filePath)
      throw new Error("invalid serialized file reference");
    return $createFileReference(value.filePath).updateFromJSON(value);
  }
  override exportJSON(): SerializedFileReference {
    return { ...super.exportJSON(), type: "file-reference", version: 1, filePath: this.__filePath };
  }
  override isTextEntity() {
    return true;
  }
}

export const COMPOSER_NODES = [FileReferenceNode];

export function $createFileReference(path: string): FileReferenceNode {
  return new FileReferenceNode(path).setMode("token");
}

// Plain text is one paragraph: Lexical joins sibling paragraphs with a blank line.
export function $restoreContent(content: DraftContent): void {
  const paragraph = $createParagraphNode();
  for (const segment of content.segments) {
    if (segment.kind === "file") {
      paragraph.append($createFileReference(segment.path));
      continue;
    }
    segment.text.split("\n").forEach((line, index) => {
      if (index > 0) paragraph.append($createLineBreakNode());
      if (line) paragraph.append($createTextNode(line));
    });
  }
  const root = $getRoot();
  root.clear();
  root.append(paragraph);
  root.selectEnd();
}

export function readContent(state: EditorState): DraftContent {
  return state.read(() => {
    const segments: DraftSegment[] = [];
    const visit = (node: LexicalNode) => {
      if (node instanceof FileReferenceNode) segments.push({ kind: "file", path: node.__filePath });
      else if ($isLineBreakNode(node)) segments.push({ kind: "text", text: "\n" });
      else if ($isTextNode(node)) segments.push({ kind: "text", text: node.getTextContent() });
      else if ($isElementNode(node)) node.getChildren().forEach(visit);
    };
    $getRoot()
      .getChildren()
      .forEach((paragraph, index) => {
        if (index > 0) segments.push({ kind: "text", text: "\n" });
        visit(paragraph);
      });
    return DraftContent.of(segments);
  });
}
