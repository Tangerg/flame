export async function createShikiWorker(): Promise<Worker> {
  const { default: ShikiWorker } = await import(
    /* @flame-lazy:syntax-highlighting */ "./shiki.worker?worker&inline"
  );
  return new ShikiWorker();
}
