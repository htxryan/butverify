import starlightPlugin from '@astrojs/starlight-tailwind';

const tokens = {
  bg: '#0b0d12',
  surface: '#11141b',
  surface2: '#181c25',
  text: '#e6e8ef',
  'text-dim': '#94a0b3',
  border: '#232735',
  accent: '#8aa9ff',
  'accent-strong': '#5b85ff',
  warn: '#f5a623',
  danger: '#ff6b6b',
  good: '#5fd49e',
};

/** @type {import("tailwindcss").Config} */
export default {
  content: ['./src/**/*.{astro,html,js,jsx,md,mdx,svelte,ts,tsx}'],
  darkMode: ['class', '[data-theme="dark"]'],
  theme: {
    extend: {
      colors: {
        bv: tokens,
        accent: {
          50: '#eef3ff',
          100: '#dde7ff',
          200: '#b8c9ff',
          300: '#8aa9ff',
          400: '#5b85ff',
          500: '#3a66f0',
          600: '#274cc7',
          700: '#1d3a99',
          800: '#16306d',
          900: '#0c1d44',
        },
        gray: {
          50: '#f7f8fa',
          100: '#e6e8ef',
          200: '#c4cad6',
          300: '#94a0b3',
          400: '#6c7689',
          500: '#4a5364',
          600: '#323948',
          700: '#232735',
          800: '#181c25',
          900: '#11141b',
          950: '#0b0d12',
        },
      },
      fontFamily: {
        sans: [
          'Inter',
          'ui-sans-serif',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'system-ui',
          'sans-serif',
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'SF Mono', 'Menlo', 'Geist Mono', 'monospace'],
      },
      maxWidth: {
        prose: '68ch',
        copy: '44rem',
      },
      borderRadius: {
        sm: '4px',
        DEFAULT: '6px',
        md: '8px',
        lg: '12px',
        xl: '20px',
      },
    },
  },
  plugins: [starlightPlugin()],
};
