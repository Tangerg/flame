import { Presentation } from "../../../../plugins/genui/contracts/document";
import { interfaceRequest } from "../../../../plugins/genui/contracts/fixture";
const request = interfaceRequest(true);
request.title = "Agent plugin delivery review";
request.messages[0]!.createSurface.dataModel.data.title = "Captured delivery review";
export const GENUI_REPORT = Presentation.accept(request, []).snapshot();
