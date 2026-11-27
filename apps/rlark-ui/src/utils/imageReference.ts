export function imageReferenceHasWhitespace(value: string): boolean {
  return /\s/.test(value);
}

// Return an explicit image tag, excluding the registry port and digest.
export function imageReferenceTag(value: string): string | undefined {
  const nameAndTag = value.split("@", 1)[0];
  const separator = nameAndTag.lastIndexOf(":");
  if (separator <= nameAndTag.lastIndexOf("/") || separator === nameAndTag.length - 1) return undefined;
  return nameAndTag.slice(separator + 1);
}
