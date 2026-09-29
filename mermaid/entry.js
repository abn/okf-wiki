import mermaid from "mermaid";
import elk from "@mermaid-js/layout-elk";

// Register the compact ELK layout, then re-export mermaid so the browser loads
// mermaid and ELK as a single offline module.
mermaid.registerLayoutLoaders(elk);

export default mermaid;
