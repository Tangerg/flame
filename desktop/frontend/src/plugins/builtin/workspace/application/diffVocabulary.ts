export const DIFF_MODES = ["worktree", "base"] as const;
export type WorkspaceDiffMode = (typeof DIFF_MODES)[number];

export const DIFF_LAYOUTS = ["unified", "split"] as const;
export type DiffLayout = (typeof DIFF_LAYOUTS)[number];
