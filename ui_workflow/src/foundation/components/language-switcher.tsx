import { useTranslation } from "react-i18next";

export function LanguageSwitcher() {
  const { i18n, t } = useTranslation();
  return (
    <select
      aria-label={t("common.language")}
      className="rounded-md border border-border bg-background px-2 py-1 text-xs text-foreground"
      value={i18n.resolvedLanguage?.startsWith("en") ? "en-US" : "zh-CN"}
      onChange={(event) => void i18n.changeLanguage(event.target.value)}
    >
      <option value="zh-CN">简体中文</option>
      <option value="en-US">English</option>
    </select>
  );
}
