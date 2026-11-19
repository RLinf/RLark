import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { readStyles } from "./read-styles.mjs";

const domainsSource = await readFile(
  new URL("../src/pages/Domains.tsx", import.meta.url),
  "utf8",
);
const createJobSource = await readFile(
  new URL("../src/pages/CreateJob.tsx", import.meta.url),
  "utf8",
);
const jobsSource = await readFile(
  new URL("../src/pages/Jobs.tsx", import.meta.url),
  "utf8",
);
const styles = await readStyles();

test("long domain names are truncated with their full value available on hover", () => {
  assert.match(domainsSource, /domains-table-panel/);
  assert.match(domainsSource, /<strong title=\{d\.metadata\.name\}>/);
  assert.match(domainsSource, /className="domain-detail-title"/);
  assert.match(
    createJobSource,
    /className="field-hint network-domain-summary"/,
  );
  assert.match(createJobSource, /title=\{automaticDomain \|\| undefined\}/);
  assert.match(
    jobsSource,
    /className: job\.domain \? "public-config-truncated-value"/,
  );
  assert.match(
    styles,
    /\.domains-table-panel table[\s\S]*?table-layout: fixed/,
  );
  assert.match(styles, /\.domain-detail-title[\s\S]*?text-overflow: ellipsis/);
});

test("job details render every injected SSH key with its owner", () => {
  assert.match(jobsSource, /resolveSSHKeyOwners\(job\.sshPublicKey, sshKeys\)/);
  assert.match(jobsSource, /resolvedSSHKeys\.map/);
  assert.match(
    jobsSource,
    /owners\.map\(\(\{ user \}\) => user\)\.join\(", "\)/,
  );
  assert.match(styles, /\.job-ssh-key-list/);
});
