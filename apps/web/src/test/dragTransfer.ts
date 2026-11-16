export function dragTransfer(files: readonly File[] = []): DataTransfer {
  const data = new Map<string, string>();
  return {
    effectAllowed: "all",
    dropEffect: "none",
    get types() {
      return [...(files.length ? ["Files"] : []), ...data.keys()];
    },
    files: files as unknown as FileList,
    items: files.map((file) => ({
      kind: "file",
      type: file.type,
      getAsFile: () => file,
    })) as unknown as DataTransferItemList,
    setData: (type, value) => {
      data.set(type, value);
    },
    getData: (type) => data.get(type) ?? "",
    clearData: (type) => {
      if (type) data.delete(type);
      else data.clear();
    },
    setDragImage() {},
  };
}
