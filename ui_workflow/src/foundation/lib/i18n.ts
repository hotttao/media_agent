/**
 * i18next singleton. Side-effect import (from main.tsx in standalone, or from
 * the plugin entry) initializes the instance; react-i18next's useTranslation
 * picks it up via initReactI18next.
 *
 * Adapted from agent-platform/web/src/lib/i18n.ts. In plugin mode, also
 * subscribes to the host's `lang.change` event so the plugin's local i18next
 * stays in sync with the host language (per docs/plan/ui_workflow.md §4.3
 * 方案 C - plugin carries its own i18next copy since window.tos doesn't expose
 * useTranslation).
 */
import i18n from "i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import { initReactI18next } from "react-i18next";
import { LANGUAGE_STORAGE_KEY } from "@/foundation/lib/constants";
import { isPluginHost } from "@/foundation/lib/plugin-host";

const localeModules = import.meta.glob<{ default: Record<string, unknown> }>(
  "../../locales/*.json",
  { eager: true },
);

const resources = Object.entries(localeModules).reduce(
  (acc, [path, module]) => {
    const locale = path.match(/.*\/locales\/(.+)\.json$/)?.[1];
    if (locale) acc[locale] = { translation: module.default };
    return acc;
  },
  {} as Record<string, { translation: Record<string, unknown> }>,
);

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    supportedLngs: Object.keys(resources),
    fallbackLng: "en-US",
    load: "currentOnly",
    debug: false,
    detection: {
      order: ["localStorage", "navigator"],
      caches: ["localStorage"],
      lookupLocalStorage: LANGUAGE_STORAGE_KEY,
      convertDetectedLanguage: (lng: string) =>
        lng.toLowerCase().startsWith("zh") ? "zh-CN" : "en-US",
    },
    interpolation: { escapeValue: false },
    react: { useSuspense: false },
  });

// Plugin mode: follow host language changes. Standalone has no window.tos and
// relies on LanguageDetector alone.
if (isPluginHost() && window.tos) {
  window.tos.events.on("lang.change", (lng: string) => {
    void i18n.changeLanguage(lng);
  });
}

export { i18n };
export default i18n;
