export function bindClientPageLifecycle(view: Window, retire: () => void): () => void {
  const pagehide = (event: PageTransitionEvent) => {
    if (!event.persisted) retire();
  };
  view.addEventListener("pagehide", pagehide);
  return () => view.removeEventListener("pagehide", pagehide);
}

export function bindDraftExitWarning(
  view: Window,
  drafts: {
    hasUnconfirmedDrafts(): boolean;
    subscribePersistence(listener: () => void): () => void;
  },
): () => void {
  const warn = (event: BeforeUnloadEvent) => event.preventDefault();
  const synchronize = () => {
    if (drafts.hasUnconfirmedDrafts()) view.addEventListener("beforeunload", warn);
    else view.removeEventListener("beforeunload", warn);
  };
  const unsubscribe = drafts.subscribePersistence(synchronize);
  synchronize();
  return () => {
    unsubscribe();
    view.removeEventListener("beforeunload", warn);
  };
}
