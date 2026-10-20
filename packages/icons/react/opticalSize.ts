export function opticalSize(size: number): 16 | 24 {
  return size < 22 ? 16 : 24;
}
