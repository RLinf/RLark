import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const read = (relative) => readFile(new URL(relative, import.meta.url), "utf8");

// 数据录入类弹层：误点遮罩会丢失全部已填内容，因此遮罩不得关闭弹层。
const formModals = [
  "../src/pages/CreateJob.tsx",
  "../src/pages/Workflows.tsx",
  "../src/pages/Storage.tsx",
  "../src/pages/ImageRegistries.tsx",
  "../src/pages/SSHKeys.tsx",
  "../src/pages/Domains.tsx",
];

for (const relative of formModals) {
  test(`${relative} keeps form modals open when the backdrop is clicked`, async () => {
    const source = await read(relative);
    const backdrops = source.matchAll(
      /className="modal-backdrop[^"]*"([\s\S]{0,220}?)>/g,
    );
    for (const [, tail] of backdrops) {
      assert.doesNotMatch(tail, /onMouseDown/);
      assert.doesNotMatch(tail, /onClick/);
    }
  });
}

test("job tags editing keeps its modal open when the backdrop is clicked", async () => {
  const source = await read("../src/pages/Jobs.tsx");
  const [, tail] = source.match(
    /className="modal-backdrop job-tags-modal-backdrop"([\s\S]{0,220}?)>/,
  );
  assert.doesNotMatch(tail, /onMouseDown/);
});

test("confirmation dialogs still dismiss on backdrop click", async () => {
  const jobs = await read("../src/pages/Jobs.tsx");
  for (const backdrop of [
    "job-lifecycle-backdrop",
    "delete-job-backdrop",
    "restart-choice-backdrop",
  ]) {
    const [, tail] = jobs.match(
      new RegExp(`className="modal-backdrop ${backdrop}"([\\s\\S]{0,220}?)>`),
    );
    assert.match(tail, /onMouseDown/);
  }
  const clusters = await read("../src/pages/ClusterManagement.tsx");
  const [, clusterTail] = clusters.match(
    /className="modal-backdrop job-lifecycle-backdrop"([\s\S]{0,220}?)>/,
  );
  assert.match(clusterTail, /onMouseDown/);
});
