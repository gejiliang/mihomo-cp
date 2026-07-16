import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { en, type TranslationKey } from './en';
import { zh } from './zh';

export type Locale = 'en' | 'zh';

const translations: Record<Locale, Record<string, string>> = { en, zh };

interface I18nState {
  locale: Locale;
  setLocale: (locale: Locale) => void;
}

export const useI18nStore = create<I18nState>()(
  persist(
    (set) => ({
      locale: 'en',
      setLocale: (locale) => set({ locale }),
    }),
    { name: 'i18n-storage' },
  ),
);

/**
 * Translation hook. Returns a `t` function that translates keys.
 * Supports simple interpolation: t('key', { name: 'foo' }) replaces {name} with foo.
 */
export function useT() {
  const locale = useI18nStore((s) => s.locale);
  const dict = translations[locale];

  function t(key: TranslationKey, params?: Record<string, string | number>): string {
    let text = dict[key] ?? en[key] ?? key;
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        text = text.replaceAll(`{${k}}`, String(v));
      }
    }
    return text;
  }

  return t;
}

export type { TranslationKey };
