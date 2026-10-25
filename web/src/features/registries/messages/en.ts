// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

export const enRegistries = {
  title: "Registries", description: "Saved registry configuration is shared with compatible hosts. Credential identities and secrets are write-only.",
  create: "Add registry", edit: "Edit", remove: "Delete", test: "Test saved credentials", save: "Save", cancel: "Cancel", close: "Close",
  empty: "No registries saved", url: "Registry address", descriptionLabel: "Description", type: "Registry type", generic: "Generic", ecr: "Amazon ECR",
  disabled: "Disabled",
  enabled: "Enabled", insecure: "Allow insecure registry", stored: "Credential stored", noCredential: "No credential stored",
  username: "New username", valueLabel: "New password or token", keyIdentityLabel: "New access key ID", keyValueLabel: "New secret access key", region: "AWS region", repositories: "Repository names (one per line)",
  credentialsHint: "Leave credential fields empty to preserve the stored credential. Existing values are never shown or prefilled.",
  clear: "Clear the complete saved credential", invalid: "Enter a valid registry address without credentials.",
  editTitle: "Edit registry", createTitle: "Add registry", deleteTitle: "Delete registry?", deleteBody: "Delete {url} and its saved credential? This also changes the credential configuration synchronized to hosts.",
  testTitle: "Test registry", testHint: "Tests the saved credential from the hub. Success does not prove access to every repository or from every host.",
  testSuccess: "Registry reachable; saved credential accepted.", checkedAt: "Checked: {time}", unsupported: "This registry type cannot be edited here.",
};
