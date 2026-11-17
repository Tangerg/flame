import {
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  $isElementNode,
  createEditor,
} from "lexical";
import { expect, it } from "vitest";
import { FileReferenceNode } from "./FileReferenceNode";
it("retains accepted references in a checkpoint without upgrading manually typed text", () => {
  const editor = createEditor({
    nodes: [FileReferenceNode],
    onError(error) {
      throw error;
    },
  });
  editor.update(
    () => {
      $getRoot().append(
        $createParagraphNode().append(
          $createTextNode("@src/manual.ts "),
          new FileReferenceNode("docs/my notes.md").setMode("token"),
        ),
      );
    },
    { discrete: true },
  );
  const checkpoint = JSON.stringify(editor.getEditorState().toJSON());
  editor.parseEditorState(checkpoint).read(() => {
    const paragraph = $getRoot().getFirstChildOrThrow();
    if (!$isElementNode(paragraph)) throw new Error("expected a paragraph");
    const nodes = paragraph.getChildren();
    expect(nodes[0]).not.toBeInstanceOf(FileReferenceNode);
    expect(nodes[1]).toBeInstanceOf(FileReferenceNode);
    expect($getRoot().getTextContent()).toBe('@src/manual.ts @"docs/my notes.md"');
  });
});
