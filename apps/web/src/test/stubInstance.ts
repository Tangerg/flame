/** Replace methods on the exact object supplied by this test; no module binding is installed. */
export function stubInstance<T extends object>(target: T, replacements: Partial<T>): () => void {
  const before = new Map<keyof T, PropertyDescriptor | undefined>();
  for (const key of Object.keys(replacements) as Array<keyof T>) {
    before.set(key, Object.getOwnPropertyDescriptor(target, key));
    Object.defineProperty(target, key, {
      configurable: true,
      enumerable: true,
      writable: true,
      value: replacements[key],
    });
  }
  return () => {
    for (const [key, descriptor] of before) {
      if (descriptor) Object.defineProperty(target, key, descriptor);
      else Reflect.deleteProperty(target, key);
    }
  };
}
