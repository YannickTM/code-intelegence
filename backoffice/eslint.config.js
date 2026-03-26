import nextPlugin from "@next/eslint-plugin-next";
import reactPlugin from "eslint-plugin-react";
import hooksPlugin from "eslint-plugin-react-hooks";
import tseslint from "typescript-eslint";

export default tseslint.config(
  // Next.js core-web-vitals rules
  nextPlugin.configs["core-web-vitals"],
  // React recommended (flat config, jsx-runtime mode for React 19)
  // @ts-expect-error -- eslint-plugin-react flat config types allow undefined but the key always exists
  reactPlugin.configs.flat["jsx-runtime"],
  // React Hooks
  hooksPlugin.configs.flat["recommended-latest"],
  // TypeScript
  {
    files: ["**/*.ts", "**/*.tsx"],
    extends: [
      ...tseslint.configs.recommendedTypeChecked,
      ...tseslint.configs.stylisticTypeChecked,
    ],
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      "@typescript-eslint/array-type": "off",
      "@typescript-eslint/consistent-type-definitions": "off",
      "@typescript-eslint/consistent-type-imports": [
        "warn",
        { prefer: "type-imports", fixStyle: "inline-type-imports" },
      ],
      "@typescript-eslint/no-unused-vars": [
        "warn",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      "@typescript-eslint/require-await": "off",
      "@typescript-eslint/no-misused-promises": [
        "error",
        { checksVoidReturn: { attributes: false } },
      ],
    },
  },
  // Global settings
  {
    linterOptions: {
      reportUnusedDisableDirectives: true,
    },
    settings: {
      react: {
        version: "detect",
      },
    },
  },
  // Disable type-checked rules for test files & vitest config (vitest provides its own type context)
  {
    files: ["**/*.test.ts", "**/*.test.tsx", "vitest.config.ts"],
    extends: [tseslint.configs.disableTypeChecked],
  },
  // Ignores (replaces eslint-config-next's ignores)
  {
    ignores: [".next/**", "out/**", "build/**", "next-env.d.ts"],
  },
);
