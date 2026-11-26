import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import {
  defaultTreeAdapter as tree,
  parseFragment,
  serialize,
  type DefaultTreeAdapterTypes,
} from "parse5";
import { TooltipProvider } from "@flame/ui";
import { expect, it, vi } from "vitest";
import { Chip } from "@/ui/atoms/chip";

vi.mock("@/ui/icons/index", () => ({ Icon: () => null }));

function elements(parent: DefaultTreeAdapterTypes.ParentNode): DefaultTreeAdapterTypes.Element[] {
  return parent.childNodes.flatMap((node) =>
    tree.isElementNode(node) ? [node, ...elements(node)] : [],
  );
}

it("labels each review and remove button with its own visible chip value", () => {
  const first = "recipe.txt:1177-1179";
  const second = "recipe.txt:1377-1379";
  const markup = renderToStaticMarkup(
    createElement(TooltipProvider, {
      children: [first, second].map((value) =>
        createElement(Chip, {
          key: value,
          children: value,
          kind: "attached",
          title: "Review reference",
          closeLabel: "Remove reference",
          onOpen: () => {},
          onClose: () => {},
        }),
      ),
    }),
  );

  const nodes = elements(parseFragment(markup));
  const labelledText = new Map(
    nodes.flatMap((node) => {
      const id = node.attrs.find((attr) => attr.name === "id");
      return id ? [[id.value, serialize(node)] as const] : [];
    }),
  );
  const buttons = nodes.filter((element) => element.tagName === "button");
  const labels = buttons.map((button) => {
    const ids = button.attrs.find((attr) => attr.name === "aria-labelledby")?.value.split(" ");
    return ids?.map((id) => labelledText.get(id)).join(" ");
  });

  expect(labels).toEqual([
    first,
    `Remove reference ${first}`,
    second,
    `Remove reference ${second}`,
  ]);
});
