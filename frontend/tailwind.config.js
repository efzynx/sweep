/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{svelte,js,ts}'],
  theme: {
    extend: {
      colors: {
        'nordic-bg': '#07090e',
        'nordic-window': '#0c1017',
        'nordic-sidebar': '#090d14',
        'nordic-card': '#121824',
        'nordic-card-hover': '#172030',
        'nordic-border': '#1c2536',
        'nordic-cyan': {
          DEFAULT: '#06b6d4',
          dim: 'rgba(6, 182, 212, 0.12)',
          glow: 'rgba(6, 182, 212, 0.25)',
        },
        'nordic-purple': {
          DEFAULT: '#a855f7',
          dim: 'rgba(168, 85, 247, 0.12)',
        },
        'nordic-green': {
          DEFAULT: '#10b981',
          dim: 'rgba(16, 185, 129, 0.12)',
        },
        'nordic-amber': {
          DEFAULT: '#f59e0b',
          dim: 'rgba(245, 158, 11, 0.12)',
        },
        'nordic-red': {
          DEFAULT: '#ef4444',
          dim: 'rgba(239, 68, 68, 0.12)',
        },
        'nordic-text': {
          primary: '#f8fafc',
          secondary: '#94a3b8',
          muted: '#64748b',
        },
      },
      fontFamily: {
        sans: ['Inter', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['JetBrains Mono', 'SF Mono', 'Fira Code', 'monospace'],
      },
      boxShadow: {
        glow: '0 0 14px rgba(6, 182, 212, 0.25)',
        'glow-lg': '0 0 20px rgba(6, 182, 212, 0.45)',
      },
    },
  },
  plugins: [],
};
