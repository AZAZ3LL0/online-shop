/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./web/templates/**/*.templ", "./internal/**/*.go"],
  theme: {
    extend: {
      colors: {
        accent: {
          DEFAULT: "#e11d48",
          fg: "#ffffff",
        },
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "-apple-system", "Segoe UI", "sans-serif"],
      },
    },
  },
  plugins: [],
};
