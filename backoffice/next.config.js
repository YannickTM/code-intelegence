/**
 * Run `build` or `dev` with `SKIP_ENV_VALIDATION` to skip env validation. This is especially useful
 * for Docker builds.
 */
import "./src/env.js";

/** @type {import("next").NextConfig} */
const config = {
  output: "standalone",
  devIndicators: {
    position: "bottom-right", // top-right, bottom-right, top-left, bottom-left
  },
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "**.github.com",
      },
      {
        protocol: "https",
        hostname: "**.githubusercontent.com",
      },
    ],
  },
};

export default config;
