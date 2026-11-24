export interface DirectorySelection {
  readonly owner: object;
  readonly endpoint: string;
  readonly canBrowse: boolean;
  readonly path: string;
  readonly busy: boolean;
  readonly error: string | null;
  // The Project the created Session joins, captured when the picker opened.
  readonly projectId?: string;
}
export interface WorkingDirectoryAccess {
  open(projectId?: string): void;
  browse(expectedOwner: object): Promise<void>;
  useSelection(): DirectorySelection | null;
  selection(): DirectorySelection | null;
  update(
    expected: DirectorySelection,
    update: { path?: string; busy?: boolean; error?: string | null } | null,
  ): boolean;
}
