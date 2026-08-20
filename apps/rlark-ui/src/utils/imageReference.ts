export function imageReferenceHasWhitespace(value: string): boolean {
  return /\s/.test(value);
}
