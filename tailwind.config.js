/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./static/**/*.{html,js}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        "tertiary-container": "#b64100",
        "secondary-container": "#6efa98",
        "on-surface": "#151c27",
        "on-secondary-fixed": "#00210b",
        "on-primary": "#ffffff",
        "secondary-fixed-dim": "#52e082",
        "error-container": "#ffdad6",
        "surface-container-lowest": "#ffffff",
        "on-primary-fixed": "#001945",
        "inverse-on-surface": "#ebf1ff",
        "outline": "#727786",
        "outline-variant": "#c2c6d7",
        "inverse-surface": "#2a313d",
        "primary": "#004aae",
        "primary-container": "#0060df",
        "error": "#ba1a1a",
        "on-tertiary-container": "#ffe1d7",
        "on-secondary-fixed-variant": "#005226",
        "on-tertiary": "#ffffff",
        "surface-container-low": "#f0f3ff",
        "tertiary": "#8e3100",
        "surface-bright": "#f9f9ff",
        "on-secondary": "#ffffff",
        "on-secondary-container": "#007237",
        "surface": "#f9f9ff",
        "on-surface-variant": "#424654",
        "on-primary-fixed-variant": "#00419d",
        "on-primary-container": "#e0e7ff",
        "surface-container": "#e7eefe",
        "tertiary-fixed": "#ffdbce",
        "inverse-primary": "#b0c6ff",
        "on-error-container": "#93000a",
        "secondary-fixed": "#71fd9b",
        "background": "#f9f9ff",
        "on-tertiary-fixed-variant": "#7f2b00",
        "on-error": "#ffffff",
        "surface-tint": "#0057cc",
        "surface-container-highest": "#dce2f3",
        "surface-variant": "#dce2f3",
        "primary-fixed": "#d9e2ff",
        "on-background": "#151c27",
        "surface-dim": "#d3daea",
        "primary-fixed-dim": "#b0c6ff",
        "on-tertiary-fixed": "#370e00",
        "secondary": "#006d34",
        "tertiary-fixed-dim": "#ffb599",
        "surface-container-high": "#e2e8f8"
      },
      borderRadius: {
        DEFAULT: "0.25rem",
        lg: "0.5rem",
        xl: "0.75rem",
        full: "9999px"
      },
      spacing: {
        "container-max-width": "640px",
        "gutter": "1.5rem",
        "section-gap": "4rem",
        "stack-gap": "1rem",
        "margin-mobile": "1rem"
      },
      fontFamily: {
        "headline-lg": ["Inter"],
        "mono-sm": ["JetBrains Mono"],
        "body-md": ["Inter"],
        "headline-lg-mobile": ["Inter"],
        "label-md": ["Inter"],
        "headline-xl": ["Inter"],
        "body-sm": ["Inter"]
      },
      fontSize: {
        "headline-lg": ["32px", {
          lineHeight: "40px",
          letterSpacing: "-0.01em",
          fontWeight: "600"
        }],
        "mono-sm": ["13px", {
          lineHeight: "18px",
          fontWeight: "400"
        }],
        "body-md": ["16px", {
          lineHeight: "24px",
          fontWeight: "400"
        }],
        "headline-lg-mobile": ["28px", {
          lineHeight: "36px",
          fontWeight: "600"
        }],
        "label-md": ["14px", {
          lineHeight: "16px",
          fontWeight: "600"
        }],
        "headline-xl": ["48px", {
          lineHeight: "56px",
          letterSpacing: "-0.02em",
          fontWeight: "700"
        }],
        "body-sm": ["14px", {
          lineHeight: "20px",
          fontWeight: "400"
        }]
      }
    },
  },
  plugins: [
    require('@tailwindcss/forms'),
  ],
}
