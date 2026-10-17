import { Brain } from "@flame/icons/react";
import { IconButton, SearchField } from "../src/index";

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
void (<SearchField value="" aria-label="Search" readOnly />);
// @ts-expect-error Search values have one controlled owner.
void (<SearchField defaultValue="uncontrolled" aria-label="Search" />);
// @ts-expect-error A missing value cannot describe clear-action availability.
void (<SearchField aria-label="Search" />);
