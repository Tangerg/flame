import { test } from "./test";
import { registerUiOwnershipContracts } from "./uiOwnership.scenarios";
import { registerComposerPasteContracts } from "./composerAttachments.scenarios";
test.use({ browserName: "webkit" });

registerUiOwnershipContracts();
registerComposerPasteContracts();
