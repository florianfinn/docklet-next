// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

export const enImageErrors = {
  "image_active-update-protected": "An active update protects this image.",
  "image_busy": "Another image operation is running. Wait and try again.",
  "image_changed": "The image changed. Refresh the list before trying again.",
  "image_in-use": "The image is in use, including by stopped containers, and is kept.",
  "image_invalid-archive": "The image archive could not be validated.",
  "image_invalid-prune": "The cleanup options are invalid.",
  "image_invalid-reference": "The image reference is invalid.",
  "image_load-failed": "The image could not be loaded.",
  "image_not-found": "The image no longer exists.",
  "image_prune-failed": "The image could not be cleaned up.",
  "image_pull-failed": "The image could not be pulled. Check the reference and saved registry configuration.",
  "image_remove-failed": "The image could not be removed.",
  "image_reserved-reference": "This reference is reserved for internal image retention.",
  "image_restore-point-protected": "A restore point protects this image.",
  "image_system-protected": "This image is protected because Docklet uses it.",
  "image_tag-failed": "The image tag could not be changed.",
  "image_usage-unavailable": "Image usage could not be verified. Nothing is assumed unused.",
  "registry_authentication-failed": "The registry rejected the saved credentials.",
  "registry_authentication-required": "The registry requires saved credentials.",
  "registry_credential-unavailable": "The saved registry credential is unavailable.",
  "registry_exists": "A registry with this address already exists.",
  "registry_inline-credentials-not-allowed": "Save credentials in registry management before pulling.",
  "registry_invalid-config": "The registry configuration is invalid.",
  "registry_not-found": "The registry no longer exists.",
  "registry_persistence-failed": "The registry configuration could not be saved.",
  "registry_rate-limited": "The registry rate limit was reached. Try again later.",
  "registry_sync-failed": "The registry configuration could not be synchronized to a host.",
  "registry_unavailable": "The registry could not be reached.",
  "update_check-unavailable": "Update information is unavailable.",
  "update_deploy-failed": "The update could not be deployed.",
  "update_invalid-target": "This target does not support the requested update operation.",
  "update_persistence-failed": "The update state could not be saved.",
  "update_requires-hub-update": "Update the hub before selecting this agent version.",
  "update_system-version-unknown": "The system image version could not be verified.",
  "image_outcome-unknown": "The operation may have completed. Inspect images and Activities before retrying.",
  "registry_outcome-unknown": "The save or deletion may have completed. Refresh the registries before retrying.",
  "registry_invalid-response": "The registry response could not be read."
};
