import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const readSource = (path) => readFile(new URL(`../src/${path}`, import.meta.url), "utf8");
const [shell, iam, shared, edition] = await Promise.all([
  readSource("styles/foundation/shell.css"),
  readSource("astribot/iam/iam.css"),
  readSource("components/shared.tsx"),
  readSource("astribot/styles.css"),
]);

function zIndex(css, selector) {
  const rule = css.match(new RegExp(`^\\.${selector}\\s*\\{([^}]+)\\}`, "m"));
  const value = rule?.[1].match(/z-index:\s*(\d+);/);
  assert.ok(value, `Missing z-index for .${selector}`);
  return Number(value[1]);
}

test("IAM details cover the shell header and account dialogs cover the details", () => {
  const header = zIndex(shell, "topbar");
  const details = zIndex(iam, "iam-drawer-backdrop");
  const dialog = zIndex(iam, "iam-modal-backdrop");
  assert.ok(details > header);
  assert.ok(dialog > details);
});

test("IAM controls preserve the shared dropdown indicator", () => {
  const controls = iam.match(/^\.iam-page input, \.iam-page select\s*\{([^}]+)\}/m)?.[1];
  assert.ok(controls);
  assert.match(controls, /background-color:\s*var\(--iam-panel\)/);
  assert.doesNotMatch(controls, /\bbackground\s*:/);
});

test("the Astribot edition places RLark in the footer and Astribot in the sidebar", () => {
  const footer = shared.slice(shared.indexOf("export function PlatformFooter("), shared.indexOf("export type SortDirection"));
  const logo = shared.slice(shared.indexOf("export function Logo("), shared.indexOf("export function StatusBadge("));
  assert.match(logo, /const astribot = iamPortalEnabled\(\)/);
  assert.match(logo, /const brand = astribot \? "Astribot" : "RLark"/);
  assert.match(logo, /src=\{astribot \? astribotLight : [^}]+\}/);
  assert.match(logo, /src=\{astribot \? astribotDark : [^}]+\}/);
  assert.match(footer, /\{astribot && \([\s\S]+platform-footer-logo brand-logo-light[\s\S]+platform-footer-logo brand-logo-dark[\s\S]+RLark Open Source/);
  assert.match(edition, /\.platform-footer-astribot \.platform-footer-links\s*\{[^}]+flex-wrap: wrap/);
  assert.match(edition, /\.sidebar-collapsed \.brand-astribot\s*\{[^}]+--edition-logo-fit: cover/);
  assert.match(edition, /@media \(max-width: 780px\)\s*\{\s*\.sidebar \.brand-astribot\s*\{[^}]+--edition-logo-fit: cover/);
});
