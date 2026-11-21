import embed from "vega-embed";
import { expressionInterpreter } from "vega-interpreter";

// One classic bundle for every mode. charts.js injects it as a script tag the
// first time a chart nears the viewport, which works from file:// as well as
// over HTTP, so there is no separate module build. The interpreter lets a
// chart render under a Content-Security-Policy without 'unsafe-eval'.
window.__WIKI_VEGA = { embed, expressionInterpreter };
