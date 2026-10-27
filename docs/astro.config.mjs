import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";
import mermaid from "astro-mermaid";
import starlightLinksValidator from "starlight-links-validator";

export default defineConfig({
  site: "https://tuppr.home-operations.com",
  integrations: [
    // Must precede starlight so its markdown transform sees the mermaid fences first.
    mermaid(),
    starlight({
      title: "tuppr",
      description:
        "Kubernetes controller for automated, orchestrated Talos Linux and Kubernetes upgrades.",
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/home-operations/tuppr",
        },
      ],
      editLink: {
        baseUrl: "https://github.com/home-operations/tuppr/edit/main/docs/",
      },
      sidebar: [
        "index",
        { label: "Getting started", items: ["requirements", "quickstart"] },
        { label: "Concepts", items: ["coordination", "versioning"] },
        {
          label: "User guide",
          items: [
            "talos-upgrades",
            "kubernetes-upgrades",
            "notifications",
            "monitoring",
            "operations",
            "configuration",
          ],
        },
        "development",
      ],
      plugins: [starlightLinksValidator()],
    }),
  ],
});
