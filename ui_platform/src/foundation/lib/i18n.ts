import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import enUS from "@/locales/en-US.json";
import zhCN from "@/locales/zh-CN.json";

function preferredLanguage(): string {
  try {
    const value = localStorage.getItem("tos-language");
    if (value === "zh-CN" || value === "en-US") return value;
  } catch {}
  return navigator.language.startsWith("zh") ? "zh-CN" : "en-US";
}
i18n.on("languageChanged", (language) => {
  try {
    localStorage.setItem("tos-language", language);
  } catch {}
});
void i18n.use(initReactI18next).init({
  resources: { "en-US": { translation: enUS }, "zh-CN": { translation: zhCN } },
  lng: preferredLanguage(),
  fallbackLng: "en-US",
  interpolation: { escapeValue: false },
});

export { i18n };
