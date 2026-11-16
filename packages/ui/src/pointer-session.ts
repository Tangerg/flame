export type PointerOutcome = "complete" | "cancel" | "retire";

interface PointerInput {
  button: number;
  isPrimary: boolean;
  pointerId: number;
  clientX: number;
  clientY: number;
}

interface PointerCallbacks {
  current(): boolean;
  move(delta: { x: number; y: number }): void;
  finish(outcome: PointerOutcome): void;
}

export class PointerSession {
  readonly #target: HTMLElement;
  readonly #window: Window;
  readonly #input: Pick<PointerInput, "pointerId" | "clientX" | "clientY">;
  readonly #callbacks: PointerCallbacks;
  #settled = false;

  static start(
    target: HTMLElement,
    input: PointerInput,
    callbacks: PointerCallbacks,
  ): PointerSession | null {
    if (input.button !== 0 || !input.isPrimary || !target.isConnected)
      return null;
    return new PointerSession(target, input, callbacks);
  }

  private constructor(
    target: HTMLElement,
    input: PointerInput,
    callbacks: PointerCallbacks,
  ) {
    const window = target.ownerDocument.defaultView;
    if (!window) throw new Error("pointer target must belong to a window");
    this.#target = target;
    this.#window = window;
    this.#input = {
      pointerId: input.pointerId,
      clientX: input.clientX,
      clientY: input.clientY,
    };
    this.#callbacks = callbacks;
    window.addEventListener("pointermove", this.#move);
    window.addEventListener("pointerup", this.#up);
    window.addEventListener("pointercancel", this.#cancelPointer);
    window.addEventListener("keydown", this.#keydown, true);
    window.addEventListener("blur", this.#cancel);
    window.addEventListener("pagehide", this.retire);
    target.addEventListener("lostpointercapture", this.#cancelPointer);
    target.addEventListener("blur", this.#cancel);
    target.ownerDocument.addEventListener("visibilitychange", this.#visibility);
    try {
      target.setPointerCapture(input.pointerId);
    } catch (error) {
      this.#settled = true;
      this.#detach();
      throw error;
    }
  }

  get active(): boolean {
    return !this.#settled;
  }

  retire = (): void => this.#finish("retire");

  #move = (event: PointerEvent) => {
    if (this.#settled || event.pointerId !== this.#input.pointerId) return;
    try {
      if (!this.#callbacks.current()) {
        this.retire();
        return;
      }
      this.#callbacks.move({
        x: event.clientX - this.#input.clientX,
        y: event.clientY - this.#input.clientY,
      });
    } catch (error) {
      this.retire();
      throw error;
    }
  };

  #up = (event: PointerEvent) => {
    if (event.pointerId === this.#input.pointerId) this.#finish("complete");
  };

  #cancelPointer = (event: PointerEvent) => {
    if (event.pointerId === this.#input.pointerId) this.#cancel();
  };

  #cancel = () => this.#finish("cancel");

  #keydown = (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;
    event.preventDefault();
    event.stopPropagation();
    this.#cancel();
  };

  #visibility = () => {
    if (this.#target.ownerDocument.visibilityState === "hidden") this.retire();
  };

  #detach() {
    this.#window.removeEventListener("pointermove", this.#move);
    this.#window.removeEventListener("pointerup", this.#up);
    this.#window.removeEventListener("pointercancel", this.#cancelPointer);
    this.#window.removeEventListener("keydown", this.#keydown, true);
    this.#window.removeEventListener("blur", this.#cancel);
    this.#window.removeEventListener("pagehide", this.retire);
    this.#target.removeEventListener("lostpointercapture", this.#cancelPointer);
    this.#target.removeEventListener("blur", this.#cancel);
    this.#target.ownerDocument.removeEventListener(
      "visibilitychange",
      this.#visibility,
    );
  }

  #finish(outcome: PointerOutcome) {
    if (this.#settled) return;
    if (outcome !== "retire") {
      try {
        if (!this.#callbacks.current()) outcome = "retire";
      } catch (error) {
        this.retire();
        throw error;
      }
    }
    // Releasing capture may synchronously reenter through lostpointercapture.
    this.#settled = true;
    this.#detach();
    try {
      if (this.#target.hasPointerCapture(this.#input.pointerId))
        this.#target.releasePointerCapture(this.#input.pointerId);
    } finally {
      this.#callbacks.finish(outcome);
    }
  }
}
