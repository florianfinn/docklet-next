import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { linterOptions: { reportUnusedDisableDirectives: "error" } },
  { ignores: ["dist", "node_modules"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: { ecmaVersion: 2023, globals: globals.browser },
    plugins: { "react-hooks": reactHooks, "react-refresh": reactRefresh },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": ["warn", { allowConstantExport: true, extraHOCs: ["switchable"] }],
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_", varsIgnorePattern: "^_" }],
    },
  },
  {
    // The design book script runs as a classic browser script, outside the Vite build.
    files: ["designbook/**/*.js"],
    languageOptions: { ecmaVersion: 2023, sourceType: "script", globals: globals.browser },
  },
  {
    // shadcn primitives, the router module and test helpers are not hot-reloaded components.
    files: ["src/shared/ui/**/*.tsx", "src/app/router.tsx", "src/test/**/*.tsx"],
    rules: { "react-refresh/only-export-components": "off" },
  },
);
