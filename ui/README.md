# Flame UI

`@flame/ui` owns the shared presentation vocabulary used by Desktop and portable views: Base UI interaction primitives, token-dressed controls, typography, icon layout, document focus rules and compact panel composition, record disclosures, facts, metrics and readable tables. `@flame/icons` remains the owner of drawings, optical masters, SVG exports and discovery metadata. Product icon-name mappings remain with their consumer.

Appearance preferences and theme selection belong to the application. The library receives a controlled font scale and a read-only `ThemeProjection`; it never chooses or persists a theme. The isolated carrier applies projections through a generated bootstrap before starting the guest and updates the existing document over its admitted port. Projection updates cannot request business data or submit commands.

```tsx
import { Button, IconButton, FontScaleProvider } from "@flame/ui";
import { Brain } from "@flame/icons/react";
import "@flame/ui/styles.css";

<FontScaleProvider fontSize={14}>
  <Button variant="primary">Review</Button>
  <IconButton icon={Brain} title="Memory" />
</FontScaleProvider>;
```

The build emits JavaScript, declarations and one stylesheet. Ordinary consumers need no StyleX transform or application aliases. StyleX consumers may import the shared token declarations from `@flame/ui/tokens.stylex`; those declarations are compiled by their existing StyleX pipeline. Component overrides use the existing `styles` contract so each element merges its styles in one `stylex.props` call.

```sh
npm ci --prefix icons
npm run build --prefix icons
npm ci --prefix ui
npm run check --prefix ui
```

The package is private and is consumed through repository-local `file:` dependencies. Package verification installs locally packed UI and icon archives into an independent consumer. It never publishes to npm. Source owns styling; generated `dist` and token-name outputs are not committed. Changes to shared components require the Desktop gate and the portable resource build and carrier acceptance.

## Review preview

The Desktop visual fixture renders the library with the real Flame appearance owner. Run `npm run visual:dev --prefix desktop/frontend` from the repository root and open `/visual/?fixture=atoms&theme=light` or the dark counterpart. The shared-panel specimen includes the Memory toolbar, status filter and record row. This preview uses an isolated in-memory transport and does not contact a user Runtime.
