/** @type {import('tailwindcss').Config} */
module.exports = {
  important: true,
  content: [
    "./src/**/*.{html,ts,css,scss}",
    "./src/index.html",
  ],
  theme: {
    extend: {
      colors: {
        'atlassian-blue': '#0052CC',
        'atlassian-dark-blue': '#0747A6',
        'atlassian-light-gray': '#F4F5F7',
        'atlassian-text': '#172B4D',
        'atlassian-subtle-text': '#6B778C',
        'atlassian-border': '#DFE1E6',
        'atlassian-success': '#36B37E',
        'atlassian-danger': '#FF5630',
        'atlassian-warning': '#FF991F',
      },
      animation: {
        'slide-in': 'slide-in 0.3s ease-out forwards',
      },
      keyframes: {
        'slide-in': {
          '0%': { transform: 'translateX(100%)', opacity: '0' },
          '100%': { transform: 'translateX(0)', opacity: '1' },
        },
      },
    },
  },
  plugins: [],
}
