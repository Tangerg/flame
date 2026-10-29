export interface VisualStyleMotion {
  instantMs: number;
  fastMs: number;
  mediumMs: number;
  slowMs: number;
  drawerMs: number;
  easeOut: readonly [number, number, number, number];
  drawerProgress: readonly [number, number, ...number[]];
  pressScale: number;
}

export const DEFAULT_MOTION: VisualStyleMotion = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  slowMs: 300,
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
  drawerProgress: [
    0, 0.06981, 0.21761, 0.38345, 0.53716, 0.66615, 0.76765, 0.84375, 0.89859, 0.93672, 0.96233,
    0.97894, 0.98929, 0.99544, 0.99887, 1.00061, 1.00135, 1.00152, 1.00142, 1.00119, 1,
  ],
  pressScale: 0.96,
};
