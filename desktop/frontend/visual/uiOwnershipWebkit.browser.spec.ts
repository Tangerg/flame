import { test } from "./test";
import { registerUiOwnershipContracts } from "./uiOwnership.scenarios";

test.use({ browserName: "webkit" });
registerUiOwnershipContracts();
