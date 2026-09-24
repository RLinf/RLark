import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { readStyles } from "./read-styles.mjs";

const readSource = (path) =>
  readFile(new URL(`../src/${path}`, import.meta.url), "utf8");

const [
  domainsSource,
  workflowsSource,
  clusterManagementSource,
  clustersSource,
  overviewSource,
  adminDashboardSource,
  adminPageSource,
  sshKeysSource,
  imageRegistriesSource,
  storageSource,
  systemConfigSource,
  stylesSource,
] = await Promise.all([
  readSource("pages/Domains.tsx"),
  readSource("pages/Workflows.tsx"),
  readSource("pages/ClusterManagement.tsx"),
  readSource("pages/Clusters.tsx"),
  readSource("pages/Overview.tsx"),
  readSource("admin/AdminDashboard.tsx"),
  readSource("admin/AdminPage.tsx"),
  readSource("pages/SSHKeys.tsx"),
  readSource("pages/ImageRegistries.tsx"),
  readSource("pages/Storage.tsx"),
  readSource("pages/SystemConfig.tsx"),
  readStyles(),
]);

test("resource lists expose controlled refresh state and mask their tables", () => {
  for (const source of [
    domainsSource,
    workflowsSource,
    clusterManagementSource,
  ]) {
    assert.match(
      source,
      /const \[refreshing, setRefreshing\] = useState\(false\)/,
    );
    assert.match(source, /refreshing=\{refreshing\}/);
    assert.match(source, /refreshable-region/);
    assert.match(source, /aria-busy=\{refreshing\}/);
    assert.match(source, /<RefreshOverlay/);
  }
});

test("dashboard-wide refreshes mask data while keeping their headers visible", () => {
  for (const source of [
    clustersSource,
    overviewSource,
    adminDashboardSource,
    adminPageSource,
    systemConfigSource,
  ]) {
    assert.match(source, /page-refresh-region/);
    assert.match(source, /<RefreshOverlay/);
  }

  assert.match(
    stylesSource,
    /\.refreshable-region\.page-refresh-region > \.section-heading/,
  );
  assert.match(stylesSource, /z-index: 21/);
  assert.match(
    stylesSource,
    /\.refreshable-region\.is-refreshing\.page-refresh-region/,
  );
});

test("system configuration separates categories with horizontal navigation", () => {
  assert.match(
    systemConfigSource,
    /className="api-category-bar system-config-category-bar"/,
  );
  assert.match(systemConfigSource, /activeCategory === "ssh"/);
  assert.match(systemConfigSource, /activeCategory === "deployment"/);
  assert.match(systemConfigSource, /activeCategory === "log"/);
  assert.match(systemConfigSource, /SSH 跳板配置/);
  assert.match(systemConfigSource, /部署配置默认值/);
  assert.match(systemConfigSource, /日志后端配置/);
  assert.match(systemConfigSource, /system-config-overview/);
  assert.match(systemConfigSource, /system-config-panel/);
});

test("credential lists keep existing rows visible during refresh", () => {
  assert.match(sshKeysSource, /\{filteredKeys\.length === 0 \? \(/);
  assert.match(imageRegistriesSource, /\{filteredItems\.length === 0 \? \(/);

  for (const source of [sshKeysSource, imageRegistriesSource]) {
    assert.match(source, /refreshable-region/);
    assert.match(source, /<RefreshOverlay/);
  }
});

test("storage refreshes mask the storage class and directory data regions", () => {
  assert.match(storageSource, /storage-class-table-panel refreshable-region/);
  assert.match(storageSource, /storage-files-table-panel refreshable-region/);
  assert.match(storageSource, /label=\{zh \? "正在刷新存储类列表"/);
  assert.match(storageSource, /label=\{zh \? "正在刷新目录内容"/);

  const fileTableSource = storageSource.slice(
    storageSource.indexOf("storage-files-table-panel refreshable-region"),
  );
  assert.doesNotMatch(fileTableSource, /\{loading && \(/);
  assert.match(fileTableSource, /\{filteredPrefixes\.map/);
  assert.match(fileTableSource, /\{pagedObjects\.map/);
});
