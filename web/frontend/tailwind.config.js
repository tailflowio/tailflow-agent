import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

// Resolve relative to this config file, not to CWD
const __configDir = dirname(fileURLToPath(new URL(import.meta.url)))

/** @type {import('tailwindcss').Config} */
export default {
  content: [
    resolve(__configDir, "index.html"),
    resolve(__configDir, "src/**/*.{vue,js,ts,jsx,tsx}"),
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"DM Sans"', 'system-ui', 'sans-serif'],
        mono: ['"DM Mono"', 'monospace'],
      },
      colors: {
        g: {
          0: 'var(--g-0)',
          1: 'var(--g-1)',
          2: 'var(--g-2)',
          3: 'var(--g-3)',
          4: 'var(--g-4)',
          5: 'var(--g-5)',
          6: 'var(--g-6)',
          7: 'var(--g-7)',
          8: 'var(--g-8)',
          9: 'var(--g-9)',
          10: 'var(--g-10)',
          11: 'var(--g-11)',
          12: 'var(--g-12)',
          13: 'var(--g-13)',
          14: 'var(--g-14)',
          15: 'var(--g-15)',
        },
        emerald: { 400: 'rgb(var(--c-green) / <alpha-value>)' },
        red: { 400: 'rgb(var(--c-red) / <alpha-value>)' },
        amber: { 400: 'rgb(var(--c-amber) / <alpha-value>)' },
      },
    },
  },
  plugins: [],
}
