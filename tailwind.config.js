/** @type {import('tailwindcss').Config} */

// Every colour of the project is a CSS variable, so one component renders light
// in the admin and dark on the storefront without a second set of classes
// (tech.md §19.2). The variables carry space-separated RGB channels, which is
// what lets the opacity utilities (bg-surface/60) keep working.
const token = (name) => `rgb(var(--k-${name}) / <alpha-value>)`;

module.exports = {
  // The Go files under web/templates matter too: toneClasses() and
  // buttonClasses() in components/types.go are where the badge, alert and
  // button colours are actually written down.
  content: ["./web/templates/**/*.templ", "./web/templates/**/*.go", "./internal/**/*.go"],
  theme: {
    extend: {
      colors: {
        bg: token("bg"),
        "bg-deep": token("bg-deep"),
        surface: token("surface"),
        "surface-2": token("surface-2"),
        plate: token("plate"),
        ink: {
          DEFAULT: token("ink"),
          muted: token("ink-muted"),
          faint: token("ink-faint"),
        },
        hair: {
          DEFAULT: token("hair"),
          strong: token("hair-strong"),
        },
        accent: {
          DEFAULT: token("accent"),
          fg: token("accent-fg"),
        },
        gold: token("gold"),
        indigo: token("indigo"),
        success: { bg: token("success-bg"), ink: token("success-ink"), hair: token("success-hair") },
        warning: { bg: token("warning-bg"), ink: token("warning-ink"), hair: token("warning-hair") },
        danger: {
          bg: token("danger-bg"),
          ink: token("danger-ink"),
          hair: token("danger-hair"),
          edge: token("danger-edge"),
          solid: token("danger-solid"),
          "solid-fg": token("danger-solid-fg"),
        },
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "-apple-system", "Segoe UI", "sans-serif"],
        // No web font: font-src is 'self' (tech.md §9.7, §19.7), so the display
        // face is whatever serif the reader already has.
        display: ['"Iowan Old Style"', '"Palatino Linotype"', "Palatino", "Georgia", "ui-serif", "serif"],
      },
      letterSpacing: {
        ornament: "0.32em",
      },
      transitionTimingFunction: {
        soft: "cubic-bezier(.2,.7,.3,1)",
      },
      keyframes: {
        // The marquee scrolls exactly half its width, so the duplicated slogan
        // lands where the original started and the loop has no seam.
        marquee: {
          from: { transform: "translateX(0)" },
          to: { transform: "translateX(-50%)" },
        },
        rise: {
          from: { opacity: "0", transform: "translateY(14px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
        glow: {
          "0%, 100%": { opacity: "0.35" },
          "50%": { opacity: "0.6" },
        },
      },
      animation: {
        marquee: "marquee 42s linear infinite",
        rise: "rise 0.7s cubic-bezier(.2,.7,.3,1) both",
        glow: "glow 9s ease-in-out infinite",
      },
    },
  },
  plugins: [],
};
