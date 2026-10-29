import type {} from "remark-parse";
import { math } from "micromark-extension-math";
import { mathFromMarkdown } from "mdast-util-math";
import { markdownLineEnding } from "micromark-util-character";
import { factorySpace } from "micromark-factory-space";
import { type Extension as FromMarkdownExtension, type Handle } from "mdast-util-from-markdown";
import { type Code, type Construct, type State } from "micromark-util-types";
import { type Processor } from "unified";

declare module "micromark-util-types" {
  interface TokenTypeMap {
    flameMathInline: "flameMathInline";
    flameMathDisplay: "flameMathDisplay";
    flameMathFlow: "flameMathFlow";
    flameMathMarker: "flameMathMarker";
    flameMathValue: "flameMathValue";
  }
}

const DELIMITERS = [
  { open: "\\(", close: "\\)", display: false },
  { open: "\\\\(", close: "\\\\)", display: false },
  { open: "\\[", close: "\\]", display: true },
  { open: "\\\\[", close: "\\\\]", display: true },
  { open: "[/inline]", close: "[/inline]", display: false },
  { open: "[/math]", close: "[/math]", display: true },
] as const;

function marker(value: string): Construct {
  return {
    partial: true,
    tokenize(effects, ok, nok) {
      let index = 0;
      return (code) => {
        effects.enter("flameMathMarker");
        return consume(code);
      };
      function consume(code: Code): ReturnType<State> {
        if (code !== value.charCodeAt(index)) return nok(code);
        effects.consume(code);
        index += 1;
        if (index < value.length) return consume;
        effects.exit("flameMathMarker");
        return ok;
      }
    },
  };
}

function delimitedMath(
  { open, close, display }: (typeof DELIMITERS)[number],
  flow: boolean,
): Construct {
  const type = flow ? "flameMathFlow" : display ? "flameMathDisplay" : "flameMathInline";
  return {
    name: type,
    concrete: flow,
    tokenize(effects, ok, nok) {
      const continuation: State = (code) =>
        flow && this.parser.lazy[this.now().line] ? nok(code) : content(code);
      let valueOpen = false;
      return start;
      function start(code: Code): ReturnType<State> {
        effects.enter(type);
        return effects.attempt(marker(open), content, nok)(code);
      }
      function content(code: Code): ReturnType<State> {
        if (code === null || (!display && markdownLineEnding(code))) return nok(code);
        if (markdownLineEnding(code)) {
          endValue();
          effects.enter("lineEnding");
          effects.consume(code);
          effects.exit("lineEnding");
          return continuation;
        }
        return code === close.charCodeAt(0)
          ? effects.check(marker(close), closing, data)(code)
          : data(code);
      }
      function data(code: Code): ReturnType<State> {
        if (!valueOpen) {
          effects.enter("flameMathValue");
          valueOpen = true;
        }
        effects.consume(code);
        return content;
      }
      function endValue(): void {
        if (!valueOpen) return;
        effects.exit("flameMathValue");
        valueOpen = false;
      }
      function closing(code: Code): ReturnType<State> {
        endValue();
        return effects.attempt(marker(close), closed, nok)(code);
      }
      function closed(code: Code): ReturnType<State> {
        effects.exit(type);
        return flow ? factorySpace(effects, after, "whitespace")(code) : ok(code);
      }
      function after(code: Code): ReturnType<State> {
        return code === null || markdownLineEnding(code) ? ok(code) : nok(code);
      }
    },
  };
}

const currencyDollar: Construct = {
  name: "currencyDollar",
  tokenize(effects, ok, nok) {
    return effects.check(
      {
        partial: true,
        tokenize(probe, yes, no) {
          return (code) => {
            probe.enter("data");
            probe.consume(code);
            return (next) => {
              probe.exit("data");
              return next !== null && next >= 48 && next <= 57 ? yes(next) : no(next);
            };
          };
        },
      },
      literal,
      nok,
    );
    function literal(code: Code): ReturnType<State> {
      effects.enter("data");
      effects.consume(code);
      effects.exit("data");
      return ok;
    }
  },
};

const enterMath: Handle = function (token) {
  this.enter({ type: token.type === "flameMathFlow" ? "math" : "inlineMath", value: "" }, token);
  this.buffer();
};

const exitMath: Handle = function (token) {
  const value = this.resume().trim();
  const node = this.stack.at(-1);
  if (node?.type !== "math" && node?.type !== "inlineMath") throw new Error("missing math node");
  node.value = value;
  const code = {
    type: "element" as const,
    tagName: "code",
    properties: {
      className: [
        "language-math",
        token.type === "flameMathInline" ? "math-inline" : "math-display",
      ],
    },
    children: [{ type: "text" as const, value: node.value }],
  };
  node.data =
    token.type === "flameMathFlow"
      ? { hName: "pre", hChildren: [code] }
      : { hName: "code", hProperties: code.properties, hChildren: code.children };
  this.exit(token);
};

const fromMarkdown: FromMarkdownExtension = {
  enter: {
    flameMathInline: enterMath,
    flameMathDisplay: enterMath,
    flameMathFlow: enterMath,
  },
  exit: {
    flameMathValue(token) {
      this.config.enter.data!.call(this, token);
      this.config.exit.data!.call(this, token);
    },
    flameMathInline: exitMath,
    flameMathDisplay: exitMath,
    flameMathFlow: exitMath,
  },
};

export function remarkMathSyntax(this: Processor): void {
  const data = this.data();
  const extensions = data.micromarkExtensions ?? (data.micromarkExtensions = []);
  const text: Record<number, Construct[]> = { 36: [currencyDollar] };
  const flow: Record<number, Construct[]> = {};
  for (const delimiter of DELIMITERS) {
    const code = delimiter.open.charCodeAt(0);
    (text[code] ??= []).push(delimitedMath(delimiter, false));
    if (delimiter.display) (flow[code] ??= []).push(delimitedMath(delimiter, true));
  }
  extensions.push(math(), { flow, text });
  const projections = data.fromMarkdownExtensions ?? (data.fromMarkdownExtensions = []);
  projections.push(mathFromMarkdown(), fromMarkdown);
}
