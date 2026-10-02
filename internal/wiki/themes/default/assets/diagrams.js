/* ============================================================================
   okf-wiki: diagram runtime and lightbox
   ----------------------------------------------------------------------------
   Two jobs, one module:
     1. Render `pre.mermaid` blocks with mermaid, at natural size so a wide
        topology chart is legible and pans instead of being squashed to an
        illegible sliver. The vendored bundle under BASE vendor/ is tried
        first so a container renders offline; the CDN is a fallback that only
        runs when the bundle is missing, and a render without --vendor or
        OKF_WIKI_VENDOR resolves is therefore not offline.
     2. Open a full-screen pan/zoom viewer when a diagram is clicked (scroll =
        zoom, drag = pan, pinch on touch, +/-/0/esc keys, focus trap).
        Dependency-free.
   If mermaid cannot be loaded the raw source stays in place: legible, not
   broken.
   ========================================================================== */
(function () {
  'use strict';

  var V = window.__WIKI_ASSET ? '?v=' + window.__WIKI_ASSET : '';
  var BASE = window.__WIKI_BASE || '/wiki/';
  // Offline bundle (mermaid + ELK, vendored into the image at build time).
  var LOCAL_BUNDLE = BASE + 'vendor/mermaid-bundle.min.mjs';
  // Exact versions, not major ranges: this is a fallback for a build where the
  // vendor step was skipped, and a moving tag would mean the diagram a page
  // shows can change without the page changing. Pinned in mermaid/package.json.
  var CDN = 'https://cdn.jsdelivr.net/npm/mermaid@12.0.0/dist/mermaid.esm.min.mjs';
  var ELK_CDN = 'https://cdn.jsdelivr.net/npm/@mermaid-js/layout-elk@1.0.0/dist/mermaid-layout-elk.esm.min.mjs';

  function isDark() {
    var t = document.documentElement.getAttribute('data-theme');
    if (t === 'dark') return true;
    if (t === 'light') return false;
    return window.matchMedia('(prefers-color-scheme: dark)').matches;
  }

  // The diagram palette is read from the theme's own custom properties rather
  // than hardcoded here, so a theme recolours every diagram by declaring
  // --diagram-* in tokens.css and nothing else. A theme that wants both
  // grounds declares them per ground, the way the default does; the
  // fallbacks are the dark values, so a theme that declares none of them
  // still renders rather than an undefined one.
  function token(name, fallback) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name);
    v = (v || '').trim();
    return v || fallback;
  }

  function diagramPalette(dark) {
    var dim = function (a, b) { return dark ? b : a; };
    return {
      ground:     token('--diagram-ground', '#0C0C0D'),
      surface:    token('--diagram-surface', '#17171A'),
      surfaceAlt: token('--diagram-surface-alt', '#101012'),
      ink:        token('--diagram-ink', '#EDEDEF'),
      muted:      token('--diagram-muted', '#8A8A93'),
      line:       token('--diagram-line', '#3A3A40'),
      accent:     token('--diagram-accent', '#5FD68C'),
      accentSoft: token('--diagram-accent-soft', '#1E3A2A'),
      accentInk:  token('--diagram-accent-ink', '#6EE7A8'),
      accent2:    token('--diagram-accent-2', '#86EFAC'),
      accent3:    token('--diagram-accent-3', '#A7F3C4'),
      yellow:     token('--diagram-yellow', '#F3C83D'),
      cyan:       token('--diagram-cyan', '#67E8F9'),
      red:        token('--diagram-red', '#FCA5A5'),
      amber:      token('--diagram-amber', '#FCD34D'),
      green:      token('--diagram-green', '#86EFAC'),
      blue:       token('--diagram-blue', '#93C5FD'),
      violet:     token('--diagram-violet', '#C4B5FD'),
      pink:       token('--diagram-pink', '#F9A8D8'),
      font:       token('--diagram-font', 'Inter, ui-sans-serif, system-ui, sans-serif'),
      inverse:    dim('#0C0C0D', '#FAFAFA')
    };
  }

  // The theme's palette mapped onto mermaid's `base` theme. `base` is the only
  // mermaid theme that honours themeVariables, so every surface, border, edge,
  // label and diagram-type colour below comes from the palette above rather
  // than from mermaid's stock defaults.
  function mermaidConfig(dark, useElk) {
    var p = diagramPalette(dark);
    var cfg = {
      startOnLoad: false,
      suppressErrorRendering: true,
      securityLevel: 'strict',
      theme: 'base',

      themeVariables: {
        darkMode: dark,
        fontFamily: p.font,
        fontSize: 14,
        background: p.ground,

        primaryColor: p.surface,
        primaryTextColor: p.ink,
        primaryBorderColor: p.accent,
        secondaryColor: p.accentSoft,
        secondaryTextColor: p.ink,
        secondaryBorderColor: p.accent,
        tertiaryColor: p.surfaceAlt,
        tertiaryTextColor: p.ink,
        tertiaryBorderColor: p.line,

        mainBkg: p.surface,
        nodeBorder: p.accent,
        nodeTextColor: p.ink,

        textColor: p.ink,
        titleColor: p.ink,
        lineColor: p.accent,
        edgeLabelBackground: p.ground,
        clusterBkg: p.accentSoft,
        clusterBorder: p.line,
        labelBackground: p.ground,
        labelTextColor: p.ink,

        noteBkgColor: p.ground,
        noteTextColor: p.amber,
        noteBorderColor: p.line,
        errorBkgColor: p.surfaceAlt,
        errorTextColor: p.red,

        actorBkg: p.surface,
        actorBorder: p.accent,
        actorTextColor: p.ink,
        actorLineColor: p.line,
        signalColor: p.accent,
        signalTextColor: p.ink,
        labelBoxBkgColor: p.accentSoft,
        labelBoxBorderColor: p.accent,
        labelBoxTextColor: p.accentInk,
        loopTextColor: p.muted,
        activationBkgColor: p.accentSoft,
        activationBorderColor: p.accent,
        sequenceNumberColor: p.inverse,

        stateBkg: p.surface,
        stateBorder: p.accent,
        // Mermaid paints the state name in the node fill when this is unset,
        // so without it every state box is empty in both grounds.
        stateLabelColor: p.ink,
        compositeBackground: p.surfaceAlt,
        compositeBorder: p.line,
        labelBackgroundColor: p.ground,
        classText: p.ink,

        sectionBkgColor: p.accentSoft,
        altSectionBkgColor: p.surface,
        sectionBkgColor2: p.surfaceAlt,
        taskBkgColor: p.accent,
        taskBorderColor: p.accent,
        taskTextColor: p.inverse,
        activeTaskBkgColor: p.accent2,
        activeTaskBorderColor: p.accent2,
        doneTaskBkgColor: p.surfaceAlt,
        doneTaskBorderColor: p.line,
        critBkgColor: p.surfaceAlt,
        critBorderColor: p.red,
        todayLineColor: p.accent,
        gridColor: p.line,
        commitLabelColor: p.ink,
        commitLabelBackground: p.surfaceAlt
      },
      // Small, safe polish on mermaid's own SVG: rounded subgraph frames and a
      // consistent 1.4px stroke on nodes and edges.
      themeCSS: [
        '.cluster rect { rx: 10px; ry: 10px; }',
        '.node rect, .node circle, .node ellipse, .node polygon, .node path { stroke-width: 1.4px; }',
        '.edgePath .path { stroke-width: 1.4px; }'
      ].join('\n')
    };

    if (useElk) { cfg.layout = 'elk'; cfg.elk = { mergeEdges: true }; }

    // Each renderer reads useMaxWidth from its own config block, not from the
    // top level, so every type has to be told. Natural width is what lets a
    // wide diagram scroll sideways in its card instead of shrinking its
    // labels to fit the column.
    var TYPES = ['flowchart', 'state', 'sequence', 'class', 'er', 'gantt',
      'gitGraph', 'journey', 'mindmap', 'pie', 'quadrantChart', 'timeline',
      'xyChart', 'block', 'requirement', 'c4', 'packet', 'kanban', 'radar',
      'sankey', 'treemap', 'venn', 'architecture', 'ishikawa', 'treeView',
      'usecase', 'swimlane', 'cynefin', 'eventmodeling', 'agentflow'];
    TYPES.forEach(function (t) {
      cfg[t] = Object.assign({}, cfg[t], { useMaxWidth: false });
    });

    // The sequence diagram's font sizes are not settable from here. Its
    // renderer reads them through an accessor bound to a snapshot of its own
    // defaults, so getConfig() reports the values below while the SVG still
    // carries the default 16px inline. stylesheet.css pins those classes
    // instead; a config key here would only look like it worked.

    // Categorical scales for pie, quadrant and git diagrams. Ordered so
    // adjacent slices stay distinguishable, and every entry is a palette
    // member rather than a new value.
    var pal = [p.accent, p.yellow, p.cyan, p.violet, p.red, p.accent2,
      p.amber, p.blue, p.accent3, p.green, p.pink, p.muted];
    for (var i = 0; i < 12; i++) {
      cfg.themeVariables['pie' + (i + 1)] = pal[i];
      cfg.themeVariables['cScale' + i] = pal[i];
      if (i < 8) {
        cfg.themeVariables['git' + i] = pal[i];
        cfg.themeVariables['gitInv' + i] = p.inverse;
      }
    }
    return cfg;
  }

  async function renderAll() {
    var diagrams = document.querySelectorAll('pre.mermaid');
    if (!diagrams.length) return;

    var mermaid, useElk = false;
    // Prefer the offline vendor bundle (mermaid + ELK in one module). Fall back
    // to the CDN for local development without a vendor step.
    try {
      mermaid = (await import(LOCAL_BUNDLE + V)).default;
      useElk = true;
    } catch (e) {
      try {
        mermaid = (await import(CDN)).default;
      } catch (err) {
        console.warn('Mermaid runtime unavailable; showing diagram source:', err);
        return;
      }
      try {
        var elk = await import(ELK_CDN);
        mermaid.registerLayoutLoaders(elk.default);
        useElk = true;
      } catch (err) {
        console.warn('ELK layout unavailable; using default mermaid layout:', err);
      }
    }

    mermaid.initialize(mermaidConfig(isDark(), useElk));

    var nodes = Array.prototype.slice.call(diagrams);
    nodes.forEach(function (el) { el.__mmdSource = el.textContent; });

    var failed = [];
    for (var i = 0; i < nodes.length; i++) {
      try {
        await mermaid.run({ nodes: [nodes[i]] });
        decorate(nodes[i]);
      } catch (err) {
        failed.push({ el: nodes[i], err: err });
      }
    }

    // A diagram can fail under ELK (or a theme option) yet succeed with mermaid's
    // default layout. Retry failures once without ELK before showing the source.
    if (failed.length && useElk) {
      mermaid.initialize(mermaidConfig(isDark(), false));
      for (var j = 0; j < failed.length; j++) {
        var el = failed[j].el;
        el.textContent = el.__mmdSource;
        el.removeAttribute('data-processed');
        try {
          await mermaid.run({ nodes: [el] });
          decorate(el);
        } catch (err2) {
          console.warn('Mermaid rendering failed:', err2 && err2.message ? err2.message : err2);
          showFailure(el, err2);
        }
      }
    } else {
      failed.forEach(function (f) {
        console.warn('Mermaid rendering failed:', f.err && f.err.message ? f.err.message : f.err);
        showFailure(f.el, f.err);
      });
    }
    initLightbox();
  }

  // A diagram that will not parse becomes a card carrying the source and the
  // parser's own message, so the mistake is readable where it is written. A
  // failed run empties the element, so the source is restored first.
  function showFailure(pre, err) {
    pre.textContent = pre.__mmdSource;
    pre.removeAttribute('data-processed');
    pre.classList.add('mermaid-fallback');

    var msg = err && err.message ? String(err.message) : 'Mermaid could not render this diagram.';
    // The parser reports the offending text after a colon; keep the whole
    // message, since the useful part varies by diagram type.
    var frame = wrapFrame(pre);
    var bar = document.createElement('div');
    bar.className = 'mermaid-toolbar';
    var kind = document.createElement('span');
    kind.className = 'mermaid-type';
    kind.textContent = 'Diagram';
    bar.appendChild(kind);
    frame.insertBefore(bar, pre);

    var card = document.createElement('div');
    card.className = 'mermaid-error';
    card.setAttribute('role', 'alert');

    var head = document.createElement('p');
    head.className = 'mermaid-error-head';
    head.textContent = 'This diagram did not render';
    var detail = document.createElement('p');
    detail.className = 'mermaid-error-detail';
    detail.textContent = msg;
    card.appendChild(head);
    card.appendChild(detail);

    // The source keeps its own element, so a reader can select and copy it.
    var src = document.createElement('pre');
    src.className = 'mermaid-error-source';
    src.textContent = pre.__mmdSource;
    card.appendChild(src);

    pre.hidden = true;
    frame.appendChild(card);
  }

  // Mermaid's own aria-roledescription, mapped to the names a reader knows.
  // An unmapped type falls through as written rather than as "diagram".
  var TYPE_NAMES = {
    'flowchart-v2': 'Flowchart',
    'sequence': 'Sequence diagram',
    'stateDiagram': 'State diagram',
    'classDiagram': 'Class diagram',
    'er': 'Entity relationship',
    'gantt': 'Gantt chart',
    'pie': 'Pie chart',
    'gitGraph': 'Git graph',
    'journey': 'User journey',
    'mindmap': 'Mind map',
    'quadrantChart': 'Quadrant chart',
    'timeline': 'Timeline',
    'xychart': 'XY chart',
    'block': 'Block diagram',
    'requirement': 'Requirement diagram',
    'c4': 'C4 diagram'
  };

  function diagramType(pre) {
    var svg = pre.querySelector('svg');
    var role = svg ? svg.getAttribute('aria-roledescription') : '';
    if (!role) return 'Diagram';
    return TYPE_NAMES[role] || role;
  }

  function iconButton(cls, label, path, onClick) {
    var b = document.createElement('button');
    b.type = 'button';
    b.className = cls;
    b.setAttribute('aria-label', label);
    b.title = label;
    b.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" ' +
      'stroke="currentColor" stroke-width="2" aria-hidden="true">' + path + '</svg>';
    b.addEventListener('click', function (e) {
      e.stopPropagation();
      onClick();
    });
    return b;
  }

  // The card a diagram lives in. Shared by a rendered diagram and a failed
  // one, so an error gets the same frame, toolbar and width as a drawing.
  function wrapFrame(pre) {
    var existing = pre.closest('.mermaid-frame');
    if (existing) return existing;
    var frame = document.createElement('div');
    frame.className = 'mermaid-frame';
    pre.parentNode.insertBefore(frame, pre);
    frame.appendChild(pre);
    return frame;
  }

  function decorate(pre) {
    if (pre.dataset.decorated) return;
    pre.dataset.decorated = '1';
    pre.classList.add('mermaid-rendered');

    var frame = wrapFrame(pre);

    var bar = document.createElement('div');
    bar.className = 'mermaid-toolbar';
    var kind = document.createElement('span');
    kind.className = 'mermaid-type';
    kind.textContent = diagramType(pre);
    bar.appendChild(kind);

    var actions = document.createElement('span');
    actions.className = 'mermaid-actions';

    // Zoom lives in the card, not only in the viewer: a diagram taller or wider
    // than the card opens fitted to it, and these step the scale up or down.
    var zoom = document.createElement('span');
    zoom.className = 'mermaid-zoom';
    var out = iconButton('mermaid-zoom-out', 'Zoom out', '<path d="M5 12h14"/>',
      function () { cardZoom(frame, 0.8); });
    var pct = document.createElement('button');
    pct.type = 'button';
    pct.className = 'mermaid-zoom-pct';
    pct.setAttribute('aria-label', 'Fit diagram to the card');
    pct.title = 'Fit diagram to the card';
    pct.addEventListener('click', function (e) {
      e.stopPropagation();
      cardFit(frame);
    });
    var zin = iconButton('mermaid-zoom-in', 'Zoom in', '<path d="M12 5v14M5 12h14"/>',
      function () { cardZoom(frame, 1.25); });
    zoom.appendChild(out);
    zoom.appendChild(pct);
    zoom.appendChild(zin);
    actions.appendChild(zoom);

    actions.appendChild(iconButton('mermaid-copy', 'Copy diagram source',
      '<rect x="9" y="9" width="12" height="12" rx="2"/>' +
      '<path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
      function () { copySource(pre, actions); }));
    actions.appendChild(iconButton('mermaid-expand', 'Open diagram in full screen',
      '<path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/>',
      function () { openLightbox(pre); }));
    bar.appendChild(actions);
    frame.insertBefore(bar, pre);

    cardInit(frame, pct);
  }

  // The card's own zoom, separate from the viewer. The diagram is resized by
  // setting the SVG's size, so the card's scroll area takes over once a zoomed
  // diagram outgrows it.
  function naturalSize(svg) {
    var vb = svg && svg.getAttribute('viewBox');
    if (vb) {
      var p = vb.split(/[ ,]+/).map(Number);
      if (p[2] && p[3]) return { w: p[2], h: p[3] };
    }
    var r = svg ? svg.getBoundingClientRect() : null;
    return { w: (r && r.width) || 1, h: (r && r.height) || 1 };
  }

  function cardInit(frame, pct) {
    var svg = frame.querySelector('pre.mermaid svg');
    if (!svg) return;
    var nat = naturalSize(svg);
    frame.__zoom = { w: nat.w, h: nat.h, scale: 1, fit: 1, pct: pct };
    cardFit(frame);
  }

  function cardApply(frame) {
    var z = frame.__zoom;
    var svg = frame.querySelector('pre.mermaid svg');
    if (!z || !svg) return;
    svg.style.width = (z.w * z.scale) + 'px';
    svg.style.height = (z.h * z.scale) + 'px';
    svg.style.maxWidth = 'none';
    z.pct.textContent = Math.round(z.scale * 100) + '%';
  }

  // The scale at which the whole drawing fits the card, never past natural size.
  function cardFitScale(frame) {
    var z = frame.__zoom;
    var pre = frame.querySelector('pre.mermaid');
    if (!z || !pre) return 1;
    var cs = getComputedStyle(pre);
    var availW = pre.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight);
    var cap = parseFloat(cs.maxHeight);
    var boxH = isFinite(cap) ? cap : pre.clientHeight;
    var availH = boxH - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom);
    if (availW <= 0 || availH <= 0) return 1;
    return Math.min(availW / z.w, availH / z.h, 1);
  }

  function cardFit(frame) {
    var z = frame.__zoom;
    if (!z) return;
    z.fit = cardFitScale(frame);
    z.scale = z.fit;
    cardApply(frame);
  }

  function cardZoom(frame, f) {
    var z = frame.__zoom;
    if (!z) return;
    z.scale = clamp(z.scale * f, 0.1, 4);
    cardApply(frame);
  }

  // A width change re-fits a diagram the reader has not zoomed, and leaves a
  // zoomed one where they put it.
  function refitCards() {
    document.querySelectorAll('.mermaid-frame').forEach(function (frame) {
      var z = frame.__zoom;
      if (!z || Math.abs(z.scale - z.fit) > 1e-6) return;
      cardFit(frame);
    });
  }
  window.addEventListener('resize', refitCards);

  // Copy the fence body, not the rendered text: the source is what a reader
  // wants to paste elsewhere. Feedback is the button's own title, so no text
  // appears in the toolbar.
  function copySource(pre, actions) {
    var src = pre.__mmdSource || pre.textContent;
    var done = function (ok) {
      var label = ok ? 'Copied' : 'Copy failed';
      actions.setAttribute('data-copy', ok ? 'ok' : 'fail');
      actions.setAttribute('aria-label', label);
      setTimeout(function () {
        actions.removeAttribute('data-copy');
        actions.setAttribute('aria-label', '');
      }, 1200);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(src).then(function () { done(true); }, function () { done(false); });
      return;
    }
    done(false);
  }

  /* ------------------------------------------------------------- lightbox */
  var lb = null;
  var state = null;
  var nat = { w: 1, h: 1 };
  var prevOverflow = null;
  var prevFocus = null;
  var pointers = new Map();
  var pinch = null;

  function clamp(v, lo, hi) { return Math.min(hi, Math.max(lo, v)); }
  function zoomKeepingPoint(st, cx, cy, nextScale) {
    var k = nextScale / st.scale;
    return { scale: nextScale, tx: cx - (cx - st.tx) * k, ty: cy - (cy - st.ty) * k };
  }
  function pinchDistance(a, b) { return Math.hypot(a.x - b.x, a.y - b.y); }
  function pinchMidpoint(a, b) { return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 }; }

  function lbApply() {
    if (!state || !lb) return;
    lb.content.style.transform =
      'translate(' + state.tx + 'px,' + state.ty + 'px) scale(' + state.scale + ')';
    lb.pct.textContent = Math.round(state.scale * 100) + '%';
  }
  function lbZoomAt(f, cx, cy) {
    if (!state) return;
    var next = zoomKeepingPoint(state, cx, cy, clamp(state.scale * f, 0.2, 12));
    state.tx = next.tx; state.ty = next.ty; state.scale = next.scale;
    lbApply();
  }
  function lbZoom(f) {
    if (!lb) return;
    var r = lb.stage.getBoundingClientRect();
    lbZoomAt(f, r.width / 2, r.height / 2);
  }
  function lbFit() {
    if (!lb) return;
    var sw = lb.stage.clientWidth || window.innerWidth;
    var sh = lb.stage.clientHeight || (window.innerHeight - 68);
    var cw = nat.w || 1, ch = nat.h || 1;
    var fit = Math.min((sw / cw) * 0.9, (sh / ch) * 0.86);
    // Open on the whole drawing. A diagram larger than the stage is scaled
    // down to fit, because an edge off-screen is not something a reader knows
    // to pan for; a smaller one still grows a little, which is the case the
    // viewer is for. Zooming in and panning is how detail is reached.
    var scale = Math.min(fit, 1.25);
    state = { scale: scale, tx: (sw - cw * scale) / 2, ty: (sh - ch * scale) / 2, fit: scale };
    lbApply();
  }

  function buildLightbox() {
    if (lb) return lb;
    var el = document.createElement('div');
    el.className = 'wiki-lbx';
    el.setAttribute('role', 'dialog');
    el.setAttribute('aria-modal', 'true');
    el.setAttribute('aria-label', 'Diagram viewer');
    el.innerHTML =
      '<div class="wiki-lbx-bar">' +
        '<div class="wiki-lbx-id"><span class="wiki-lbx-pill">Diagram</span><span class="wiki-lbx-title"></span></div>' +
        '<div class="wiki-lbx-ctrls">' +
          '<button type="button" class="wiki-lbx-btn" data-act="out" aria-label="Zoom out"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"><path d="M3.5 8h9"/></svg></button>' +
          '<button type="button" class="wiki-lbx-btn" data-act="fit" aria-label="Fit to screen">100%</button>' +
          '<button type="button" class="wiki-lbx-btn" data-act="in" aria-label="Zoom in"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"><path d="M8 3.5v9M3.5 8h9"/></svg></button>' +
          '<button type="button" class="wiki-lbx-btn" data-act="close" aria-label="Close viewer" style="margin-left:6px"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M4 4l8 8M12 4l-8 8"/></svg></button>' +
        '</div>' +
      '</div>' +
      '<div class="wiki-lbx-stage"><div class="wiki-lbx-content"></div><div class="wiki-lbx-hint"></div></div>';
    document.body.appendChild(el);

    var stage = el.querySelector('.wiki-lbx-stage');
    lb = {
      el: el, stage: stage,
      content: el.querySelector('.wiki-lbx-content'),
      title: el.querySelector('.wiki-lbx-title'),
      pct: el.querySelector('[data-act="fit"]'),
      hint: el.querySelector('.wiki-lbx-hint')
    };

    el.querySelector('[data-act="out"]').onclick = function () { lbZoom(0.8); };
    el.querySelector('[data-act="in"]').onclick = function () { lbZoom(1.25); };
    lb.pct.onclick = function () { lbFit(); };
    el.querySelector('[data-act="close"]').onclick = function () { closeLightbox(); };
    el.addEventListener('pointerdown', function (e) { if (e.target === el) closeLightbox(); });
    // Clicking the field around the diagram closes too, which is what "click
    // outside" means once the viewer fills the screen. A drag to pan is not a
    // click, so the pointer has to stay put between down and up.
    var downAt = null;
    stage.addEventListener('pointerdown', function (e) { downAt = { x: e.clientX, y: e.clientY }; });
    stage.addEventListener('click', function (e) {
      if (e.target !== stage || !downAt) return;
      if (Math.abs(e.clientX - downAt.x) > 4 || Math.abs(e.clientY - downAt.y) > 4) return;
      closeLightbox();
    });

    stage.addEventListener('wheel', function (e) {
      e.preventDefault();
      var r = stage.getBoundingClientRect();
      lbZoomAt(Math.exp(-e.deltaY * 0.0015), e.clientX - r.left, e.clientY - r.top);
    }, { passive: false });

    stage.addEventListener('pointerdown', function (e) {
      stage.setPointerCapture(e.pointerId);
      pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
      pinch = null;
      stage.classList.add('is-grabbing');
    });
    stage.addEventListener('pointermove', function (e) {
      if (!state || !pointers.has(e.pointerId)) return;
      var prev = pointers.get(e.pointerId);
      pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
      var pts = Array.from(pointers.values());
      var r = stage.getBoundingClientRect();
      if (pts.length === 1) {
        state.tx += e.clientX - prev.x; state.ty += e.clientY - prev.y; lbApply();
      } else if (pts.length >= 2) {
        var a = pts[0], b = pts[1];
        var dist = pinchDistance(a, b);
        var m = pinchMidpoint(a, b);
        var mid = { x: m.x - r.left, y: m.y - r.top };
        if (pinch) {
          lbZoomAt(dist / pinch.dist, mid.x, mid.y);
          state.tx += mid.x - pinch.mid.x; state.ty += mid.y - pinch.mid.y; lbApply();
        }
        pinch = { dist: dist, mid: mid };
      }
    });
    var up = function (e) {
      pointers.delete(e.pointerId);
      if (pointers.size < 2) pinch = null;
      if (pointers.size === 0) stage.classList.remove('is-grabbing');
    };
    stage.addEventListener('pointerup', up);
    stage.addEventListener('pointercancel', up);
    stage.addEventListener('dblclick', function (e) {
      if (!state) return;
      var r = stage.getBoundingClientRect();
      if (state.scale > state.fit * 1.4) lbFit();
      else lbZoomAt(2 / (state.scale / state.fit), e.clientX - r.left, e.clientY - r.top);
    });

    document.addEventListener('keydown', function (e) {
      if (!lb || !lb.el.classList.contains('is-open')) return;
      if (e.key === 'Escape') closeLightbox();
      else if (e.key === '+' || e.key === '=') lbZoom(1.25);
      else if (e.key === '-' || e.key === '_') lbZoom(0.8);
      else if (e.key === '0') lbFit();
      else if (e.key === 'Tab') {
        var f = Array.prototype.filter.call(lb.el.querySelectorAll('button'), function (b) { return b.offsetParent !== null; });
        if (!f.length) return;
        var first = f[0], last = f[f.length - 1], active = document.activeElement;
        var inside = lb.el.contains(active);
        if (e.shiftKey && (active === first || !inside)) { e.preventDefault(); last.focus(); }
        else if (!e.shiftKey && (active === last || !inside)) { e.preventDefault(); first.focus(); }
      }
    });
    return lb;
  }

  // A heading's own text, without the # permalink the client appends to it.
  function headingLabel(h) {
    var clone = h.cloneNode(true);
    var anchor = clone.querySelector('.heading-anchor');
    if (anchor) anchor.remove();
    return clone.textContent.trim();
  }

  function titleFor(pre) {
    // Nearest preceding heading in the article is a better label than "Diagram".
    // Start from the frame: the diagram is wrapped, so the <pre> itself has no
    // previous sibling.
    var node = pre.closest('.mermaid-frame') || pre;
    while (node && node.previousElementSibling) {
      node = node.previousElementSibling;
      if (/^H[1-4]$/.test(node.tagName)) return headingLabel(node);
    }
    var h1 = document.querySelector('article h1');
    return (h1 && headingLabel(h1)) || 'Diagram';
  }

  function openLightbox(pre) {
    var svg = pre && pre.querySelector('svg');
    if (!svg) return;
    buildLightbox();
    if (!lb) return;

    // The stage takes the diagram's own ground so the drawing sits on the same
    // field it was rendered against, rather than on a guess at one.
    lb.stage.style.background = diagramPalette(isDark()).ground;

    var w = 0, h = 0;
    var vb = svg.getAttribute('viewBox');
    if (vb) { var p = vb.split(/[ ,]+/).map(Number); w = p[2]; h = p[3]; }
    if (!w || !h) { var r = svg.getBoundingClientRect(); w = r.width; h = r.height; }
    nat = { w: w, h: h };

    var clone = svg.cloneNode(true);
    clone.style.width = w + 'px';
    clone.style.height = h + 'px';
    clone.style.maxWidth = 'none';
    clone.style.display = 'block';
    lb.content.innerHTML = '';
    lb.content.appendChild(clone);

    lb.title.textContent = titleFor(pre);
    var touch = window.matchMedia && window.matchMedia('(pointer:coarse)').matches;
    lb.hint.textContent = touch
      ? 'pinch to zoom · drag to pan · close to exit'
      : 'scroll to zoom · drag to pan · double-click to toggle · esc to close';

    lb.el.classList.add('is-open');
    if (prevOverflow == null) prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    prevFocus = document.activeElement;
    var closeBtn = lb.el.querySelector('[data-act="close"]');
    if (closeBtn) { try { closeBtn.focus({ preventScroll: true }); } catch (e) {} }
    requestAnimationFrame(function () { requestAnimationFrame(lbFit); });
  }

  function closeLightbox() {
    if (!lb) return;
    lb.el.classList.remove('is-open');
    pointers.clear();
    pinch = null;
    document.body.style.overflow = prevOverflow || '';
    prevOverflow = null;
    if (prevFocus && prevFocus.focus) { try { prevFocus.focus(); } catch (e) {} }
  }

  function initLightbox() {
    if (document.__wikiDiagramLb) return;
    document.__wikiDiagramLb = true;
    document.addEventListener('click', function (e) {
      var target = e.target;
      if (!(target instanceof Element)) return;
      if (target.closest('.mermaid-expand')) return; // handled by the button itself
      var pre = target.closest('pre.mermaid.mermaid-rendered');
      if (pre) openLightbox(pre);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', renderAll);
  } else {
    renderAll();
  }
})();
