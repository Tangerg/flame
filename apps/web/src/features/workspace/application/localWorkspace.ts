export interface LocalWorkspace {
  available(): boolean;
  open(cwd: string, path: string): Promise<boolean>;
  reveal(cwd: string, path: string): Promise<boolean>;
}
