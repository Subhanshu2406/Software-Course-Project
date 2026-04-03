/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,jsx}'],
  theme: {
    extend: {
      colors: {
        surface: {
          DEFAULT: '#131315',
          dim: '#101012',
          low: '#1b1b1d',
          container: '#201f21',
          highest: '#353437',
        },
        primary: {
          DEFAULT: '#c0c1ff',
          container: '#8083ff',
        },
        tertiary: {
          DEFAULT: '#ffb783',
          container: '#8b5e3c',
        },
        error: {
          DEFAULT: '#ffb4ab',
          container: '#93000a',
        },
        outline: {
          variant: '#464554',
        },
        'on-surface': {
          DEFAULT: '#e5e1e4',
          variant: '#c7c4d7',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'monospace'],
      },
    },
  },
  plugins: [],
}
