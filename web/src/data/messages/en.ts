export const enBackend = {
  errors: {
    "image": {
      "active-update-protected": "An active or unresolved run retains this image.",
      "busy": "Another image operation is running. Try again.",
      "changed": "The image or registry operation could not be completed.",
      "in-use": "This image is used by a running or stopped container.",
      "invalid-archive": "The archive cannot be safely validated.",
      "invalid-prune": "The image or registry operation could not be completed.",
      "invalid-reference": "The image or registry operation could not be completed.",
      "load-failed": "The image or registry operation could not be completed.",
      "not-found": "The image or registry operation could not be completed.",
      "prune-failed": "The image or registry operation could not be completed.",
      "pull-failed": "The image or registry operation could not be completed.",
      "remove-failed": "The image or registry operation could not be completed.",
      "reserved-reference": "This reference is reserved for internal retention.",
      "restore-point-protected": "A restore point retains this image.",
      "system-protected": "This image is required by the system.",
      "tag-failed": "The image or registry operation could not be completed.",
      "usage-unavailable": "The image or registry operation could not be completed."
    },
    "registry": {
      "authentication-failed": "The image or registry operation could not be completed.",
      "authentication-required": "The image or registry operation could not be completed.",
      "credential-unavailable": "Registry credentials require a valid encryption key.",
      "exists": "A registry with this identity already exists.",
      "inline-credentials-not-allowed": "Use saved registry credentials or anonymous access.",
      "invalid-config": "The image or registry operation could not be completed.",
      "not-found": "Registry not found.",
      "persistence-failed": "The image or registry operation could not be completed.",
      "rate-limited": "The image or registry operation could not be completed.",
      "sync-failed": "The image or registry operation could not be completed.",
      "unavailable": "The image or registry operation could not be completed."
    },
    "update": {
      "check-unavailable": "The update check could not be completed.",
      "deploy-failed": "The image or registry operation could not be completed.",
      "invalid-target": "Select an explicit valid update target.",
      "persistence-failed": "The image or registry operation could not be completed.",
      "requires-hub-update": "Update the hub before upgrading this agent.",
      "system-version-unknown": "A compatible release target could not be verified."
    }
  },
  title: "No backend connected",
  hint: "This build is not connected to a hub yet. Local appearance settings are available.",
};
