import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { readStyles } from "./read-styles.mjs";

const popoverSource = await readFile(
  new URL("../src/components/TagFilterPopover.tsx", import.meta.url),
  "utf8",
);
const jobsSource = await readFile(
  new URL("../src/pages/Jobs.tsx", import.meta.url),
  "utf8",
);

test("the tag filter still sends one key=value pair per selected value", () => {
  // The gateway re-groups the flattened pairs by key, so the serialization has
  // to stay a flat comma-separated list for the AND/OR split to apply.
  assert.match(
    jobsSource,
    /Object\.entries\(tagFilter\)\s*\.flatMap\(\(\[key, values\]\) => values\.map\(\(value\) => `\$\{key\}=\$\{value\}`\)\)\s*\.join\(","\)/,
  );
});

test("the popover states that keys are ANDed and values are ORed", () => {
  assert.match(
    popoverSource,
    /多个标签 key 之间为「且」，同一 key 下的多个值之间为「或」。/,
  );
  assert.match(
    popoverSource,
    /Different keys are ANDed; values within the same key are ORed\./,
  );
});

test("no control selects every key at once", () => {
  // Selecting all keys requires one job to carry all of them, which returns an
  // empty list on any realistic data set. The only bulk control left operates
  // on the values of a single key, where the backend ORs the selection.
  assert.doesNotMatch(popoverSource, /toggleSelectAllKeys/);
  assert.equal(
    popoverSource.match(/\{zh \? "全选\/取消" : "All \/ None"\}/g)?.length,
    1,
  );
  assert.match(
    popoverSource,
    /onClick=\{\(\) => toggleKeyAll\(effectiveKey\)\}/,
  );
  // The bulk control is hidden until a key with values is in focus, otherwise
  // it would have no target.
  assert.match(
    popoverSource,
    /\{effectiveKey && selectedValues\.length > 0 && \(/,
  );
});

test("the key column keeps a static header instead of a bulk action", () => {
  assert.match(popoverSource, /\{zh \? "标签 key" : "Keys"\}/);
});

test("only a click moves the focus between tag keys", () => {
  // Hover-driven focus made the value column follow the pointer, so every key
  // the pointer crossed on its way to the values replaced the column content
  // and the values of the key below an already-selected one could never be
  // ticked. The row is now a checkbox (select all) plus a button (focus).
  assert.doesNotMatch(popoverSource, /onMouseEnter|onMouseLeave/);
  assert.match(
    popoverSource,
    /className="tag-filter-key-main"\s*aria-pressed=\{effectiveKey === k\}\s*onClick=\{\(\) => setActiveKey\(k\)\}/,
  );
  assert.match(popoverSource, /const \[activeKey, setActiveKey\] = useState/);
  // The selected-key fallback only applies while no key is focused, or after a
  // search removed the focused key from the list.
  assert.match(
    popoverSource,
    /activeKey && filteredKeys\.includes\(activeKey\)\s*\?\s*activeKey\s*:\s*\(filteredKeys\.find\(keySelected\) \?\? null\)/,
  );
  // The empty value column tells the user to click rather than to hover.
  assert.match(popoverSource, /点击左侧的 key 查看对应的值/);
  assert.match(popoverSource, /Click a key on the left to see its values/);
});

test("the focused key and the selected keys are styled apart", async () => {
  const styles = await readStyles();
  // --blue-bg used to be undefined, so the selected rows fell back to a light
  // lavender that stayed light in the dark theme and swallowed the label.
  assert.match(styles, /\.theme-dark \{[^}]*--blue-bg:/);
  assert.doesNotMatch(styles, /var\(--blue-bg,/);
  assert.match(
    styles,
    /\.tag-filter-key-item\.active \{\s*border-right-color: var\(--blue\);/,
  );
});

test("the column header lays out the title and the bulk action side by side", async () => {
  const styles = await readStyles();
  assert.match(
    styles,
    /\.tag-filter-col-head \{[^}]*justify-content: space-between;/,
  );
  // Long tag keys must not push the bulk action out of the popover.
  assert.match(
    styles,
    /\.tag-filter-col-title \{[^}]*text-overflow: ellipsis;/,
  );
  assert.match(styles, /\.tag-filter-hint \{/);
});
