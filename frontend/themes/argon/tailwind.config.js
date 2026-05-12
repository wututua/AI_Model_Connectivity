/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans:  ['"Open Sans"', 'system-ui', '-apple-system', 'sans-serif'],
        mono:  ['"Fira Code"', 'ui-monospace', 'Menlo', 'monospace'],
      },
      colors: {
        // Argon palette
        argon: {
          primary:   '#5e72e4',
          'primary-dark': '#324cdd',
          info:      '#11cdef',
          success:   '#2dce89',
          warning:   '#fb6340',
          danger:    '#f5365c',
          default:   '#172b4d',
          heading:   '#32325d',
          text:      '#525f7f',
          muted:     '#8898aa',
          border:    '#e9ecef',
          bg:        '#f8f9fe',
          card:      '#ffffff',
        },
      },
      borderRadius: {
        argon:    '0.375rem',
        'argon-lg': '0.75rem',
      },
      boxShadow: {
        argon:    '0 0 2rem 0 rgba(136,152,170,.15)',
        'argon-lg': '0 1rem 3rem rgba(31, 45, 61, .125)',
        'argon-sm': '0 .125rem .25rem rgba(0,0,0,.075)',
      },
      backgroundImage: {
        'argon-gradient':       'linear-gradient(87deg, #5e72e4 0, #825ee4 100%)',
        'argon-gradient-info':  'linear-gradient(87deg, #11cdef 0, #1171ef 100%)',
        'argon-gradient-success': 'linear-gradient(87deg, #2dce89 0, #2dcecc 100%)',
        'argon-hero':           'linear-gradient(135deg, #172b4d 0%, #1a174d 100%)',
      },
    },
  },
  plugins: [],
}
