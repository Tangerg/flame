import { useEffect, useLayoutEffect, useRef, type RefObject } from "react";
import { useContentVisible } from "./content-visibility";

function availableFocusTarget(
  target: HTMLElement | null | undefined,
): target is HTMLElement {
  if (
    !target?.isConnected ||
    target.closest("[hidden], [inert], [aria-hidden='true']") ||
    target.matches(":disabled")
  )
    return false;
  const window = target.ownerDocument.defaultView!;
  for (let node: HTMLElement | null = target; node; node = node.parentElement) {
    const style = window.getComputedStyle(node);
    if (
      style.display === "none" ||
      style.visibility === "hidden" ||
      style.visibility === "collapse"
    )
      return false;
  }
  return true;
}

export function handoffFocus(
  source: HTMLElement,
  target: HTMLElement | null | undefined,
): boolean {
  if (
    !source.contains(source.ownerDocument.activeElement) ||
    target?.ownerDocument !== source.ownerDocument ||
    !availableFocusTarget(target)
  )
    return false;
  target.focus({ preventScroll: true });
  return target.ownerDocument.activeElement === target;
}

export class RemovalFocus {
  readonly #source: HTMLElement;
  readonly #node: HTMLElement;
  readonly #current: () => boolean;
  #settled = false;

  static capture(
    source: HTMLElement,
    current: () => boolean,
  ): RemovalFocus | null {
    const focused = source.ownerDocument.activeElement;
    if (
      !source.isConnected ||
      !focused ||
      !source.contains(focused) ||
      !current()
    )
      return null;
    return new RemovalFocus(source, focused as HTMLElement, current);
  }

  private constructor(
    source: HTMLElement,
    node: HTMLElement,
    current: () => boolean,
  ) {
    this.#source = source;
    this.#node = node;
    this.#current = current;
    node.ownerDocument.addEventListener("focusin", this.#laterFocus, true);
  }

  #laterFocus = (event: FocusEvent) => {
    if (event.target !== this.#node) this.retire();
  };

  retire = (): void => {
    this.#settled = true;
    this.#node.ownerDocument.removeEventListener(
      "focusin",
      this.#laterFocus,
      true,
    );
  };

  restore(resolve: () => HTMLElement | null | undefined): boolean {
    if (this.#settled) return false;
    this.retire();
    const document = this.#node.ownerDocument;
    if (
      this.#source.isConnected ||
      this.#node.isConnected ||
      !this.#current() ||
      document.activeElement !== document.body
    )
      return false;
    const target = resolve();
    if (target?.ownerDocument !== document || !availableFocusTarget(target))
      return false;
    target.focus({ preventScroll: true });
    return document.activeElement === target;
  }
}

export function useRemovalFocus(
  source: RefObject<HTMLElement | null>,
  target: () => HTMLElement | null | undefined,
  current: (source: HTMLElement) => boolean = () => true,
) {
  const visible = useContentVisible();
  const binding = useRef({ target, current, visible });
  const witness = useRef<RemovalFocus | null>(null);
  useLayoutEffect(() => {
    binding.current = { target, current, visible };
  });
  useLayoutEffect(() => {
    const node = source.current;
    return () => {
      witness.current?.retire();
      witness.current = node
        ? RemovalFocus.capture(
            node,
            () => binding.current.visible && binding.current.current(node),
          )
        : null;
    };
  }, [source]);
  useEffect(
    () => () => {
      // The opener's availability must commit before a removed form returns focus.
      const captured = witness.current;
      witness.current = null;
      captured?.restore(binding.current.target);
    },
    [source],
  );
}
