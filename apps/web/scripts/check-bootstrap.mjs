#!/usr/bin/env node
import { fileURLToPath } from "node:url";
import { checkPresentation } from "../../../tooling/presentation.mjs";

const repository = fileURLToPath(new URL("../../../", import.meta.url));
await checkPresentation(repository);
console.log("[check-bootstrap] presentation projections agree");
