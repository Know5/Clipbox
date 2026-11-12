export const TARGET_LANGUAGES = [
  "简体中文",
  "繁體中文",
  "English",
  "日本語",
  "한국어",
  "Français",
  "Deutsch",
  "Español",
  "Русский",
];

export function textToolError(error: unknown, fallback: string): string {
  return typeof error === "string" && error
    ? error
    : error instanceof Error
      ? error.message
      : fallback;
}
