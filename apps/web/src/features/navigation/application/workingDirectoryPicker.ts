export interface DirectorySelection {
  readonly owner: object;
  readonly endpoint: string;
  readonly canBrowse: boolean;
  readonly path: string;
  readonly busy: boolean;
  readonly error: string | null;
}
export interface WorkingDirectoryAccess {
  open(): void;
  browse(expectedOwner: object): Promise<void>;
  useSelection(): DirectorySelection | null;
  selection(): DirectorySelection | null;
  update(
    expected: DirectorySelection,
    update: { path?: string; busy?: boolean; error?: string | null } | null,
  ): boolean;
}
