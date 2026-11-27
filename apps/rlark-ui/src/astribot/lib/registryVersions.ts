import type { RegistryTagDetail } from "../api";

/** Tags that resolve to the same manifest digest, shown as one image version. */
export interface RegistryVersion {
  key: string;
  digest?: string;
  /** The first tag is the main tag; the rest are aliases. */
  tags: string[];
  created?: string;
  size?: number;
  platforms: string[];
  unavailable: boolean;
}

const timestampedTag = /^\d{8}-\d{4}-/;

function mainTagFirst(tags: string[]): string[] {
  const sorted = [...tags];
  const index = sorted.findIndex((tag) => timestampedTag.test(tag));
  const main =
    index >= 0
      ? index
      : sorted.reduce((best, tag, i) => (tag.length > sorted[best].length ? i : best), 0);
  const [picked] = sorted.splice(main, 1);
  return [picked, ...sorted];
}

function createdTime(version: RegistryVersion): number {
  const value = version.created ? Date.parse(version.created) : NaN;
  return Number.isNaN(value) ? -Infinity : value;
}

/** Groups tag details by digest and sorts versions newest first; unknown times go last. */
export function groupRegistryVersions(details: RegistryTagDetail[]): RegistryVersion[] {
  const byDigest = new Map<string, RegistryVersion>();
  const versions: RegistryVersion[] = [];
  for (const detail of details) {
    const digest = detail.digest!;
    if (detail.error) {
      versions.push({ key: `tag:${detail.tag}`, digest: detail.digest, tags: [detail.tag], platforms: [], unavailable: true });
      continue;
    }
    const existing = byDigest.get(digest);
    if (existing) {
      existing.tags.push(detail.tag);
      continue;
    }
    const version: RegistryVersion = {
      key: digest,
      digest,
      tags: [detail.tag],
      created: detail.created,
      size: detail.size,
      platforms: detail.platforms ?? [],
      unavailable: false,
    };
    byDigest.set(digest, version);
    versions.push(version);
  }
  for (const version of versions) version.tags = mainTagFirst(version.tags);
  const timestamped = (version: RegistryVersion) => (timestampedTag.test(version.tags[0]) ? 1 : 0);
  return versions.sort(
    (a, b) =>
      createdTime(b) - createdTime(a) ||
      timestamped(b) - timestamped(a) ||
      b.tags[0].localeCompare(a.tags[0]),
  );
}

export function splitRepository(name: string): { namespace: string; short: string } {
  const index = name.indexOf("/");
  return index < 0 ? { namespace: "", short: name } : { namespace: name.slice(0, index), short: name.slice(index + 1) };
}

export function formatBytes(bytes: number | undefined): string {
  if (bytes === undefined || !Number.isFinite(bytes) || bytes < 0) return "—";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
  return `${(bytes / 1024 ** 3).toFixed(2)} GB`;
}

export function isRecent(value: string | undefined, now: number): boolean {
  const time = value ? Date.parse(value) : NaN;
  return !Number.isNaN(time) && now - time >= 0 && now - time < 86400000;
}
