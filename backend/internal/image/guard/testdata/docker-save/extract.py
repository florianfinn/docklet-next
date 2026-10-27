# Source: docklet-next/issues/229
# Revision: 474f31a45f4909c153dee30a37306fcd838678da
# Retrieved: 2026-10-25
# License: Apache-2.0
# Deviations: Extract only import metadata; never retain layer or attestation payloads.

import json
import pathlib
import sys
import tarfile


def extract(source, destination):
    destination = pathlib.Path(destination)
    with tarfile.open(source) as archive:
        entries = {member.name: member for member in archive if member.isfile()}
        metadata = {}

        def read(name):
            if name in metadata or name not in entries:
                return None
            if entries[name].size > 8 << 20:
                raise ValueError("oversized metadata")
            data = archive.extractfile(entries[name]).read()
            value = json.loads(data)
            metadata[name] = data
            return value

        def walk(descriptor):
            algorithm, digest = descriptor["digest"].split(":", 1)
            if algorithm != "sha256" or len(digest) != 64 or any(c not in "0123456789abcdef" for c in digest):
                raise ValueError("unsupported metadata digest")
            value = read("blobs/sha256/" + digest)
            if value is None:
                return  # Engine retains descriptors for unpulled platforms.
            for child in value.get("manifests", []):
                walk(child)
            config = value.get("config")
            if config:
                read("blobs/" + config["digest"].replace(":", "/", 1))
            # Deliberately do not traverse layers, including JSON attestations.

        read("oci-layout")
        root = read("index.json")
        for descriptor in root["manifests"]:
            walk(descriptor)
        for manifest in read("manifest.json"):
            read(manifest["Config"])
        members = [{"path": name, "json": name in metadata} for name in entries]
        destination.mkdir(parents=True, exist_ok=True)
        for name, data in metadata.items():
            output = destination / name
            if not output.resolve().is_relative_to(destination.resolve()):
                raise ValueError("unsafe metadata path")
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_bytes(data)
        (destination / "members.json").write_text(json.dumps(members, indent=2) + "\n")
        return len(metadata)


if __name__ == "__main__":
    print(extract(sys.argv[1], sys.argv[2]), "JSON metadata files")
