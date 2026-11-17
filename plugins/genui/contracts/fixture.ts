import { CATALOG_ID } from "./catalog";
export function interfaceRequest(withDialog = false) {
  return {
    title: "Release review",
    operations: [],
    messages: [
      {
        version: "1.0",
        createSurface: {
          surfaceId: "review",
          catalogId: CATALOG_ID,
          sendDataModel: false,
          dataModel: {
            data: { title: "Release 9007199254740993" },
            draft: { note: "original", portal: "" },
          },
          components: [
            {
              id: "root",
              component: "Column",
              children: [
                "title",
                "note",
                "send",
                ...(withDialog ? ["dialog"] : []),
              ],
            },
            {
              id: "title",
              component: "Text",
              text: { "@path": "/data/title" },
            },
            {
              id: "note",
              component: "TextField",
              label: "Review note",
              value: { "@path": "/draft/note" },
            },
            {
              id: "send",
              component: "Button",
              child: "caption",
              action: {
                event: {
                  name: "followup",
                  context: {
                    message: "Review this",
                    payload: { note: { "@path": "/draft/note" } },
                  },
                },
              },
            },
            { id: "caption", component: "Text", text: "Send review" },
            ...(withDialog
              ? [
                  {
                    id: "dialog",
                    component: "Dialog",
                    label: "Open captured dialog",
                    child: "portalNote",
                  },
                  {
                    id: "portalNote",
                    component: "TextField",
                    label: "Portal note",
                    value: { "@path": "/draft/portal" },
                  },
                ]
              : []),
          ],
        },
      },
    ],
  };
}
