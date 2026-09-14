// @ts-check
import { fileURLToPath } from "node:url";

import { defineConfig } from "astro/config";
import react from "@astrojs/react";

// The site is served from GitHub Pages under the repository name, so every
// link has to carry the base. SITE_URL and SITE_BASE are here so that a
// custom domain is a change to the workflow and not to this file.
export default defineConfig({
  site: process.env.SITE_URL ?? "https://tamnd.github.io",
  base: process.env.SITE_BASE ?? "/papers-reader",
  integrations: [react()],
  // A page per directory, so /p/cook-1971-np/en is a directory with an
  // index.html in it and the emitted p/cook-1971-np/en.json sits beside it
  // rather than fighting it for a name.
  build: { format: "directory" },
  // Astro's own asset handling is not wanted here. The figures are copied
  // into the build by papers emit with their sizes already in the page, and
  // running them through an optimiser would change bytes the corpus hashed.
  image: { service: { entrypoint: "astro/assets/services/noop" } },
  // The stylesheet for the mathematics is the one vendored beside the
  // renderer that wrote the markup, at ../katex/assets, pinned to the same
  // release by ../katex/SHA256SUMS. Taking it from npm instead would mean
  // the CSS and the HTML could be two different versions of KaTeX, which
  // shows up as formulas that are subtly wrong rather than as an error.
  vite: {
    resolve: {
      alias: {
        "@katex": fileURLToPath(new URL("../katex/assets", import.meta.url)),
      },
    },
    server: { fs: { allow: [".."] } },
  },
});
