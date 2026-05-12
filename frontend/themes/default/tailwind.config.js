/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        mono: ['"Fira Code"', 'ui-monospace', 'Menlo', 'monospace'],
      },
      colors: {
        brand: {
          bg:    '#0b1020',
          card:  'rgba(18,27,52,.82)',
          ok:    '#38d996',
          slow:  '#f6c453',
          error: '#ff6b7a',
          text:  '#edf2ff',
          muted: '#96a3bd',
        },
      },
      borderRadius: {
        '4xl': '2rem',
        '5xl': '2.5rem',
      },
      boxShadow: {
        glass: '0 24px 80px rgba(0,0,0,.35)',
        'glass-light': '0 24px 80px rgba(86,104,134,.18)',
      },
      backdropBlur: {
        glass: '18px',
      },
    },
  },
  plugins: [],
}
