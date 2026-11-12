import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./locales/en/translation.json";
import ru from "./locales/ru/translation.json";
import cs from "./locales/cs/translation.json";
import de from "./locales/de/translation.json";
import { locales } from "./api";
const saved = localStorage.getItem("locale");
export const explicitLocale = !!saved;
const detected = (saved || navigator.language).split("-")[0];
void i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    ru: { translation: ru },
    cs: { translation: cs },
    de: { translation: de },
  },
  lng: locales.includes(detected as never) ? detected : "en",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});
export default i18n;
