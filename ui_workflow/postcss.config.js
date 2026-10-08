import autoprefixer from "autoprefixer";
import tailwindcss from "tailwindcss";
import { scopePluginCss } from "./scripts/scope-plugin-css.mjs";

export default {
  plugins: [
    tailwindcss(),
    autoprefixer(),
    ...(process.env.VITE_TARGET === "plugin" ? [scopePluginCss()] : []),
  ],
};
