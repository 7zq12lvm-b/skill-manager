/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: [
          "ui-sans-serif",
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "Helvetica",
          "Arial",
          "sans-serif",
        ],
      },
      colors: {
        border: "hsl(218 18% 86%)",
        input: "hsl(218 18% 84%)",
        ring: "hsl(190 85% 36%)",
        background: "hsl(78 24% 97%)",
        foreground: "hsl(218 28% 12%)",
        muted: {
          DEFAULT: "hsl(205 18% 94%)",
          foreground: "hsl(218 11% 43%)",
        },
        primary: {
          DEFAULT: "hsl(190 85% 32%)",
          foreground: "hsl(180 40% 98%)",
        },
      },
    },
  },
  plugins: [],
};
