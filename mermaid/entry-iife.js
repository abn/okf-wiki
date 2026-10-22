import mermaid from "mermaid";
import elk from "@mermaid-js/layout-elk";

// A classic bundle for file:// pages: dynamic import of a local module is
// blocked there, but a plain script tag is not. ELK is registered before the
// global is published, matching the ESM entry.
mermaid.registerLayoutLoaders(elk);
window.__WIKI_MERMAID = mermaid;
