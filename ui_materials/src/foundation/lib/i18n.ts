import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import enUS from "@/locales/en-US.json";
import zhCN from "@/locales/zh-CN.json";
import type { HostEvents } from "./plugin-host";

const initialLanguage =
  typeof navigator === "undefined" ? "en-US" : navigator.language;

void i18n.use(initReactI18next).init({
  resources: {
    "en-US": { translation: enUS },
    "zh-CN": { translation: zhCN },
  },
  lng: initialLanguage.startsWith("zh") ? "zh-CN" : "en-US",
  fallbackLng: "en-US",
  interpolation: { escapeValue: false },
});

export function syncHostLanguage(events: HostEvents): () => void {
  return events.on("lang.change", (language) => {
    void i18n.changeLanguage(language.startsWith("zh") ? "zh-CN" : "en-US");
  });
}

export default i18n;
