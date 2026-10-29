import { createBackend } from "./backend";
import { fileURLToPath } from "node:url";
import { isFormatProcess, serveFormatProcess } from "./format-process";

if (isFormatProcess()) await serveFormatProcess();

export default createBackend(process.env, fileURLToPath(import.meta.url));
