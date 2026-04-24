/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        guard: {
          red: '#ef4444',
          yellow: '#f59e0b',
          green: '#10b981',
          blue: '#3b82f6',
        },
      },
    },
  },
  plugins: [],
};