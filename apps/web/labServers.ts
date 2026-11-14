const host = "127.0.0.1";

function labServer(name: string, port: number) {
  return Object.freeze({ name, host, port, origin: `http://${host}:${port}` });
}

export const LAB_DEVELOPMENT_SERVER = labServer("Lab development", 4174);
export const LAB_PREVIEW_SERVER = labServer("Lab preview", 4175);
