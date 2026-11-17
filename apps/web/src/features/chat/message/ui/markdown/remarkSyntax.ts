import type { PluggableList } from "unified";
import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";

export const remarkSyntax: PluggableList = [
  remarkGfm,
  remarkBreaks,
  remarkCjkFriendly,
  [remarkMath, { singleDollarTextMath: false }],
];
