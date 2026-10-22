(function () {
  'use strict';

  var app = document.getElementById('graphApp');
  if (!app) return;

  var NS = 'http://www.w3.org/2000/svg';
  var W = 1000;
  var H = 700;

  var svg = document.getElementById('graphSvg');
  var edgeLayer = document.getElementById('graphEdges');
  var nodeLayer = document.getElementById('graphNodes');
  var status = document.getElementById('graphStatus');
  var filter = document.getElementById('graphFilter');
  var kind = document.getElementById('graphKind');
  var labels = document.getElementById('graphLabels');
  var fit = document.getElementById('graphFit');
  var download = document.getElementById('graphDownload');
  var details = document.getElementById('graphDetails');

  var base = app.getAttribute('data-graph-base') || '/';
  if (base.charAt(base.length - 1) !== '/') base += '/';

  var state = {
    nodes: [],
    edges: [],
    edgeElements: [],
    raw: '',
    selected: null,
    tabbable: null,
    view: { x: 0, y: 0, w: W, h: H },
    step: 0,
    totalTicks: 0,
    dragging: null,
    panning: null
  };

  // A node press captures the pointer, so the click that follows can land on
  // the svg rather than the node. The background click would then clear the
  // panel the press just opened; swallow that one click instead.
  var swallowClick = false;

  download.disabled = true;

  function setStatus(text, isError) {
    status.textContent = text;
    status.classList.toggle('is-error', !!isError);
  }

  function nodeRadius(n) {
    return 5 + Math.min(10, Math.sqrt(n.degree) * 2.2) + (n.kind === 'tag' ? 1 : 0);
  }

  function labelText(title) {
    return title.length > 32 ? title.slice(0, 31).replace(/\s+$/, '') + '\u2026' : title;
  }

  function ariaLabel(n) {
    var what = n.kind === 'tag' ? 'tag' : n.stub ? 'missing page' : 'page';
    var links = n.degree === 1 ? '1 connection' : n.degree + ' connections';
    return n.title + ', ' + what + ', ' + links;
  }

  function inlineCatalogue() {
    var el = document.getElementById('graphData');
    return el ? (el.textContent || '').trim() : '';
  }

  function load() {
    var text = inlineCatalogue();
    if (text) {
      start(text, 'the page');
      return;
    }
    setStatus('Loading the graph...');
    fetch(base + 'graph.json', { headers: { Accept: 'application/json' } })
      .then(function (res) {
        if (!res.ok) throw new Error('HTTP ' + res.status);
        return res.text();
      })
      .then(function (raw) { start(raw, 'graph.json'); })
      .catch(function (err) {
        var detail = err && err.message ? err.message : 'the request failed';
        setStatus('The graph could not be loaded (' + detail + '). The raw file is at ' + base + 'graph.json.', true);
      });
  }

  function start(raw, source) {
    var data;
    try {
      data = JSON.parse(raw);
    } catch (e) {
      setStatus('The graph catalogue is not valid JSON.', true);
      return;
    }
    if (!data || data.version !== 1 || !Array.isArray(data.nodes) || !Array.isArray(data.edges)) {
      setStatus('The graph catalogue has a shape this view does not know (expected version 1).', true);
      return;
    }

    state.raw = raw;
    state.nodes = data.nodes.map(function (n, i) {
      return {
        id: String(n.id || ''),
        title: String(n.title || n.id || ''),
        url: n.url ? String(n.url) : '',
        kind: n.kind === 'tag' ? 'tag' : 'page',
        section: n.section ? String(n.section) : '',
        tags: Array.isArray(n.tags) ? n.tags.map(String) : [],
        stub: !!n.stub,
        site: n.site ? String(n.site) : '',
        degree: 0,
        x: 0,
        y: 0,
        vx: 0,
        vy: 0,
        index: i,
        el: null,
        circle: null,
        fixed: false
      };
    });

    var byId = Object.create(null);
    state.nodes.forEach(function (n) { byId[n.id] = n; });
    state.edges = [];
    data.edges.forEach(function (e) {
      var from = byId[e.from];
      var to = byId[e.to];
      if (!from || !to) return;
      state.edges.push({
        from: from,
        to: to,
        kind: e.kind === 'tag' ? 'tag' : 'link'
      });
      from.degree += 1;
      to.degree += 1;
    });

    if (!state.nodes.length) {
      setStatus('The catalogue is empty. Add pages to the bundle and render again.');
      return;
    }

    download.disabled = false;
    layoutInitial();
    draw();
    simulateThenRender();
    setStatus('Loaded from ' + source + '.');
    applyFilter();
  }

  function layoutInitial() {
    var n = state.nodes.length;
    var golden = Math.PI * (3 - Math.sqrt(5));
    var radius = Math.min(W, H) * 0.42;
    state.nodes.forEach(function (node, i) {
      var r = radius * Math.sqrt((i + 0.5) / n);
      var a = i * golden;
      node.x = W / 2 + Math.cos(a) * r;
      node.y = H / 2 + Math.sin(a) * r;
      node.vx = 0;
      node.vy = 0;
    });
  }

  function tick() {
    repel();
    springs();
    centre();
    integrate();
  }

  function repel() {
    var nodes = state.nodes;
    var n = nodes.length;

    function pair(a, b) {
      var dx = a.x - b.x;
      var dy = a.y - b.y;
      var d2 = dx * dx + dy * dy;
      if (d2 < 1) {
        dx = Math.random() - 0.5;
        dy = Math.random() - 0.5;
        d2 = dx * dx + dy * dy + 0.01;
      }
      if (d2 > 320 * 320) return;
      var d = Math.sqrt(d2);
      var force = Math.min(14, 3000 / d2);
      var fx = (dx / d) * force;
      var fy = (dy / d) * force;
      a.vx += fx;
      a.vy += fy;
      b.vx -= fx;
      b.vy -= fy;
    }

    if (n <= 300) {
      for (var i = 0; i < n; i++) {
        for (var j = i + 1; j < n; j++) pair(nodes[i], nodes[j]);
      }
      return;
    }

    var cell = 180;
    var grid = Object.create(null);
    for (var k = 0; k < n; k++) {
      var node = nodes[k];
      var key = Math.floor(node.x / cell) + ',' + Math.floor(node.y / cell);
      (grid[key] || (grid[key] = [])).push(k);
    }
    for (var a = 0; a < n; a++) {
      var current = nodes[a];
      var cx = Math.floor(current.x / cell);
      var cy = Math.floor(current.y / cell);
      for (var gx = cx - 1; gx <= cx + 1; gx++) {
        for (var gy = cy - 1; gy <= cy + 1; gy++) {
          var bucket = grid[gx + ',' + gy];
          if (!bucket) continue;
          for (var b = 0; b < bucket.length; b++) {
            var other = bucket[b];
            if (other <= a) continue;
            pair(current, nodes[other]);
          }
        }
      }
    }
  }

  function springs() {
    state.edges.forEach(function (e) {
      var dx = e.to.x - e.from.x;
      var dy = e.to.y - e.from.y;
      var d = Math.sqrt(dx * dx + dy * dy) || 0.01;
      var target = e.kind === 'tag' ? 70 : 110;
      var force = (d - target) * 0.011;
      var fx = (dx / d) * force;
      var fy = (dy / d) * force;
      e.from.vx += fx;
      e.from.vy += fy;
      e.to.vx -= fx;
      e.to.vy -= fy;
    });
  }

  function centre() {
    state.nodes.forEach(function (n) {
      n.vx += (W / 2 - n.x) * 0.0018;
      n.vy += (H / 2 - n.y) * 0.0018;
    });
  }

  function integrate() {
    state.nodes.forEach(function (n) {
      if (n.fixed) {
        n.vx = 0;
        n.vy = 0;
        return;
      }
      n.vx *= 0.87;
      n.vy *= 0.87;
      n.x += n.vx;
      n.y += n.vy;
      if (n.x < 20) { n.x = 20; n.vx *= -0.4; }
      if (n.x > W - 20) { n.x = W - 20; n.vx *= -0.4; }
      if (n.y < 20) { n.y = 20; n.vy *= -0.4; }
      if (n.y > H - 20) { n.y = H - 20; n.vy *= -0.4; }
    });
  }

  function draw() {
    edgeLayer.textContent = '';
    nodeLayer.textContent = '';
    state.edgeElements = [];

    state.edges.forEach(function (e) {
      var line = document.createElementNS(NS, 'line');
      line.setAttribute('class', 'graph-edge ' + e.kind);
      line.setAttribute('vector-effect', 'non-scaling-stroke');
      edgeLayer.appendChild(line);
      state.edgeElements.push({ el: line, edge: e });
    });

    state.nodes.forEach(function (n) {
      var g = document.createElementNS(NS, 'g');
      g.setAttribute('class', 'graph-node-group ' + n.kind + (n.stub ? ' stub' : ''));
      g.setAttribute('data-id', n.id);

      var r = nodeRadius(n);
      var circle = document.createElementNS(NS, 'circle');
      circle.setAttribute('r', String(r));
      circle.setAttribute('tabindex', n.index === 0 ? '0' : '-1');
      circle.setAttribute('role', 'button');
      circle.setAttribute('aria-label', ariaLabel(n));
      circle.addEventListener('focus', function () { setTabbable(n); });
      circle.addEventListener('click', function (ev) {
        ev.stopPropagation();
        swallowClick = false;
        select(n);
      });
      circle.addEventListener('keydown', function (ev) {
        if (ev.key === 'Enter' || ev.key === ' ') {
          ev.preventDefault();
          select(n);
          return;
        }
        if (ev.key === 'ArrowRight' || ev.key === 'ArrowDown') {
          ev.preventDefault();
          focusStep(n, 1);
          return;
        }
        if (ev.key === 'ArrowLeft' || ev.key === 'ArrowUp') {
          ev.preventDefault();
          focusStep(n, -1);
          return;
        }
        if (ev.key === 'Home' || ev.key === 'End') {
          ev.preventDefault();
          focusEdge(ev.key === 'End');
        }
      });
      circle.addEventListener('pointerdown', function (ev) { startDrag(ev, n); });
      g.appendChild(circle);

      var label = document.createElementNS(NS, 'text');
      label.setAttribute('class', 'graph-label');
      label.setAttribute('text-anchor', 'middle');
      label.setAttribute('aria-hidden', 'true');
      label.setAttribute('y', String(-r - 5));
      label.textContent = labelText(n.title);
      g.appendChild(label);

      nodeLayer.appendChild(g);
      n.el = g;
      n.circle = circle;
    });

    applyLabels();
    renderPositions();
    placeLabels();
  }

  function renderPositions() {
    state.nodes.forEach(function (n) {
      if (n.el) {
        n.el.setAttribute('transform', 'translate(' + n.x.toFixed(2) + ',' + n.y.toFixed(2) + ')');
      }
    });
    state.edgeElements.forEach(function (item) {
      var e = item.edge;
      item.el.setAttribute('x1', e.from.x.toFixed(2));
      item.el.setAttribute('y1', e.from.y.toFixed(2));
      item.el.setAttribute('x2', e.to.x.toFixed(2));
      item.el.setAttribute('y2', e.to.y.toFixed(2));
    });
  }

  function simulateThenRender() {
    var n = state.nodes.length;
    state.totalTicks = n > 600 ? 120 : n > 300 ? 200 : 400;
    state.step = 0;

    if (n <= 250) {
      for (; state.step < state.totalTicks; state.step++) tick();
      fitView();
      renderPositions();
      placeLabels();
      return;
    }

    setStatus('Laying out ' + n + ' nodes...');
    function frame() {
      var end = Math.min(state.step + 4, state.totalTicks);
      for (; state.step < end; state.step++) tick();
      renderPositions();
      if (state.step < state.totalTicks) {
        requestAnimationFrame(frame);
        return;
      }
      fitView();
      renderPositions();
      applyFilter();
    }
    requestAnimationFrame(frame);
  }

  function applyView() {
    svg.setAttribute('viewBox', state.view.x + ' ' + state.view.y + ' ' + state.view.w + ' ' + state.view.h);
  }

  function fitView() {
    if (!state.nodes.length) {
      state.view = { x: 0, y: 0, w: W, h: H };
      applyView();
      return;
    }
    var minX = Infinity;
    var minY = Infinity;
    var maxX = -Infinity;
    var maxY = -Infinity;
    state.nodes.forEach(function (n) {
      var r = nodeRadius(n) + 28;
      minX = Math.min(minX, n.x - r);
      minY = Math.min(minY, n.y - r);
      maxX = Math.max(maxX, n.x + r);
      maxY = Math.max(maxY, n.y + r);
    });
    var pad = 20;
    minX -= pad;
    minY -= pad;
    maxX += pad;
    maxY += pad;
    state.view = {
      x: minX,
      y: minY,
      w: Math.max(240, maxX - minX),
      h: Math.max(180, maxY - minY)
    };
    applyView();
  }

  function matches(n) {
    if (kind.value === 'page' && (n.kind !== 'page' || n.stub)) return false;
    if (kind.value === 'tag' && n.kind !== 'tag') return false;
    if (kind.value === 'stub' && !n.stub) return false;
    var q = filter.value.trim().toLowerCase();
    if (!q) return true;
    var hay = (n.title + ' ' + n.id + ' ' + n.section + ' ' + n.tags.join(' ') + ' ' + n.site).toLowerCase();
    return hay.indexOf(q) !== -1;
  }

  function applyFilter() {
    var shown = 0;
    state.nodes.forEach(function (n) {
      var ok = matches(n);
      if (ok) shown += 1;
      if (n.el) n.el.classList.toggle('is-dim', !ok);
    });
    state.edgeElements.forEach(function (item) {
      var ok = matches(item.edge.from) && matches(item.edge.to);
      item.el.classList.toggle('is-dim', !ok);
    });
    placeLabels();
    ensureTabbable();
    var total = state.nodes.length;
    var text = 'Showing ' + shown + ' of ' + total + (total === 1 ? ' node' : ' nodes') +
      '. Scroll to zoom, drag the background to pan, or select Fit view.';
    setStatus(text);
    var desc = document.getElementById('graphDesc');
    if (desc) desc.textContent = 'Pages and tags as nodes. ' + text +
      ' Use the arrow keys to move between nodes and Enter for details.';
  }

  function applyLabels() {
    app.classList.toggle('hide-labels', !labels.checked);
    placeLabels();
  }

  // Labels are placed greedily, the busiest nodes first. A label that would
  // overlap one already placed is hidden rather than drawn on top of it; the
  // node still reveals its name on hover or selection, and the panel has the
  // full title. This keeps a dense graph legible without dropping any node.
  function placeLabels() {
    var order = state.nodes.slice().sort(function (a, b) {
      return b.degree - a.degree || a.title.localeCompare(b.title);
    });
    var boxes = [];
    order.forEach(function (n) {
      var label = n.el && n.el.querySelector('.graph-label');
      if (!label) return;
      if (!labels.checked || !matches(n)) {
        label.classList.add('is-colliding');
        return;
      }
      var text = label.textContent || '';
      var w = Math.max(18, text.length * 5.6);
      var h = 13;
      var r = nodeRadius(n);
      var rect = {
        x: n.x - w / 2 - 2,
        y: n.y - r - 5 - h,
        w: w + 4,
        h: h + 2
      };
      var collides = false;
      for (var i = 0; i < boxes.length; i++) {
        var b = boxes[i];
        if (rect.x < b.x + b.w && rect.x + rect.w > b.x &&
            rect.y < b.y + b.h && rect.y + rect.h > b.y) {
          collides = true;
          break;
        }
      }
      label.classList.toggle('is-colliding', collides);
      if (!collides) boxes.push(rect);
    });
  }

  function setTabbable(node) {
    state.tabbable = node;
    state.nodes.forEach(function (n) {
      if (n.circle) n.circle.setAttribute('tabindex', n === node ? '0' : '-1');
    });
  }

  // Roving tabindex: one node is in the tab order at a time, and the arrow keys
  // move between the nodes a filter leaves visible. Enter or Space opens the
  // details, so a large graph is one tab stop rather than one per page.
  function ensureTabbable() {
    var current = state.tabbable;
    if (current && current.el && !current.el.classList.contains('is-dim')) return;
    for (var i = 0; i < state.nodes.length; i++) {
      var n = state.nodes[i];
      if (n.el && !n.el.classList.contains('is-dim')) {
        setTabbable(n);
        return;
      }
    }
    state.tabbable = null;
    state.nodes.forEach(function (n) {
      if (n.circle) n.circle.setAttribute('tabindex', '-1');
    });
  }

  function focusStep(from, step) {
    var total = state.nodes.length;
    var idx = from.index;
    for (var i = 0; i < total; i++) {
      idx = (idx + step + total) % total;
      var candidate = state.nodes[idx];
      if (candidate.el && !candidate.el.classList.contains('is-dim')) {
        setTabbable(candidate);
        candidate.circle.focus();
        return;
      }
    }
  }

  function focusEdge(last) {
    var visible = state.nodes.filter(function (n) {
      return n.el && !n.el.classList.contains('is-dim');
    });
    if (!visible.length) return;
    var node = last ? visible[visible.length - 1] : visible[0];
    setTabbable(node);
    node.circle.focus();
  }

  function select(n) {
    state.selected = n;
    setTabbable(n);
    state.nodes.forEach(function (m) {
      if (m.el) m.el.classList.toggle('is-selected', m === n);
    });

    details.textContent = '';

    var heading = document.createElement('p');
    heading.className = 'graph-details-kind';
    heading.textContent = n.kind === 'tag' ? 'Tag hub' : n.stub ? 'Missing page' : (n.section ? 'Page / ' + n.section : 'Page');
    details.appendChild(heading);

    var title = document.createElement('h2');
    title.textContent = n.title;
    details.appendChild(title);

    var incoming = 0;
    var outgoing = 0;
    state.edges.forEach(function (e) {
      if (e.to === n) incoming += 1;
      if (e.from === n) outgoing += 1;
    });
    var meta = document.createElement('p');
    meta.className = 'graph-details-meta';
    var bits = [incoming + ' incoming', outgoing + ' outgoing'];
    if (n.site) bits.push(n.site);
    meta.textContent = bits.join(' / ');
    details.appendChild(meta);

    if (n.tags.length) {
      var row = document.createElement('p');
      row.className = 'graph-details-tags';
      n.tags.forEach(function (t) {
        var chip = document.createElement('span');
        chip.className = 'graph-tag';
        chip.textContent = '#' + t;
        row.appendChild(chip);
      });
      details.appendChild(row);
    }

    if (n.stub) {
      var note = document.createElement('p');
      note.textContent = 'This page is linked from the bundle but does not exist in it.';
      details.appendChild(note);
    }

    if (n.url) {
      var open = document.createElement('a');
      open.className = 'graph-open';
      open.href = n.url;
      open.textContent = n.kind === 'tag' ? 'Open tag page' : 'Open page';
      details.appendChild(open);
    }
    details.hidden = false;
  }

  function clearSelection() {
    state.selected = null;
    state.nodes.forEach(function (n) {
      if (n.el) n.el.classList.remove('is-selected');
    });
    details.hidden = true;
    details.textContent = '';
  }

  function toSvg(clientX, clientY) {
    var pt = svg.createSVGPoint();
    pt.x = clientX;
    pt.y = clientY;
    var ctm = svg.getScreenCTM();
    if (!ctm) return { x: 0, y: 0 };
    return pt.matrixTransform(ctm.inverse());
  }

  function startDrag(ev, n) {
    if (ev.button !== 0) return;
    ev.stopPropagation();
    ev.preventDefault();
    state.dragging = n;
    n.fixed = true;
    n.circle.setPointerCapture(ev.pointerId);
    svg.classList.add('is-dragging');
    select(n);
  }

  filter.addEventListener('input', applyFilter);
  kind.addEventListener('change', applyFilter);
  labels.addEventListener('change', applyLabels);
  fit.addEventListener('click', fitView);

  download.addEventListener('click', function () {
    if (!state.raw) return;
    var blob = new Blob([state.raw], { type: 'application/json' });
    var url = URL.createObjectURL(blob);
    var a = document.createElement('a');
    a.href = url;
    a.download = 'graph.json';
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
  });

  svg.addEventListener('click', function (ev) {
    if (swallowClick) {
      swallowClick = false;
      return;
    }
    if (ev.target === svg) clearSelection();
  });

  svg.addEventListener('dblclick', function (ev) {
    if (ev.target.closest && ev.target.closest('.graph-node-group')) return;
    fitView();
  });

  svg.addEventListener('wheel', function (ev) {
    ev.preventDefault();
    var v = state.view;
    var factor = Math.exp(ev.deltaY * 0.0015);
    var nextW = Math.min(4000, Math.max(180, v.w * factor));
    var nextH = nextW * (v.h / v.w);
    var pt = toSvg(ev.clientX, ev.clientY);
    v.x = pt.x - (pt.x - v.x) * (nextW / v.w);
    v.y = pt.y - (pt.y - v.y) * (nextH / v.h);
    v.w = nextW;
    v.h = nextH;
    applyView();
  }, { passive: false });

  svg.addEventListener('pointerdown', function (ev) {
    if (ev.button !== 0) return;
    if (ev.target.closest && ev.target.closest('.graph-node-group')) return;
    // A background press starts a new interaction, so any pending click from a
    // node that never produced one must not be swallowed.
    swallowClick = false;
    var ctm = svg.getScreenCTM();
    state.panning = {
      x: ev.clientX,
      y: ev.clientY,
      vx: state.view.x,
      vy: state.view.y,
      sx: ctm ? 1 / ctm.a : state.view.w / (svg.clientWidth || 1),
      sy: ctm ? 1 / ctm.d : state.view.h / (svg.clientHeight || 1),
      moved: false
    };
    svg.setPointerCapture(ev.pointerId);
    svg.classList.add('is-panning');
  });

  svg.addEventListener('pointermove', function (ev) {
    if (state.panning) {
      if (Math.abs(ev.clientX - state.panning.x) > 3 || Math.abs(ev.clientY - state.panning.y) > 3) {
        state.panning.moved = true;
      }
      state.view.x = state.panning.vx - (ev.clientX - state.panning.x) * state.panning.sx;
      state.view.y = state.panning.vy - (ev.clientY - state.panning.y) * state.panning.sy;
      applyView();
      return;
    }
    if (state.dragging) {
      var pt = toSvg(ev.clientX, ev.clientY);
      state.dragging.x = pt.x;
      state.dragging.y = pt.y;
      renderPositions();
    }
  });

  function endPointer(ev) {
    if (state.panning) {
      var panned = state.panning.moved;
      state.panning = null;
      svg.classList.remove('is-panning');
      if (svg.hasPointerCapture(ev.pointerId)) svg.releasePointerCapture(ev.pointerId);
      if (panned) swallowClick = true;
    }
    if (state.dragging) {
      state.dragging.fixed = false;
      state.dragging = null;
      svg.classList.remove('is-dragging');
      swallowClick = true;
    }
    if (state.nodes.length) placeLabels();
  }
  svg.addEventListener('pointerup', endPointer);
  svg.addEventListener('pointercancel', endPointer);

  document.addEventListener('keydown', function (ev) {
    if (ev.key === 'Escape' && state.selected) clearSelection();
  });

  load();
})();
