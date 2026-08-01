/** @type {import('tailwindcss').Config} */
module.exports = {
  // The Go files under web/templates matter too: toneClasses() and
  // buttonClasses() in components/types.go are where the badge, alert and
  // button colours are actually written down.
  content: ["./web/templates/**/*.templ", "./web/templates/**/*.go", "./internal/**/*.go"],
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
