import {
  TextNode,
  type EditorConfig,
  type NodeKey,
  type SerializedTextNode,
  type Spread,
} from "lexical";
import { formatFileReference } from "@/features/composer/application/draftContext";

type SerializedFileReference = Spread<
  { filePath: string; type: "file-reference"; version: 1 },
  SerializedTextNode
>;
export class FileReferenceNode extends TextNode {
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
      throw new Error("invalid file reference checkpoint");
    return new FileReferenceNode(value.filePath).updateFromJSON(value).setMode("token");
  }
  override exportJSON(): SerializedFileReference {
    return { ...super.exportJSON(), type: "file-reference", version: 1, filePath: this.__filePath };
  }
  override createDOM(config: EditorConfig) {
    const element = super.createDOM(config);
    element.dataset.fileReference = this.__filePath;
    return element;
  }
  override isTextEntity() {
    return true;
  }
}
