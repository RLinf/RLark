import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { readStyles } from "./read-styles.mjs";

const overlayContainerSource = await readFile(
  new URL("../src/utils/overlayContainer.ts", import.meta.url),
  "utf8",
);
const portalSources = await Promise.all(
  ["../src/pages/Jobs.tsx", "../src/components/TagEditor.tsx"].map(
    async (path) => [
      path,
      await readFile(new URL(path, import.meta.url), "utf8"),
    ],
  ),
);

test("overlays are portalled into the themed app shell, not the body", () => {
  // `.app-shell` carries the theme-light / theme-dark class, which both
  // supplies the theme CSS variables and scopes every `.theme-dark …` rule.
  // Portalling into document.body puts the overlay outside that subtree, so it
  // renders with the light palette while the rest of the page is dark.
  assert.match(
    overlayContainerSource,
    /document\.querySelector<HTMLElement>\("\.app-shell"\) \?\? document\.body/,
  );
});

test("no component portals an overlay straight into document.body", () => {
  for (const [path, source] of portalSources) {
    for (const [, target] of source.matchAll(
      /createPortal\(\s*(?:[\s\S]*?),\s*([^,()\s][^,]*?),?\s*\)/g,
    )) {
      assert.doesNotMatch(
        target,
        /document\.body/,
        `${path} still portals an overlay into document.body`,
      );
    }
  }
  for (const [path, source] of portalSources) {
    assert.match(
      source,
      /import \{ overlayContainer \} from "\.\.\/utils\/overlayContainer";/,
      `${path} must resolve its portal target through overlayContainer()`,
    );
  }
});

test("overlay surfaces take their colours from the theme variables", async () => {
  const styles = await readStyles();
  // The job tag popover used to hardcode a white background and a light grey
  // border, which stayed light even once the portal target was fixed.
  assert.match(
    styles,
    /\.job-tag-popover \{[^}]*background: var\(--panel\);[^}]*border: 1px solid var\(--line-strong\);/,
  );
  assert.match(
    styles,
    /\.job-tag-chip \{[^}]*border: 1px solid var\(--line-strong\);/,
  );
  assert.doesNotMatch(styles, /var\(--card-bg/);
});

test("light-only accent surfaces inside overlays have a dark-theme override", async () => {
  const styles = await readStyles();
  assert.match(styles, /\.theme-dark \.tag-editor-error \{/);
  assert.match(styles, /\.theme-dark \.job-tag-chip-inline \{/);
});
