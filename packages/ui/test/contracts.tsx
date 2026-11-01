import { Brain } from "@flame/icons/react";
import {
  Checkbox,
  DiffFileHeader,
  IconButton,
  PanelHeader,
  SearchField,
  Toolbar,
} from "../src/index";

void (<DiffFileHeader file={{ path: "src/reader.ts" }} />);

void (<IconButton icon={Brain} aria-label="Memory" />);
void (
  <IconButton
    icon={Brain}
    aria-labelledby="memory-label"
    title="Inspect memory"
  />
);
// @ts-expect-error A tooltip cannot supply the required accessible name.
void (<IconButton icon={Brain} title="Memory" />);
// @ts-expect-error Icon controls require a product-specific accessible name.
void (<IconButton icon={Brain} />);
const checkbox = { checked: false, onCheckedChange() {} };
void (<Checkbox {...checkbox} label="Pinned" />);
void (<Checkbox {...checkbox} ariaLabel="Select" />);
// @ts-expect-error A checkbox requires a visible label or an accessible name.
void (<Checkbox {...checkbox} />);
// @ts-expect-error A checkbox cannot have both naming modes.
void (<Checkbox {...checkbox} label="Pinned" ariaLabel="Select" />);
void (<SearchField value="" aria-label="Search" readOnly />);
// @ts-expect-error Search values have one controlled owner.
void (<SearchField defaultValue="uncontrolled" aria-label="Search" />);
// @ts-expect-error A missing value cannot describe clear-action availability.
void (<SearchField aria-label="Search" />);
void (<PanelHeader title="Agent memory" size="md" />);
void (
  <Toolbar
    label="Memory navigation"
    actions={<IconButton icon={Brain} aria-label="Inspect memory" />}
  >
    Current project
  </Toolbar>
);
