// German texts of the shared file workspace (tree, tabs, editor states).
import type { enWorkspace } from "./en";

export const deWorkspace: typeof enWorkspace = {
  unsaved: "Ungespeicherte Änderungen",
  closeTab: "{name} schließen",
  closeHint: "Entf schließt den Tab.",
  tree: {
    loading: "Lädt …",
    failed: "Konnte nicht geladen werden",
    empty: "Leer",
    dirty: "Ungespeichert",
  },
  languages: { yaml: "YAML", json: "JSON", env: "Umgebungsvariablen", ini: "Konfiguration", shell: "Shell-Skript", text: "Klartext" },
  readOnly: "Nur lesen",
  lines: "{count, plural, one {# Zeile} other {# Zeilen}}",
  saveHint: "Strg + S speichert",
  aside: {
    label: "Neben dem Editor",
    off: "Nichts",
    details: "Details",
    split: "Zweite Datei",
    offHint: "Der Editor nutzt die ganze Breite",
    detailsHint: "Prüfung, Struktur und Zusammenhänge zur offenen Datei",
    splitHint: "Eine zweite Datei daneben öffnen",
    secondary: "Zweite Datei",
    choose: "Datei für die rechte Seite wählen",
    none: "Öffne eine weitere Datei als Tab, um sie hier daneben zu sehen.",
  },
};
