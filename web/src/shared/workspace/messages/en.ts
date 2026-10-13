// American English texts of the shared file workspace (tree, tabs, editor states).
import type { deWorkspace } from "./de";

export const enWorkspace: typeof deWorkspace = {
  unsaved: "Unsaved changes",
  closeTab: "Close {name}",
  closeHint: "Delete closes the tab.",
  tree: {
    loading: "Loading …",
    failed: "Could not be loaded",
    empty: "Empty",
    dirty: "Unsaved",
  },
  languages: { yaml: "YAML", json: "JSON", env: "Environment variables", ini: "Configuration", shell: "Shell script", text: "Plain text" },
  readOnly: "Read only",
  lines: "{count, plural, one {# line} other {# lines}}",
  saveHint: "Ctrl + S saves",
  aside: {
    label: "Next to the editor",
    off: "Nothing",
    details: "Details",
    split: "Second file",
    offHint: "The editor uses the full width",
    detailsHint: "Validation, structure and relations of the open file",
    splitHint: "Open a second file alongside",
    secondary: "Second file",
    choose: "Choose file for the right side",
    none: "Open another file as a tab to see it here alongside.",
  },
};
