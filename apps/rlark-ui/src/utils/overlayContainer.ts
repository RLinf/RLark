/**
 * Returns the DOM node that overlays (popovers, tooltips, dropdowns, modals)
 * must be portalled into.
 *
 * Overlays are portalled so that their `position: fixed` box escapes the
 * clipping and stacking contexts of the surrounding table or card. The portal
 * target still has to live inside `.app-shell`, because that element carries
 * the `theme-light` / `theme-dark` class which both supplies the theme CSS
 * variables and scopes every `.theme-dark …` rule. Portalling into
 * `document.body` puts the overlay outside that subtree, so it keeps the light
 * palette while the rest of the page is dark.
 */
export function overlayContainer(): HTMLElement {
  return document.querySelector<HTMLElement>(".app-shell") ?? document.body;
}
