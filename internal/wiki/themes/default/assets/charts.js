/* ============================================================================
   okf-wiki: chart runtime
   ----------------------------------------------------------------------------
   Draws the figure.chart blocks a vega-lite fence renders to. The page only
   carries this script when it has a chart, and the Vega runtime itself is
   fetched the first time a chart comes near the viewport, so a reader who
   never scrolls to one never downloads it.

   The runtime is one classic bundle under BASE vendor/, injected as a script
   tag, which works from file:// as well as over HTTP. A served page with no
   vendored bundle falls back to pinned CDN builds; an offline render never
   does.

   Colours go to Vega as var(--chart-*) references, not resolved values, so
   the drawn SVG follows a theme toggle and prints in the light palette
   without being drawn again. The font is resolved, because Vega measures
   text with it.
   ========================================================================== */
(function () {
  'use strict';

  var V = window.__WIKI_ASSET ? '?v=' + window.__WIKI_ASSET : '';
  var BASE = window.__WIKI_BASE || '/wiki/';
  var LOCAL_BUNDLE = BASE + 'vendor/vega-bundle.min.js';
  // Exact versions, matching mermaid/package.json, so a chart cannot change
  // without the page changing. The interpreter has no classic build, so a
  // chart drawn from the CDN needs a policy that allows eval.
  var CDN = [
    'https://cdn.jsdelivr.net/npm/vega@6.4.0/build/vega.min.js',
    'https://cdn.jsdelivr.net/npm/vega-lite@6.5.0/build/vega-lite.min.js',
    'https://cdn.jsdelivr.net/npm/vega-embed@7.3.0/build/vega-embed.min.js'
  ];

  var runtime = null;

  function loadScript(src) {
    return new Promise(function (resolve, reject) {
      var s = document.createElement('script');
      s.src = src;
      s.async = false;
      s.onload = function () { resolve(); };
      s.onerror = function () { reject(new Error('could not load ' + src)); };
      document.head.appendChild(s);
    });
  }

  function loadRuntime() {
    if (runtime) return runtime;
    runtime = (async function () {
      if (window.__WIKI_VEGA) return window.__WIKI_VEGA;
      try {
        await loadScript(LOCAL_BUNDLE + V);
        if (window.__WIKI_VEGA) return window.__WIKI_VEGA;
      } catch (e) {
        if (location.protocol === 'file:' || window.__WIKI_OFFLINE) throw e;
      }
      for (var i = 0; i < CDN.length; i++) await loadScript(CDN[i]);
      if (!window.vegaEmbed) throw new Error('chart runtime unavailable');
      return { embed: window.vegaEmbed, expressionInterpreter: null };
    })();
    return runtime;
  }

  function token(name, fallback) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  }

  function ref(name) { return 'var(' + name + ')'; }

  // The theme's defaults. A spec's own config is merged over this, so an
  // author can still set a colour or a font for one chart.
  function themeConfig() {
    var font = token('--chart-font', 'system-ui, sans-serif');
    var ink = ref('--chart-ink');
    var muted = ref('--chart-muted');
    var series = [1, 2, 3, 4, 5, 6].map(function (i) { return ref('--chart-' + i); });
    return {
      background: null,
      font: font,
      view: { stroke: null },
      axis: {
        labelColor: muted, titleColor: muted,
        gridColor: ref('--chart-grid'), domainColor: ref('--chart-axis'), tickColor: ref('--chart-axis'),
        labelFontSize: 11, titleFontSize: 11, titleFontWeight: 500
      },
      axisBand: { labelAngle: 0 },
      legend: {
        labelColor: ink, titleColor: muted, labelFontSize: 12, titleFontSize: 11,
        titleFontWeight: 500, orient: 'top', symbolStrokeWidth: 2
      },
      header: { labelColor: ink, titleColor: muted },
      title: { color: ink, subtitleColor: muted, anchor: 'start', fontSize: 14, fontWeight: 600 },
      text: { color: ink },
      rule: { color: muted },
      tick: { color: muted },
      mark: { color: series[0] },
      range: { category: series }
    };
  }

  function fail(fig, err) {
    var view = fig.querySelector('.chart-view');
    var msg = err && err.message ? String(err.message) : 'unknown error';
    console.warn('Chart could not be drawn:', msg);
    if (view) {
      view.classList.add('is-drawn');
      view.innerHTML = '';
      var box = document.createElement('div');
      box.className = 'chart-error';
      box.setAttribute('role', 'note');
      box.textContent = 'This chart could not be drawn: ' + msg;
      view.appendChild(box);
    }
    var data = fig.querySelector('.chart-data');
    if (data) data.open = true;
  }

  function draw(fig) {
    if (fig.__chart) return fig.__chart;
    var view = fig.querySelector('.chart-view');
    var specEl = fig.querySelector('.chart-spec');
    var spec;
    try {
      spec = JSON.parse(specEl.textContent);
    } catch (e) {
      fail(fig, e);
      fig.__chart = Promise.resolve();
      return fig.__chart;
    }
    fig.__chart = loadRuntime().then(function (rt) {
      var opts = {
        mode: 'vega-lite', renderer: 'svg', actions: false,
        config: themeConfig(), tooltip: { theme: 'custom' }
      };
      if (rt.expressionInterpreter) {
        opts.ast = true;
        opts.expr = rt.expressionInterpreter;
      }
      return rt.embed(view, spec, opts);
    }).then(function () {
      view.classList.add('is-drawn');
      fig.setAttribute('data-drawn', '');
    }).catch(function (err) { fail(fig, err); });
    return fig.__chart;
  }

  function init() {
    var figs = Array.prototype.slice.call(document.querySelectorAll('figure.chart[data-chart="vega-lite"]'));
    if (!figs.length) return;

    if (!('IntersectionObserver' in window)) {
      figs.forEach(draw);
    } else {
      var io = new IntersectionObserver(function (entries) {
        entries.forEach(function (e) {
          if (!e.isIntersecting) return;
          io.unobserve(e.target);
          draw(e.target);
        });
      }, { rootMargin: '200px 0px' });
      figs.forEach(function (f) { io.observe(f); });
    }

    // Printing does not wait for a chart that has not been drawn yet, so its
    // data table is opened for the printout and closed again afterwards.
    var opened = [];
    window.addEventListener('beforeprint', function () {
      figs.forEach(function (f) {
        if (f.hasAttribute('data-drawn')) return;
        draw(f);
        var d = f.querySelector('.chart-data');
        if (d && !d.open) { d.open = true; opened.push(d); }
      });
    });
    window.addEventListener('afterprint', function () {
      opened.forEach(function (d) { d.open = false; });
      opened = [];
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
