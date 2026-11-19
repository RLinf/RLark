export function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

export function registryCommands(registry: string, username: string) {
  const example = `${registry}/example/image:version`;
  return [
    `docker login ${shellQuote(registry)} -u ${shellQuote(username)}`,
    `docker tag SOURCE_IMAGE:TAG ${shellQuote(example)}`,
    `docker push ${shellQuote(example)}`,
    `docker pull ${shellQuote(example)}`,
  ];
}

export function pullCommand(reference: string): string {
  return `docker pull ${shellQuote(reference)}`;
}
