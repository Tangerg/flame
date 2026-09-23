export interface Disposable {
  dispose: () => void;
}

export type ReadyHandler = () => void;
