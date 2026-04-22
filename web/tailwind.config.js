/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./src/**/*.{html,ts}",
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
      },
    },
  },
  plugins: [],
}
