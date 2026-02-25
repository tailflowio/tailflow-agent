import { createI18n } from 'vue-i18n'
import en from './en'
import fr from './fr'

export const i18n = createI18n({
  legacy: false,
  locale: localStorage.getItem('locale') || 'en',
  fallbackLocale: 'en',
  messages: { en, fr },
})
