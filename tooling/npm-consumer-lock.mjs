export function createConsumerLock(rootLock, consumer, artifacts) {
  // npm ci caches tarballs without registry metadata. Seed the consumer from
  // the frozen root selections; npm prunes unused entries during installation.
  // Workspace links cannot escape into the independent consumer.
  const packages = Object.fromEntries(
    Object.entries(rootLock.packages).filter(
      ([name, entry]) => name.startsWith("node_modules/") && !entry.link,
    ),
  );
  for (const { manifest, resolved, integrity } of artifacts) {
    packages[`node_modules/${manifest.name}`] = {
      version: manifest.version,
      resolved,
      integrity,
      dependencies: manifest.dependencies,
      peerDependencies: manifest.peerDependencies,
    };
  }
  return {
    name: consumer.name,
    version: consumer.version,
    lockfileVersion: rootLock.lockfileVersion,
    requires: true,
    packages: { "": consumer, ...packages },
  };
}
