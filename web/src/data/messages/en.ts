export const enBackend = {
  errors: {
    "image": {
      "active-update-protected": "An active or unresolved run retains this image.",
      "busy": "Another image operation is running. Try again.",
      "changed": "The image changed during the operation. Refresh and try again.",
      "in-use": "This image is used by a running or stopped container.",
      "invalid-archive": "The archive cannot be safely validated.",
      "invalid-prune": "Choose a valid prune mode and age or timestamp.",
      "invalid-reference": "Enter a valid image reference or search term.",
      "load-failed": "The image archive could not be loaded. Check the Docker connection and try again.",
      "not-found": "The image was not found. Refresh the image list.",
      "prune-failed": "Some images could not be pruned. Check the result before trying again.",
      "pull-failed": "The image could not be pulled. Check its reference, registry access and Docker connection.",
      "remove-failed": "The image could not be removed. Refresh its usage and try again.",
      "reserved-reference": "This reference is reserved for internal retention.",
      "restore-point-protected": "A restore point retains this image.",
      "system-protected": "This image is required by the system.",
      "tag-failed": "The tag could not be created. Check the source image and target reference.",
      "usage-unavailable": "Image details or usage could not be verified. Check the Docker and registry connections before trying again."
    },
    "registry": {
      "authentication-failed": "The registry rejected the saved credentials. Replace them and test again.",
      "authentication-required": "This registry requires credentials. Save credentials before testing again.",
      "credential-unavailable": "Registry credentials require a valid encryption key.",
      "exists": "A registry with this identity already exists.",
      "inline-credentials-not-allowed": "Use saved registry credentials or anonymous access.",
      "invalid-config": "Check the registry URL, type and required settings.",
      "not-found": "Registry not found.",
      "persistence-failed": "Registry settings could not be saved. Try again.",
      "rate-limited": "The registry request limit was reached. Wait before trying again.",
      "sync-failed": "Registry settings could not be synced to the agent. Check its connection and retry.",
      "unavailable": "The registry could not be reached. Check its address and network connection."
    },
    "restore": {
      "unavailable": "This restore point is incomplete or its retained images are unavailable.",
      "shared-reference": "Another workload prevents moving a shared image reference. Resolve the conflict before restoring.",
      "image-mismatch": "The restored image bindings could not be verified. Inspect the run before retrying.",
      "in-use": "An active or unresolved run still holds this restore point.",
      "not-found": "The restore point was not found. Refresh the point list.",
      "deploy-failed": "The restore deployment failed. Inspect its partial result before another recovery attempt."
    },
    "update": {
      "config-drift": "Deploy or discard the saved stack changes before updating.",
      "baseline-unavailable": "Deploy the stack once to record a verified configuration baseline.",
      "snapshot-unavailable": "A complete restore point could not be captured. Check the stack inputs and image availability.",
      "target-changed": "The admitted stack or image binding changed. Refresh before retrying.",
      "image-binding-unsupported": "This image binding or daemon image store cannot support the selected update safely.",
      "path-retired": "Use the stack image update action instead of this retired update entry point.",
      "target-not-found": "The stack update target no longer exists. Refresh the stack list.",

      "check-unavailable": "The update check could not be completed.",
      "deploy-failed": "The update could not be deployed. Check its result and the host connection.",
      "invalid-target": "Select an explicit valid update target.",
      "persistence-failed": "The update result could not be saved. Refresh its status before retrying.",
      "requires-hub-update": "Update the hub before upgrading this agent.",
      "system-version-unknown": "A compatible release target could not be verified."
    }
  },
  title: "No backend connected",
  hint: "This build is not connected to a hub yet. Local appearance settings are available.",
};
