import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";

export const remarkSyntax = [remarkGfm, remarkBreaks, remarkCjkFriendly, remarkMath];
