import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { copyText } from "../dist/test/utils/clipboard.js";

const sshKeysSource = await readFile(
  new URL("../src/pages/SSHKeys.tsx", import.meta.url),
  "utf8",
);

// 控制台里所有复制入口都必须经过 utils/clipboard 的 copyText，否则在 HTTP
// 部署（非安全上下文）下 navigator.clipboard 为 undefined，复制会直接失效。
const copyEntrySources = Object.fromEntries(
  await Promise.all(
    [
      "pages/Api.tsx",
      "pages/Clusters.tsx",
      "pages/Jobs.tsx",
      "pages/SSHKeys.tsx",
      "pages/SystemConfig.tsx",
      "pages/Workflows.tsx",
      "admin/CreateCluster.tsx",
    ].map(async (relative) => [
      relative,
      await readFile(new URL(`../src/${relative}`, import.meta.url), "utf8"),
    ]),
  ),
);

// installDom 用最小 DOM/navigator 替身模拟两类部署：
// clipboard 为 null 表示非安全上下文（HTTP 访问控制台），此时浏览器不暴露
// navigator.clipboard；execCommandResult 控制回退路径的返回值。
function installDom({ clipboard = null, execCommandResult = true } = {}) {
  const calls = { writes: [], commands: [], appended: 0, removed: 0 };
  const element = {
    value: "",
    style: {},
    focus() {},
    select() {},
    setSelectionRange() {},
    remove() {
      calls.removed += 1;
    },
  };
  const originalDocument = globalThis.document;
  const originalNavigator = Object.getOwnPropertyDescriptor(
    globalThis,
    "navigator",
  );

  globalThis.document = {
    createElement: () => element,
    body: {
      appendChild() {
        calls.appended += 1;
      },
    },
    execCommand(command) {
      calls.commands.push({ command, value: element.value });
      return execCommandResult;
    },
  };
  Object.defineProperty(globalThis, "navigator", {
    value: clipboard
      ? {
          clipboard: {
            async writeText(value) {
              return clipboard(value, calls);
            },
          },
        }
      : {},
    configurable: true,
    writable: true,
  });

  return {
    calls,
    restore() {
      if (originalDocument === undefined) delete globalThis.document;
      else globalThis.document = originalDocument;
      if (originalNavigator) {
        Object.defineProperty(globalThis, "navigator", originalNavigator);
      } else {
        delete globalThis.navigator;
      }
    },
  };
}

test("安全上下文下 copyText 走 Clipboard API", async () => {
  const dom = installDom({
    clipboard: (value, calls) => {
      calls.writes.push(value);
    },
  });
  try {
    assert.equal(await copyText("ssh-ed25519 AAAA"), true);
    assert.deepEqual(dom.calls.writes, ["ssh-ed25519 AAAA"]);
    assert.equal(dom.calls.commands.length, 0);
  } finally {
    dom.restore();
  }
});

test("navigator.clipboard 缺失时回退到 execCommand", async () => {
  const dom = installDom();
  try {
    assert.equal(await copyText("ssh-rsa BBBB"), true);
    assert.deepEqual(dom.calls.commands, [
      { command: "copy", value: "ssh-rsa BBBB" },
    ]);
    // 临时 textarea 必须挂载后再移除，避免污染页面。
    assert.equal(dom.calls.appended, 1);
    assert.equal(dom.calls.removed, 1);
  } finally {
    dom.restore();
  }
});

test("Clipboard API 抛错时同样回退到 execCommand", async () => {
  const dom = installDom({
    clipboard: () => {
      throw new Error("NotAllowedError");
    },
  });
  try {
    assert.equal(await copyText("ssh-ed25519 CCCC"), true);
    assert.deepEqual(dom.calls.commands, [
      { command: "copy", value: "ssh-ed25519 CCCC" },
    ]);
  } finally {
    dom.restore();
  }
});

test("回退路径失败时 copyText 返回 false", async () => {
  const dom = installDom({ execCommandResult: false });
  try {
    assert.equal(await copyText("ssh-ed25519 DDDD"), false);
    assert.equal(dom.calls.removed, 1);
  } finally {
    dom.restore();
  }
});

test("空文本不触碰剪贴板", async () => {
  const dom = installDom();
  try {
    assert.equal(await copyText(""), false);
    assert.equal(dom.calls.commands.length, 0);
    assert.equal(dom.calls.appended, 0);
  } finally {
    dom.restore();
  }
});

test("SSH 公钥页的两处复制都使用带回退的 copyText", () => {
  assert.doesNotMatch(sshKeysSource, /navigator\.clipboard/);
  assert.match(sshKeysSource, /import \{ copyText \} from "\.\.\/utils\/clipboard"/);
  assert.match(sshKeysSource, /if \(!\(await copyText\(sshCommand\)\)\) return;/);
  assert.match(sshKeysSource, /if \(!\(await copyText\(publicKey\)\)\) return;/);
});

test("SSH 公钥页复制成功后复用 copy-confirmed 动画", () => {
  assert.match(
    sshKeysSource,
    /copied \? "icon-button copy-confirmed" : "icon-button"/,
  );
  assert.match(
    sshKeysSource,
    /isCopied\s*\n\s*\? "icon-button copy-confirmed"\s*\n\s*: "icon-button"/,
  );
});

test("所有复制入口都改用 copyText 且不再直接调用 navigator.clipboard", () => {
  for (const [relative, source] of Object.entries(copyEntrySources)) {
    assert.doesNotMatch(
      source,
      /navigator\.clipboard/,
      `${relative} 仍直接调用 navigator.clipboard`,
    );
    assert.match(
      source,
      /import \{ copyText \} from "\.\.\/utils\/clipboard"/,
      `${relative} 未引入 copyText`,
    );
  }
});

test("复制成功态只在 copyText 返回 true 时置位", () => {
  // Api / SystemConfig / Clusters / CreateCluster / Workflows / Jobs 都必须先
  // 判定复制结果，避免在回退也失败时仍显示"已复制"。
  assert.match(copyEntrySources["pages/Api.tsx"], /if \(!ok\) return;/);
  assert.match(
    copyEntrySources["pages/SystemConfig.tsx"],
    /if \(!\(await copyText\(sshCommand\)\)\) return;/,
  );
  assert.equal(
    copyEntrySources["pages/Clusters.tsx"].match(
      /if \(!\(await copyText\(sshCommand\)\)\) return;/g,
    )?.length,
    2,
  );
  assert.match(
    copyEntrySources["admin/CreateCluster.tsx"],
    /if \(!\(await copyText\(deployYaml\)\)\) return;/,
  );
  assert.match(
    copyEntrySources["admin/CreateCluster.tsx"],
    /if \(!\(await copyText\(yaml\)\)\) return;/,
  );
  assert.match(
    copyEntrySources["pages/Jobs.tsx"],
    /if \(!\(await copyText\(lines\.join\("\\n"\)\)\)\) return;/,
  );
});

test("工作流 YAML 复制补齐了成功反馈", () => {
  const source = copyEntrySources["pages/Workflows.tsx"];
  assert.match(source, /if \(!\(await copyText\(yaml\)\)\) return;/);
  assert.match(source, /setYamlCopied\(true\);/);
  assert.match(source, /yamlCopied \? \(zh \? "已复制" : "Copied"\) : c\.api\.copy/);
});
