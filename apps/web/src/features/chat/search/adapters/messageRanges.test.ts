import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { findMessageMatches } from "@/features/chat/search/adapters/messageRanges";
let inlineDefaults: HTMLStyleElement;
beforeEach(() => {
  // Happy DOM omits these user-agent inline defaults from computed styles.
  inlineDefaults = document.createElement("style");
  inlineDefaults.textContent = "strong, em, code, a, span { display: inline; }";
  document.head.append(inlineDefaults);
});
afterEach(() => {
  inlineDefaults.remove();
  document.body.replaceChildren();
});

function rangesFor(query: string, root?: ParentNode): Range[] {
  return findMessageMatches(query, root).map((match) => match.range);
}

function textOf(selector: string): Text {
  const node = document.querySelector(selector)?.firstChild;
  if (!(node instanceof Text)) throw new Error(`missing text in ${selector}`);
  return node;
}

describe("findMessageMatches", () => {
  it("keeps each occurrence tied to its rendered message", () => {
    document.body.innerHTML = `
      <article id="first" class="msg-content">alpha alpha</article>
      <article id="second" class="msg-content">alpha</article>
    `;

    const matches = findMessageMatches("alpha");

    expect(matches.map((match) => [match.message.id, match.occurrence])).toEqual([
      ["first", 0],
      ["first", 1],
      ["second", 0],
    ]);
    expect(matches.map((match) => match.range.toString())).toEqual(["alpha", "alpha", "alpha"]);
  });

  it("finds case-insensitive matches only inside message content", () => {
    document.body.innerHTML = `
      <article class="msg-content">Alpha beta ALPHA</article>
      <aside>alpha outside chat</aside>
    `;

    const ranges = rangesFor("alpha");

    expect(ranges.map((range) => range.toString())).toEqual(["Alpha", "ALPHA"]);
  });

  it("treats regex syntax as literal text", () => {
    document.body.innerHTML = `<article class="msg-content">a+b aab a+b</article>`;

    const ranges = rangesFor("a+b");

    expect(ranges.map((range) => range.toString())).toEqual(["a+b", "a+b"]);
  });

  it("returns no ranges for an empty query", () => {
    document.body.innerHTML = `<article class="msg-content">Alpha</article>`;

    expect(rangesFor("")).toEqual([]);
  });

  it("searches only message content under the given root", () => {
    document.body.innerHTML = `
      <section id="inside"><article class="msg-content">alpha inside</article></section>
      <section><article class="msg-content">alpha elsewhere</article></section>
    `;

    const root = document.querySelector("#inside");
    if (!root) throw new Error("missing root");
    const ranges = rangesFor("alpha", root);

    expect(ranges.map((range) => range.toString())).toEqual(["alpha"]);
    expect(root.contains(ranges[0]?.startContainer ?? null)).toBe(true);
  });

  it("matches a phrase split by strong and inline code in document order", () => {
    document.body.innerHTML = `
      <article class="msg-content">
        <p>The <strong id="owns">application owns</strong> <code id="policy">transaction policy</code> and recovery.</p>
        <p>Unrelated paragraph.</p>
        <p>The application <strong>owns transaction</strong> policy across the public boundary.</p>
        <p>Another unrelated paragraph.</p>
        <p>A final plain occurrence says application owns transaction policy.</p>
      </article>
    `;

    const ranges = rangesFor("Application Owns Transaction Policy");

    expect(ranges.map((range) => range.toString())).toEqual([
      "application owns transaction policy",
      "application owns transaction policy",
      "application owns transaction policy",
    ]);
    const [first] = ranges;
    expect(first?.startContainer).toBe(textOf("#owns"));
    expect(first?.startOffset).toBe(0);
    expect(first?.endContainer).toBe(textOf("#policy"));
    expect(first?.endOffset).toBe("transaction policy".length);
  });

  it("matches formatted Chinese text with exact endpoints", () => {
    document.body.innerHTML = `<article class="msg-content"><p id="line">连续<strong>查找</strong>与<code>代码</code>内容。</p></article>`;

    const ranges = rangesFor("连续查找与代码内容");

    expect(ranges.map((range) => range.toString())).toEqual(["连续查找与代码内容"]);
    const line = document.querySelector("#line");
    expect(ranges[0]?.startContainer).toBe(line?.firstChild);
    expect(ranges[0]?.startOffset).toBe(0);
    expect(ranges[0]?.endContainer).toBe(line?.lastChild);
    expect(ranges[0]?.endOffset).toBe(2);
  });

  it("matches across nested, empty and display contents inline nodes", () => {
    document.body.innerHTML = `
      <article class="msg-content">
        <p><a href="#"><em id="head">ne</em><span></span></a><span style="display: contents"><span>s</span></span><span>ted</span> word</p>
      </article>
    `;
    document.querySelector("a")?.append(document.createTextNode(""));

    const ranges = rangesFor("nested word");

    expect(ranges.map((range) => range.toString())).toEqual(["nested word"]);
    expect(ranges[0]?.startContainer).toBe(textOf("#head"));
    expect(ranges[0]?.startOffset).toBe(0);
    expect(ranges[0]?.endContainer).toBe(document.querySelector("p")?.lastChild);
    expect(ranges[0]?.endOffset).toBe(" word".length);
  });

  it("keeps split occurrences ordered and non-overlapping", () => {
    document.body.innerHTML = `<article class="msg-content"><p>a<strong id="middle">aa</strong>a</p></article>`;

    const ranges = rangesFor("aa");

    expect(ranges.map((range) => range.toString())).toEqual(["aa", "aa"]);
    expect(ranges[0]?.endContainer).toBe(textOf("#middle"));
    expect(ranges[0]?.endOffset).toBe(1);
    expect(ranges[1]?.startContainer).toBe(textOf("#middle"));
    expect(ranges[1]?.startOffset).toBe(1);
  });

  it("treats punctuation across formatting as literal text", () => {
    document.body.innerHTML = `<article class="msg-content"><p><code>a.b</code>(x) axb(x) a.<em>b(x)</em></p></article>`;

    const ranges = rangesFor("a.b(x)");

    expect(ranges.map((range) => range.toString())).toEqual(["a.b(x)", "a.b(x)"]);
  });

  it("keeps UTF-16 endpoints exact around emoji and partial text nodes", () => {
    document.body.innerHTML = `<article class="msg-content"><p id="unicode">🧭alph<strong>a🧩be</strong><code id="unicode-end">taX</code></p></article>`;

    const ranges = rangesFor("ALPHA🧩BETA");

    expect(ranges.map((range) => range.toString())).toEqual(["alpha🧩beta"]);
    expect(ranges[0]?.startContainer).toBe(textOf("#unicode"));
    expect(ranges[0]?.startOffset).toBe(2);
    expect(ranges[0]?.endContainer).toBe(textOf("#unicode-end"));
    expect(ranges[0]?.endOffset).toBe(2);
  });

  it("does not bridge paragraphs, line breaks, block overrides or messages", () => {
    document.body.innerHTML = `
      <article class="msg-content">
        <p>Boundary tail</p><p>head marker</p>
        <p>Break tail<br>head marker</p>
        <div><span style="display: block">Block tail</span><strong>head marker</strong></div>
        <ul><li>List tail</li><li>head marker</li></ul>
      </article>
      <article class="msg-content">Message tail</article><article class="msg-content">head marker</article>
    `;

    expect(rangesFor("tailhead")).toEqual([]);
    expect(rangesFor("tail head")).toEqual([]);
    expect(rangesFor("head marker")).toHaveLength(5);
  });

  it("continues a phrase through block elements styled inline", () => {
    document.body.innerHTML = `<article class="msg-content"><div style="display: inline">Inline tail</div><div style="display: inline">head marker</div></article>`;

    const ranges = rangesFor("tailhead");

    expect(ranges.map((range) => range.toString())).toEqual(["tailhead"]);
  });
});
