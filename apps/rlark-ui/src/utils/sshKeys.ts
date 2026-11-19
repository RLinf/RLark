export type SSHUserKey = {
  index: number;
  user: string;
  public_key: string;
  added_at: string;
};

export type ResolvedSSHKey = {
  publicKey: string;
  owners: Array<{ user: string; index: number }>;
};

export type SelectableSSHKey = ResolvedSSHKey;

export type SSHKeyDuplicate = "name" | "publicKey" | null;

export function findSSHKeyDuplicate(
  name: string,
  publicKey: string,
  knownKeys: SSHUserKey[],
): SSHKeyDuplicate {
  const normalizedName = name.trim();
  const normalizedKey = publicKey.trim();

  if (knownKeys.some((key) => key.user.trim() === normalizedName))
    return "name";
  if (knownKeys.some((key) => key.public_key.trim() === normalizedKey))
    return "publicKey";
  return null;
}

export function groupSSHUserKeys(knownKeys: SSHUserKey[]): SelectableSSHKey[] {
  const keysByValue = new Map<string, SelectableSSHKey>();

  for (const key of knownKeys) {
    const publicKey = key.public_key.trim();
    if (!publicKey) continue;
    const existing = keysByValue.get(publicKey);
    if (existing) {
      existing.owners.push({ user: key.user, index: key.index });
    } else {
      keysByValue.set(publicKey, {
        publicKey,
        owners: [{ user: key.user, index: key.index }],
      });
    }
  }

  return [...keysByValue.values()];
}

export function splitSSHPublicKeys(value?: string): string[] {
  if (!value) return [];

  return value
    .split(/\r?\n/)
    .map((key) => key.trim())
    .filter(Boolean);
}

export function resolveSSHKeyOwners(
  value: string | undefined,
  knownKeys: SSHUserKey[],
): ResolvedSSHKey[] {
  const ownersByKey = new Map(
    groupSSHUserKeys(knownKeys).map(({ publicKey, owners }) => [
      publicKey,
      owners,
    ]),
  );

  return splitSSHPublicKeys(value).map((publicKey) => ({
    publicKey,
    owners: ownersByKey.get(publicKey) ?? [],
  }));
}

/**
 * Splits selected public keys into the ones still registered in SSH key
 * management and the ones whose owner record was removed. A removed record
 * cannot be rendered or deselected in the selection list, so a cloned or
 * edited Job must not carry it forward silently.
 */
export function partitionSelectableSSHPublicKeys(
  selected: string[],
  knownKeys: SSHUserKey[],
): { known: string[]; orphaned: string[] } {
  const registered = new Set(
    groupSSHUserKeys(knownKeys).map(({ publicKey }) => publicKey),
  );
  const known: string[] = [];
  const orphaned: string[] = [];

  for (const publicKey of selected) {
    (registered.has(publicKey.trim()) ? known : orphaned).push(publicKey);
  }

  return { known, orphaned };
}
