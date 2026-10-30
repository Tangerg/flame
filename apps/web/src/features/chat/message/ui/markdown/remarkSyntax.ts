import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";
import remarkGfm from "remark-gfm";
import { remarkMathSyntax } from "./remarkMathSyntax";

export const remarkSyntax = [remarkGfm, remarkBreaks, remarkCjkFriendly, remarkMathSyntax];
