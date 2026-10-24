(function () {
  'use strict';

  var app = document.getElementById('graphApp');
  if (!app) return;

  var NS = 'http://www.w3.org/2000/svg';
  var W = 1000;
  var H = 700;
  // Below this text size an edge is a grey wash rather than a structure.
  var EDGE_MIN_TEXT_PX = 4;
  var LABEL_LIMIT = 50;
  var LOCAL_VIEW_NODES = 250;
  // A first page keeps the HTML list responsive and its one roving tab stop
  // useful on a large catalogue. Show all is an explicit choice.
  var LIST_NODE_LIMIT = 200;
  var LIST_EDGE_LIMIT = 300;
  // Screen pixels per world unit for the first view and for a focused
  // neighbourhood. Names are drawn at 11 screen pixels, so these set how much
  // of the graph fits on screen at a readable size.
  var READABLE_SCALE = 0.9;
  var FOCUS_SCALE = 0.45;
  // Below this screen scale a plain page keeps its name back and only hubs
  // label the map. Zooming in brings the page names in.
  var PAGE_LABEL_SCALE = 0.9;

  var svg = document.getElementById('graphSvg');
  var sectionLayer = document.getElementById('graphSectionLayer');
  var edgeLayer = document.getElementById('graphEdges');
  var nodeLayer = document.getElementById('graphNodes');
  var status = document.getElementById('graphStatus');
  var filter = document.getElementById('graphFilter');
  var kind = document.getElementById('graphKind');
  var sectionsCheck = document.getElementById('graphSections');
  var tagHubsCheck = document.getElementById('graphTagHubs');
  var labels = document.getElementById('graphLabels');
  var tagsDetails = document.getElementById('graphTags');
  var tagsSummary = document.getElementById('graphTagsSummary');
  var tagsMenu = document.getElementById('graphTagsMenu');
  var listStatus = document.getElementById('graphListStatus');
  var listNodes = document.getElementById('graphListNodes');
  var listEdges = document.getElementById('graphListEdges');
  var listNodesTitle = document.getElementById('graphListNodesTitle');
  var listEdgesTitle = document.getElementById('graphListEdgesTitle');
  var fit = document.getElementById('graphFit');
  var clearFocusLink = document.getElementById('graphClearFocus');
  var download = document.getElementById('graphDownload');
  var details = document.getElementById('graphDetails');

  var base = app.getAttribute('data-graph-base') || '/';
  if (base.charAt(base.length - 1) !== '/') base += '/';

  var state = {
    allNodes: [],
    allEdges: [],
    nodes: [],
    edges: [],
    edgeElements: [],
    raw: '',
    selected: null,
    tabbable: null,
    adj: Object.create(null),
    focus: null,
    focusCount: 0,
    focusHops: 0,
    pendingFocus: null,
    focusMissing: false,
    selectedTags: [],
    tagBySlug: Object.create(null),
    nodeByID: Object.create(null),
    showTagHubs: true,
    clusterSections: true,
    sectionCells: null,
    clusterLabels: Object.create(null),
    legacySections: false,
    layoutGeneration: 0,
    layoutNudges: 0,
    listAllNodes: false,
    listAllEdges: false,
    anchor: null,
    hubIDs: Object.create(null),
    overview: false,
    query: '',
    labelScale: 0,
    cullView: null,
    dragMoved: false,
    dragFrom: null,
    scale: 1,
    labelFrame: 0,
    widths: Object.create(null),
    view: { x: 0, y: 0, w: W, h: H },
    step: 0,
    totalTicks: 0,
    dragging: null,
    panning: null
  };

  // The force layout's only random choice is the tiny nudge when two nodes
  // land on the same point. A content seed makes that choice repeatable
  // without changing the layout's shape.
  var layoutRandom = seededRandom(1);

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

  // The graph builder slugifies tags with the same alphabet as page slugs: a
  // lower-case word, with spaces, hyphens and underscores folded to hyphens.
  // The URL carries the slug so a tag with two spellings still has one filter.
  function tagSlug(value) {
    return String(value).toLowerCase()
      .replace(/[ _]/g, '-')
      .replace(/[^a-z0-9-]/g, '')
      .replace(/-+/g, '-')
      .replace(/^-|-$/g, '');
  }

  function uniqueTagSlugs(values) {
    var out = [];
    values.forEach(function (value) {
      var slug = tagSlug(value);
      if (slug && out.indexOf(slug) === -1) out.push(slug);
    });
    return out;
  }

  function tagSlugOfNode(n) {
    if (!n || n.kind !== 'tag') return '';
    var at = n.id.lastIndexOf('tag:');
    return at === -1 ? '' : n.id.slice(at + 4);
  }

  function tagLabel(slug) {
    var tag = state.tagBySlug[slug];
    return tag ? tag.title : '#' + slug;
  }

  function initTagFilter() {
    state.tagBySlug = Object.create(null);
    state.allNodes.forEach(function (n) {
      if (n.kind !== 'tag') return;
      var slug = tagSlugOfNode(n);
      if (!slug) return;
      if (!state.tagBySlug[slug]) {
        state.tagBySlug[slug] = { slug: slug, title: n.title, count: 0 };
      }
    });
    state.nodes.forEach(function (n) {
      if (n.kind !== 'page' || n.stub) return;
      n.tagSlugs.forEach(function (slug) {
        if (state.tagBySlug[slug]) state.tagBySlug[slug].count += 1;
      });
    });
    renderTagMenu();
    updateTagSummary();
  }

  function renderTagMenu() {
    if (!tagsMenu) return;
    tagsMenu.textContent = '';
    var slugs = Object.keys(state.tagBySlug).sort(function (a, b) {
      return tagLabel(a).localeCompare(tagLabel(b));
    });
    var missing = state.selectedTags.filter(function (slug) {
      return !state.tagBySlug[slug];
    }).sort();
    if (!slugs.length && !missing.length) {
      var empty = document.createElement('p');
      empty.className = 'graph-tags-empty';
      empty.textContent = 'No tags in this bundle.';
      tagsMenu.appendChild(empty);
      return;
    }
    var all = slugs.concat(missing);
    all.sort(function (a, b) {
      var aSelected = state.selectedTags.indexOf(a) !== -1;
      var bSelected = state.selectedTags.indexOf(b) !== -1;
      if (aSelected !== bSelected) return aSelected ? -1 : 1;
      return tagLabel(a).localeCompare(tagLabel(b));
    });
    all.forEach(function (slug) {
      var tag = state.tagBySlug[slug];
      var label = document.createElement('label');
      label.className = 'graph-tag-option' + (tag ? '' : ' is-missing');
      var box = document.createElement('input');
      box.type = 'checkbox';
      box.value = slug;
      box.checked = state.selectedTags.indexOf(slug) !== -1;
      var text = document.createElement('span');
      text.textContent = tag ? tag.title : '#' + slug;
      var count = document.createElement('small');
      count.textContent = tag ? String(tag.count) : 'not in graph';
      label.appendChild(box);
      label.appendChild(text);
      label.appendChild(count);
      tagsMenu.appendChild(label);
    });
    if (tagsDetails && tagsDetails.open) positionTagsMenu();
  }

  function updateTagSummary() {
    if (!tagsSummary) return;
    var count = state.selectedTags.length;
    if (!count) {
      tagsSummary.textContent = 'Tags: any';
      tagsSummary.removeAttribute('title');
      return;
    }
    if (count === 1) {
      tagsSummary.textContent = 'Tags: ' + tagLabel(state.selectedTags[0]);
    } else {
      tagsSummary.textContent = 'Tags: ' + count + ' selected';
    }
    tagsSummary.title = state.selectedTags.map(tagLabel).join(', ');
  }

  // The list is the non-visual path through the same visible set. It filters
  // by focus as well as by the controls, rebuilds only when that set changes,
  // and keeps one roving tab stop among its node buttons.
  function renderList() {
    if (!listNodes || !listEdges) return;
    var active = document.activeElement;
    var activeInList = !!active && ((listNodes && listNodes.contains(active)) ||
      (listEdges && listEdges.contains(active)));
    var activeID = active && active.classList && active.classList.contains('graph-list-node')
      ? active.getAttribute('data-id') : '';

    var allNodes = state.nodes.filter(listVisible).slice();
    allNodes.sort(function (a, b) {
      var byKind = listRank(a) - listRank(b);
      if (byKind) return byKind;
      var byTitle = a.title.localeCompare(b.title);
      return byTitle || a.id.localeCompare(b.id);
    });
    var allEdges = state.edges.filter(function (edge) {
      return listVisible(edge.from) && listVisible(edge.to);
    }).slice();
    allEdges.sort(function (a, b) {
      var byFrom = a.from.title.localeCompare(b.from.title);
      if (byFrom) return byFrom;
      var byTo = a.to.title.localeCompare(b.to.title);
      if (byTo) return byTo;
      return a.kind.localeCompare(b.kind);
    });

    var nodes = state.listAllNodes ? allNodes : allNodes.slice(0, LIST_NODE_LIMIT);
    if (!state.listAllNodes && state.selected &&
        allNodes.indexOf(state.selected) !== -1 && nodes.indexOf(state.selected) === -1) {
      nodes = nodes.slice(0, LIST_NODE_LIMIT - 1);
      nodes.push(state.selected);
    }
    var edges = state.listAllEdges ? allEdges : allEdges.slice(0, LIST_EDGE_LIMIT);
    if (listStatus) {
      var parts = [];
      if (nodes.length !== allNodes.length) {
        parts.push(nodes.length + ' of ' + allNodes.length + ' nodes');
      } else {
        parts.push(allNodes.length + (allNodes.length === 1 ? ' node' : ' nodes'));
      }
      if (edges.length !== allEdges.length) {
        parts.push(edges.length + ' of ' + allEdges.length + ' edges');
      } else {
        parts.push(allEdges.length + (allEdges.length === 1 ? ' edge' : ' edges'));
      }
      var capped = nodes.length !== allNodes.length || edges.length !== allEdges.length;
      listStatus.textContent = (capped ? 'Showing ' : '') + parts.join(', ') + '.';
    }
    if (listNodesTitle) listNodesTitle.textContent = 'Nodes (' + allNodes.length + ')';
    if (listEdgesTitle) listEdgesTitle.textContent = 'Edges (' + allEdges.length + ')';

    listNodes.textContent = '';
    if (!allNodes.length) {
      listNodes.appendChild(listEmpty('nodes'));
    }
    nodes.forEach(function (n) {
      var links = connections(n);
      var item = document.createElement('li');
      item.className = 'graph-list-item';
      var button = document.createElement('button');
      button.type = 'button';
      button.className = 'graph-list-node';
      button.setAttribute('data-id', n.id);
      button.setAttribute('aria-label', 'Select ' + n.title + ', ' + listKind(n));
      button.textContent = n.title;
      if (state.selected === n) {
        item.classList.add('is-selected');
        button.setAttribute('aria-current', 'true');
      }
      button.addEventListener('click', function () { select(n, true); });
      item.appendChild(button);

      var meta = document.createElement('p');
      meta.className = 'graph-list-meta';
      var kind = document.createElement('span');
      kind.textContent = listKind(n);
      var counts = document.createElement('span');
      counts.textContent = links.incoming + ' in / ' + links.outgoing + ' out';
      meta.appendChild(kind);
      meta.appendChild(counts);
      if (n.tags.length) {
        var tags = document.createElement('span');
        tags.className = 'graph-list-tags';
        tags.textContent = n.tags.map(function (tag) { return '#' + tag; }).join(' ');
        meta.appendChild(tags);
      }
      item.appendChild(meta);
      listNodes.appendChild(item);
    });
    if (nodes.length < allNodes.length) {
      var nodeMore = document.createElement('li');
      nodeMore.className = 'graph-list-more-row';
      var nodeButton = document.createElement('button');
      nodeButton.type = 'button';
      nodeButton.className = 'graph-list-more';
      nodeButton.textContent = 'Show all ' + allNodes.length + ' nodes';
      nodeButton.addEventListener('click', function () {
        state.listAllNodes = true;
        renderList();
        var shown = listNodes.querySelectorAll('.graph-list-node');
        if (shown[nodes.length]) {
          Array.prototype.forEach.call(shown, function (button) { button.tabIndex = -1; });
          shown[nodes.length].tabIndex = 0;
          shown[nodes.length].focus();
        }
      });
      nodeMore.appendChild(nodeButton);
      listNodes.appendChild(nodeMore);
    }

    listEdges.textContent = '';
    if (!allEdges.length) {
      listEdges.appendChild(listEmpty('edges'));
    }
    edges.forEach(function (edge) {
      var item = document.createElement('li');
      item.className = 'graph-list-edge';
      var cross = edge.from.siteKey && edge.to.siteKey &&
        edge.from.siteKey !== edge.to.siteKey;
      if (cross) item.classList.add('is-cross-site');
      var from = document.createElement('span');
      from.className = 'graph-list-edge-node';
      from.textContent = edge.from.title;
      var action = document.createElement('span');
      action.className = 'graph-list-edge-action';
      action.textContent = edge.kind === 'tag' ? 'tagged' : 'links to';
      var to = document.createElement('span');
      to.className = 'graph-list-edge-node';
      to.textContent = edge.to.title;
      var type = document.createElement('span');
      type.className = 'graph-list-edge-kind';
      type.textContent = cross ? 'cross-wiki' : edge.kind;
      item.appendChild(from);
      item.appendChild(action);
      item.appendChild(to);
      item.appendChild(type);
      if (edge.anchor) {
        var anchor = document.createElement('span');
        anchor.className = 'graph-list-edge-anchor';
        anchor.textContent = '#' + edge.anchor;
        item.appendChild(anchor);
      }
      if (edge.text) {
        var text = document.createElement('span');
        text.className = 'graph-list-edge-text';
        text.textContent = '\u201c' + edge.text + '\u201d';
        item.appendChild(text);
      }
      listEdges.appendChild(item);
    });
    if (edges.length < allEdges.length) {
      var edgeMore = document.createElement('li');
      edgeMore.className = 'graph-list-more-row';
      var edgeButton = document.createElement('button');
      edgeButton.type = 'button';
      edgeButton.className = 'graph-list-more';
      edgeButton.textContent = 'Show all ' + allEdges.length + ' edges';
      edgeButton.addEventListener('click', function () {
        state.listAllEdges = true;
        renderList();
        if (listEdgesTitle) {
          listEdgesTitle.tabIndex = -1;
          listEdgesTitle.focus();
        }
      });
      edgeMore.appendChild(edgeButton);
      listEdges.appendChild(edgeMore);
    }

    var target = syncListTabbable(activeID);
    if (activeInList) {
      if (!target) target = listNodes.querySelector('.graph-list-node');
      if (!target && listNodesTitle) {
        listNodesTitle.tabIndex = -1;
        target = listNodesTitle;
      }
      if (target) target.focus();
    }
  }

  function listVisible(n) {
    return !!n.matched && (!state.focus || state.focus[n.id] != null);
  }

  function listEmpty(what) {
    var hasFilters = !!state.query || kind.value !== 'all' || state.selectedTags.length > 0;
    var item = document.createElement('li');
    item.className = 'graph-list-empty';
    if (state.focus) {
      item.textContent = hasFilters
        ? 'No ' + what + ' in this focus match the filters.'
        : 'No ' + what + ' are in this focus.';
    } else if (hasFilters) {
      item.textContent = 'No ' + what + ' match the current filters.';
    } else {
      item.textContent = 'This graph has no ' + what + '.';
    }
    return item;
  }

  function listRank(n) {
    if (n.kind === 'tag') return 1;
    return n.stub ? 2 : 0;
  }

  function listKind(n) {
    if (n.kind === 'tag') return 'Tag hub';
    if (n.stub) return 'Missing page';
    return n.section ? 'Page / ' + n.section : 'Page';
  }

  function listButtons() {
    return Array.prototype.slice.call(listNodes.querySelectorAll('.graph-list-node'));
  }

  function syncListTabbable(preferredID) {
    if (!listNodes) return null;
    var buttons = listButtons();
    if (!buttons.length) return null;
    var chosen = null;
    if (preferredID) {
      chosen = buttons.filter(function (button) {
        return button.getAttribute('data-id') === preferredID;
      })[0] || null;
    }
    if (!chosen && state.selected) {
      chosen = buttons.filter(function (button) {
        return button.getAttribute('data-id') === state.selected.id;
      })[0] || null;
    }
    if (!chosen) chosen = buttons[0];
    buttons.forEach(function (button) {
      button.tabIndex = button === chosen ? 0 : -1;
    });
    return chosen;
  }

  function updateListSelection(n) {
    if (!listNodes) return;
    Array.prototype.forEach.call(listNodes.querySelectorAll('.graph-list-node'), function (button) {
      var selected = !!n && button.getAttribute('data-id') === n.id;
      if (button.parentNode) button.parentNode.classList.toggle('is-selected', selected);
      if (selected) button.setAttribute('aria-current', 'true');
      else button.removeAttribute('aria-current');
    });
    var active = document.activeElement;
    var preferred = active && active.classList && active.classList.contains('graph-list-node')
      ? active.getAttribute('data-id') : '';
    syncListTabbable(preferred);
  }

  function showListUnavailable(message) {
    if (listStatus) listStatus.textContent = message;
    if (listNodes) listNodes.textContent = '';
    if (listEdges) listEdges.textContent = '';
    if (listNodesTitle) listNodesTitle.textContent = 'Nodes';
    if (listEdgesTitle) listEdgesTitle.textContent = 'Edges';
  }

  // Keep the menu inside the viewport. The CSS anchors it left; this shifts it
  // only when its right edge would cross the viewport padding.
  function positionTagsMenu() {
    if (!tagsDetails || !tagsMenu || !tagsDetails.open) return;
    tagsMenu.style.left = '';
    tagsMenu.style.right = '';
    var parent = tagsDetails.getBoundingClientRect();
    var box = tagsMenu.getBoundingClientRect();
    var margin = 16;
    var viewport = document.documentElement.clientWidth || window.innerWidth;
    var left = parent.left;
    if (left + box.width > viewport - margin) left = viewport - margin - box.width;
    if (left < margin) left = margin;
    tagsMenu.style.left = (left - parent.left) + 'px';
    tagsMenu.style.right = 'auto';
  }

  // readURLState applies the shareable dimensions to the controls. Focus is
  // returned for the caller because the first load has to wait for layout,
  // while a popstate can apply it to the finished drawing immediately.
  function readURLState(params) {
    var k = params.get('kind');
    if (!state.showTagHubs && k === 'tag') k = 'all';
    kind.value = (k === 'page' || k === 'tag' || k === 'stub') ? k : 'all';
    filter.value = params.get('q') || '';
    var wanted = params.getAll('tag').map(function (value) {
      return tagSlug(String(value).trim());
    }).filter(function (value) { return !!value; });
    var selected = [];
    wanted.forEach(function (slug) {
      if (selected.indexOf(slug) === -1) selected.push(slug);
    });
    state.selectedTags = selected;
    renderTagMenu();
    updateTagSummary();

    var id = params.get('focus');
    if (!id) return null;
    var node = state.nodeByID[id];
    if (!node || state.adj[id] === undefined) return { missing: true };
    var hops = parseInt(params.get('depth') || '1', 10);
    hops = Math.max(1, Math.min(3, isNaN(hops) ? 1 : hops));
    return { node: node, hops: hops };
  }

  function syncURL(mode) {
    var url = new URL(window.location.href);
    var params = url.searchParams;
    params.delete('focus');
    params.delete('depth');
    params.delete('kind');
    params.delete('q');
    params.delete('tag');
    var focus = null;
    if (state.selected && state.focus) {
      focus = { id: state.selected.id, hops: state.focusHops || 1 };
    } else if (state.pendingFocus) {
      focus = { id: state.pendingFocus.node.id, hops: state.pendingFocus.hops };
    }
    if (focus) {
      params.set('focus', focus.id);
      params.set('depth', String(focus.hops));
    }
    if (kind.value !== 'all') params.set('kind', kind.value);
    var q = filter.value.trim();
    if (q) params.set('q', q);
    state.selectedTags.slice().sort().forEach(function (slug) {
      params.append('tag', slug);
    });
    var next = url.pathname + url.search + url.hash;
    var current = window.location.pathname + window.location.search + window.location.hash;
    if (next === current) return;
    try {
      if (mode === 'push') window.history.pushState(null, '', next);
      else window.history.replaceState(null, '', next);
    } catch (e) {
      // file:// history is available in current Firefox and Chrome. A browser
      // that refuses it still keeps the controls and the drawing usable.
    }
  }

  function applyURLState() {
    state.pendingFocus = null;
    var next = readURLState(new URLSearchParams(window.location.search));
    state.focusMissing = !!(next && next.missing);
    applyFilter();
    if (next && next.node) {
      select(next.node, true, next.hops, true);
    } else {
      clearSelection(true);
    }
  }

  function inlineCatalogue() {
    var el = document.getElementById('graphData');
    return el ? (el.textContent || '').trim() : '';
  }

  function load() {
    var text = inlineCatalogue();
    if (text) {
      start(text);
      return;
    }
    setStatus('Loading the graph...');
    fetch(base + 'graph.json', { headers: { Accept: 'application/json' } })
      .then(function (res) {
        if (!res.ok) throw new Error('HTTP ' + res.status);
        return res.text();
      })
      .then(function (raw) { start(raw); })
      .catch(function (err) {
        var detail = err && err.message ? err.message : 'the request failed';
        setStatus('The graph could not be loaded (' + detail + '). The raw file is at ' + base + 'graph.json.', true);
        showListUnavailable('The list is unavailable because the graph could not be loaded.');
      });
  }

  function start(raw) {
    var data;
    try {
      data = JSON.parse(raw);
    } catch (e) {
      setStatus('The graph catalogue is not valid JSON.', true);
      showListUnavailable('The list is unavailable because the graph is not valid JSON.');
      return;
    }
    if (!data || data.version !== 1 || !Array.isArray(data.nodes) || !Array.isArray(data.edges)) {
      setStatus('The graph catalogue has a shape this view does not know (expected version 1).', true);
      showListUnavailable('The list is unavailable because the graph has an unknown shape.');
      return;
    }

    state.raw = raw;
    state.allNodes = data.nodes.map(function (n, i) {
      var nodeTags = Array.isArray(n.tags) ? n.tags.map(String) : [];
      return {
        id: String(n.id || ''),
        title: String(n.title || n.id || ''),
        url: n.url ? String(n.url) : '',
        kind: n.kind === 'tag' ? 'tag' : 'page',
        section: n.section ? String(n.section) : '',
        sectionID: n.sectionId ? String(n.sectionId) : '',
        tags: nodeTags,
        tagSlugs: uniqueTagSlugs(nodeTags),
        stub: !!n.stub,
        site: n.site ? String(n.site) : '',
        siteKey: n.siteKey ? String(n.siteKey) : '',
        matched: true,
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

    state.legacySections = !state.allNodes.some(function (n) {
      return n.kind === 'page' && n.sectionID;
    });
    var byId = Object.create(null);
    state.allNodes.forEach(function (n) { byId[n.id] = n; });
    state.nodeByID = byId;
    state.allEdges = [];
    data.edges.forEach(function (e) {
      var from = byId[e.from];
      var to = byId[e.to];
      if (!from || !to) return;
      state.allEdges.push({
        from: from,
        to: to,
        kind: e.kind === 'tag' ? 'tag' : 'link',
        anchor: e.anchor ? String(e.anchor) : '',
        text: e.text ? String(e.text) : ''
      });
    });

    state.showTagHubs = tagHubsCheck ? tagHubsCheck.checked : true;
    state.clusterSections = sectionsCheck ? sectionsCheck.checked : true;
    buildVisibleGraph();

    if (!state.nodes.length) {
      setStatus('The catalogue is empty. Add pages to the bundle and render again.');
      renderList();
      return;
    }

    initTagFilter();
    updateTagKindOption();
    var initial = readURLState(new URLSearchParams(window.location.search));
    if (initial && initial.node) {
      state.pendingFocus = { node: initial.node, hops: initial.hops };
    } else if (initial && initial.missing) {
      state.focusMissing = true;
    }
    state.anchor = starterPage();

    download.disabled = false;
    sizeWorld();
    layoutInitial();
    draw();
    simulateThenRender();
  }

  // The drawing and the list share one visible graph. The complete catalogue
  // stays in allNodes/allEdges so collapsing tag hubs is a view change, not a
  // loss of the tag filter or of the data.
  function buildVisibleGraph() {
    var visible = Object.create(null);
    state.nodes = state.allNodes.filter(function (n) {
      return state.showTagHubs || n.kind !== 'tag';
    });
    state.nodes.forEach(function (n) {
      n.degree = 0;
      visible[n.id] = true;
    });
    state.edges = state.allEdges.filter(function (e) {
      return visible[e.from.id] && visible[e.to.id];
    });
    state.edges.forEach(function (e) {
      e.from.degree += 1;
      e.to.degree += 1;
    });
    state.hubIDs = pageHubs();
    state.adj = Object.create(null);
    state.nodes.forEach(function (n) { state.adj[n.id] = []; });
    state.edges.forEach(function (e) {
      state.adj[e.from.id].push({ node: e.to, dir: 'out' });
      state.adj[e.to.id].push({ node: e.from, dir: 'in' });
    });
  }

  // A page is a hub when it sits in the busiest quarter of the connected
  // pages. Tags are always hubs. Ranking rather than a degree cutoff keeps
  // ties from turning every leaf page into a hub. The set follows the visible
  // graph, so hiding tag hubs does not leave page labels pointing at degrees
  // from a graph that is no longer on screen.
  function pageHubs() {
    var pages = state.nodes.filter(function (n) {
      return n.kind === 'page' && n.degree > 0;
    }).sort(function (a, b) {
      return b.degree - a.degree || a.title.localeCompare(b.title) ||
        a.id.localeCompare(b.id);
    });
    var hubs = Object.create(null);
    var limit = Math.ceil(pages.length * 0.25);
    for (var i = 0; i < limit; i++) hubs[pages[i].id] = true;
    return hubs;
  }

  function updateTagKindOption() {
    var option = kind ? kind.querySelector('option[value="tag"]') : null;
    if (option) option.disabled = !state.showTagHubs;
    if (!state.showTagHubs && kind && kind.value === 'tag') kind.value = 'all';
  }

  // A layout toggle changes the visible graph. It clears the selection because
  // the old focus or tag hub may no longer be on the canvas, then rebuilds and
  // runs the same render path as the first load.
  function relayoutGraph(rebuild) {
    state.selected = null;
    state.focus = null;
    state.focusCount = 0;
    state.focusHops = 0;
    state.focusMissing = false;
    state.pendingFocus = null;
    state.tabbable = null;
    details.hidden = true;
    details.textContent = '';
    if (rebuild) buildVisibleGraph();
    updateTagKindOption();
    state.anchor = starterPage();
    sizeWorld();
    layoutInitial();
    draw();
    applyFocus();
    simulateThenRender();
    updateListSelection(null);
    syncURL('push');
  }

  // The view opens on a page with a typical number of connections rather than
  // the busiest one: a hub's neighbourhood is the densest part of the drawing,
  // which is the opposite of a readable first screen.
  function starterPage() {
    var pages = state.nodes.filter(function (n) { return n.kind === 'page' && !n.stub; });
    if (!pages.length) pages = state.nodes;
    var degrees = pages.map(function (n) { return n.degree; }).sort(function (a, b) { return a - b; });
    var median = degrees[Math.floor(degrees.length / 2)];
    var cap = Math.max(4, median + 2);
    var best = null;
    pages.forEach(function (n) {
      if (n.degree > cap) return;
      if (!best || n.degree > best.degree) best = n;
    });
    return best;
  }

  // A fixed 1000 by 700 world is enough for a small bundle and far too small
  // for a dense one. Giving each node the area its spring length asks for is
  // what stops the layout from saturating against the walls.
  function targetArea(n) {
    // The area per node the circles and names need, including the gaps.
    return n * 110 * 110 / 0.7;
  }

  function sizeWorld() {
    var n = Math.max(1, state.nodes.length);
    W = Math.max(1000, Math.round(Math.sqrt(targetArea(n))));
    H = Math.max(700, Math.round(W * 0.7));
  }

  // The 1st and 99th percentile of a coordinate, so one stranded page cannot
  // decide the scale of the whole drawing.
  function percentileRange(values) {
    var sorted = values.slice().sort(function (a, b) { return a - b; });
    var lo = Math.floor((sorted.length - 1) * 0.01);
    var hi = Math.floor((sorted.length - 1) * 0.99);
    return { min: sorted[lo], max: sorted[hi] };
  }

  // A force layout settles where its springs and repulsion balance, which on a
  // densely linked bundle can be far tighter than the drawing was designed
  // around. Scaling the settled cloud to the area its spring length asks for
  // gives every layout the density its labels and circles expect, without
  // asking the simulation for thousands more ticks.
  function normaliseLayout() {
    if (state.nodes.length < 200 || !state.edges.length) return;
    if (state.clusterSections) return;
    var xr = percentileRange(state.nodes.map(function (n) { return n.x; }));
    var yr = percentileRange(state.nodes.map(function (n) { return n.y; }));
    var minX = xr.min;
    var maxX = xr.max;
    var minY = yr.min;
    var maxY = yr.max;
    var area = (maxX - minX) * (maxY - minY);
    if (!area) return;
    var target = targetArea(state.nodes.length);
    var factor = Math.max(0.25, Math.min(6, Math.sqrt(target / area)));
    if (Math.abs(factor - 1) < 0.05) return;

    var cx = (minX + maxX) / 2;
    var cy = (minY + maxY) / 2;
    state.nodes.forEach(function (n) {
      n.x = cx + (n.x - cx) * factor;
      n.y = cy + (n.y - cy) * factor;
    });

    // The percentiles decide the scale, but a stranded page still has a real
    // position, so the world is sized from the full extent after scaling.
    var realMinX = Infinity;
    var realMinY = Infinity;
    var realMaxX = -Infinity;
    var realMaxY = -Infinity;
    state.nodes.forEach(function (n) {
      realMinX = Math.min(realMinX, n.x);
      realMinY = Math.min(realMinY, n.y);
      realMaxX = Math.max(realMaxX, n.x);
      realMaxY = Math.max(realMaxY, n.y);
    });
    var shiftX = 60 - realMinX;
    var shiftY = 60 - realMinY;
    state.nodes.forEach(function (n) {
      n.x += shiftX;
      n.y += shiftY;
    });
    W = Math.max(W, Math.round(realMaxX - realMinX) + 120);
    H = Math.max(H, Math.round(realMaxY - realMinY) + 120);
  }

  function layoutSeed() {
    var hash = 2166136261;
    function add(text) {
      for (var i = 0; i < text.length; i++) {
        hash ^= text.charCodeAt(i);
        hash = Math.imul(hash, 16777619);
      }
    }
    add(state.clusterSections ? 'sections' : 'force');
    state.nodes.forEach(function (n) {
      add('\x00n');
      add(n.id);
    });
    state.edges.forEach(function (e) {
      add('\x00e');
      add(e.from.id);
      add('\x00');
      add(e.to.id);
      add('\x00');
      add(e.kind);
    });
    return hash >>> 0;
  }

  // mulberry32: small, deterministic and enough for the collision nudge.
  function seededRandom(seed) {
    var value = seed >>> 0;
    return function () {
      value = (value + 0x6D2B79F5) | 0;
      var t = Math.imul(value ^ (value >>> 15), 1 | value);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }

  function seedLayout() {
    layoutRandom = seededRandom(layoutSeed());
  }

  // Placing nodes by index leaves every linked pair at a random distance, so a
  // dense graph spends its whole simulation pulling edges across the world.
  // Walking the catalogue puts each page beside the page that links to it,
  // which leaves the forces a short distance to refine and keeps a
  // neighbourhood compact when it is selected later.
  function layoutInitialWalk() {
    var seen = Object.create(null);
    var queue = [];
    var start = state.anchor || state.nodes[0];
    seen[start.id] = true;
    start.x = W / 2;
    start.y = H / 2;
    start.vx = 0;
    start.vy = 0;
    queue.push(start);

    for (var qi = 0; qi < queue.length; qi++) {
      var current = queue[qi];
      var links = state.adj[current.id] || [];
      var children = [];
      for (var li = 0; li < links.length; li++) {
        var next = links[li].node;
        if (seen[next.id]) continue;
        seen[next.id] = true;
        children.push(next);
      }
      var k = children.length;
      if (!k) continue;
      var base = Math.atan2(current.y - H / 2, current.x - W / 2);
      if (!isFinite(base)) base = qi * 2.399963;
      var radius = 110 + Math.min(20, k) * 10;
      for (var j = 0; j < k; j++) {
        var a = base + (j - (k - 1) / 2) * (Math.PI * 2 / Math.max(6, k)) + (qi % 3) * 0.2;
        var child = children[j];
        child.x = current.x + Math.cos(a) * radius;
        child.y = current.y + Math.sin(a) * radius;
        child.vx = 0;
        child.vy = 0;
        queue.push(child);
      }
    }

    // Pages in a component the walk never reached still need a place to
    // start; the halo around the main cloud is enough for the forces to take
    // over.
    var golden = Math.PI * (3 - Math.sqrt(5));
    var left = state.nodes.filter(function (node) { return !seen[node.id]; });
    left.forEach(function (node, i) {
      var a = i * golden;
      var r = Math.min(W, H) * 0.25 * Math.sqrt((i + 0.5) / Math.max(1, left.length));
      node.x = W / 2 + Math.cos(a) * r;
      node.y = H / 2 + Math.sin(a) * r;
      node.vx = 0;
      node.vy = 0;
    });
  }

  // In section mode the grid is the layout. Each group occupies one cell and
  // its members sit in a small grid inside it, so sections never dissolve into
  // the link cloud. The link edges still show the connections between cells.
  function layoutInitialClustered() {
    var groups = clusterGroups();
    var names = Object.keys(groups).sort();
    var cells = Object.create(null);
    state.sectionCells = cells;
    var cols = names.length <= 1 ? 1 : Math.ceil(Math.sqrt(names.length * (W / H)));
    cols = Math.max(1, Math.min(names.length, cols));
    var rows = Math.ceil(names.length / Math.max(1, cols));
    var cellW = W / Math.max(1, cols);
    var cellH = H / Math.max(1, rows);
    names.forEach(function (name, i) {
      var cell = {
        x: (i % cols) * cellW,
        y: Math.floor(i / cols) * cellH,
        w: cellW,
        h: cellH
      };
      cells[name] = cell;
      var members = groups[name];
      var innerCols = Math.max(1, Math.min(members.length,
        Math.ceil(Math.sqrt(members.length * (cellW / cellH)))));
      var innerRows = Math.ceil(members.length / innerCols);
      var gapX = cellW / (innerCols + 1);
      var gapY = cellH / (innerRows + 1);
      members.forEach(function (n, j) {
        n.x = cell.x + gapX * ((j % innerCols) + 1);
        n.y = cell.y + gapY * (Math.floor(j / innerCols) + 1);
        n.vx = 0;
        n.vy = 0;
      });
    });
  }

  function layoutInitial() {
    seedLayout();
    if (state.clusterSections) {
      layoutInitialClustered();
    } else {
      layoutInitialWalk();
    }
  }

  function pageSectionKey(n) {
    var site = n.siteKey || n.site;
    var id = n.sectionID || (state.legacySections ? (n.section || 'Overview') : '\x00root');
    return (site ? site + '\x00' : '') + id;
  }

  function clusterGroups() {
    var groups = Object.create(null);
    var labels = Object.create(null);
    state.nodes.forEach(function (n) {
      if (n.kind === 'page' && !n.stub) {
        var key = pageSectionKey(n);
        labels[key] = n.section || 'Overview';
        (groups[key] || (groups[key] = [])).push(n);
      }
    });
    var tags = state.nodes.filter(function (n) { return n.kind === 'tag'; });
    if (tags.length) {
      groups['\x00tag-hubs'] = tags;
      labels['\x00tag-hubs'] = 'Tag hubs';
    }
    var stubs = state.nodes.filter(function (n) { return n.kind === 'page' && n.stub; });
    if (stubs.length) {
      groups['\x00missing-pages'] = stubs;
      labels['\x00missing-pages'] = 'Missing pages';
    }

    // Distinct sections can share a display title, and synthetic cells can
    // share one with a real section. Keep the cells separate and say which is
    // which when the label alone would be ambiguous.
    var counts = Object.create(null);
    Object.keys(labels).forEach(function (key) {
      counts[labels[key]] = (counts[labels[key]] || 0) + 1;
    });
    Object.keys(labels).forEach(function (key) {
      if (counts[labels[key]] < 2) return;
      if (key.indexOf('\x00root') !== -1) {
        var root = groups[key][0];
        var rootSite = root && (root.siteKey || root.site);
        labels[key] += rootSite ? ' (' + rootSite + ')' : ' (root)';
      } else if (key.indexOf('\x00tag-hubs') !== -1) {
        labels[key] += ' (all tags)';
      } else if (key.indexOf('\x00missing-pages') !== -1) {
        labels[key] += ' (broken links)';
      } else {
        var first = groups[key][0];
        var suffix = first
          ? [first.siteKey || first.site, first.sectionID].filter(Boolean).join(' / ')
          : key;
        labels[key] += ' (' + (suffix || key) + ')';
      }
    });
    state.clusterLabels = labels;
    return groups;
  }

  function clampToSectionCell(n) {
    if (!state.clusterSections || !state.sectionCells) return;
    var key = n.kind === 'tag' ? '\x00tag-hubs' : n.stub ? '\x00missing-pages' : pageSectionKey(n);
    var cell = state.sectionCells[key];
    if (!cell) return;
    var pad = Math.min(24, Math.max(10, Math.min(cell.w, cell.h) * 0.08));
    n.x = Math.max(cell.x + pad, Math.min(cell.x + cell.w - pad, n.x));
    n.y = Math.max(cell.y + pad, Math.min(cell.y + cell.h - pad, n.y));
  }

  function renderSections() {
    if (!sectionLayer) return;
    sectionLayer.textContent = '';
    if (!state.clusterSections || !state.sectionCells) return;
    Object.keys(state.sectionCells).sort().forEach(function (name) {
      var cell = state.sectionCells[name];
      var label = document.createElementNS(NS, 'text');
      label.setAttribute('class', 'graph-section-label');
      var box = document.createElementNS(NS, 'rect');
      box.setAttribute('class', 'graph-section-box');
      box.setAttribute('vector-effect', 'non-scaling-stroke');
      box.setAttribute('x', (cell.x + 6).toFixed(2));
      box.setAttribute('y', (cell.y + 6).toFixed(2));
      box.setAttribute('width', Math.max(0, cell.w - 12).toFixed(2));
      box.setAttribute('height', Math.max(0, cell.h - 12).toFixed(2));
      sectionLayer.appendChild(box);
      label.setAttribute('x', cell.x.toFixed(2));
      label.setAttribute('y', cell.y.toFixed(2));
      label.setAttribute('text-anchor', 'start');
      label.setAttribute('dominant-baseline', 'hanging');
      label.setAttribute('data-section', name);
      label.setAttribute('aria-hidden', 'true');
      label.textContent = state.clusterLabels[name] || name;
      sectionLayer.appendChild(label);
    });
    sizeSectionLabels();
  }

  function sizeSectionLabels() {
    if (!sectionLayer) return;
    var scale = state.scale || 1;
    var pad = 6 / scale;
    Array.prototype.forEach.call(sectionLayer.querySelectorAll('.graph-section-label'), function (label) {
      var key = label.getAttribute('data-section');
      var cell = state.sectionCells && state.sectionCells[key];
      if (!cell) return;
      label.style.fontSize = (12 / scale) + 'px';
      label.setAttribute('x', (cell.x + pad).toFixed(2));
      label.setAttribute('y', (cell.y + pad).toFixed(2));
      label.removeAttribute('textLength');
      label.removeAttribute('lengthAdjust');
      var available = Math.max(18, cell.w - pad * 2);
      if (label.getComputedTextLength() > available) {
        label.setAttribute('textLength', available.toFixed(2));
        label.setAttribute('lengthAdjust', 'spacingAndGlyphs');
      }
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
        state.layoutNudges += 1;
        dx = layoutRandom() - 0.5;
        dy = layoutRandom() - 0.5;
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
      var force = (d - target) * 0.02;
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
      n.vx *= 0.8;
      n.vy *= 0.8;
      // A stiff spring on a long edge can overshoot by hundreds of units in
      // one tick, and a hub with many edges can oscillate forever. Capping the
      // step keeps the cloud settling instead of sloshing.
      var speed = Math.sqrt(n.vx * n.vx + n.vy * n.vy);
      if (speed > 12) {
        n.vx *= 12 / speed;
        n.vy *= 12 / speed;
      }
      n.x += n.vx;
      n.y += n.vy;
      if (n.x < 20) { n.x = 20; n.vx *= -0.4; }
      if (n.x > W - 20) { n.x = W - 20; n.vx *= -0.4; }
      if (n.y < 20) { n.y = 20; n.vy *= -0.4; }
      if (n.y > H - 20) { n.y = H - 20; n.vy *= -0.4; }
      clampToSectionCell(n);
    });
  }

  function draw() {
    sectionLayer.textContent = '';
    edgeLayer.textContent = '';
    nodeLayer.textContent = '';
    state.edgeElements = [];

    state.edges.forEach(function (e) {
      var line = document.createElementNS(NS, 'line');
      var cross = e.from.siteKey && e.to.siteKey && e.from.siteKey !== e.to.siteKey;
      line.setAttribute('class', 'graph-edge ' + e.kind + (cross ? ' cross-site' : ''));
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
        if (swallowClick) {
          swallowClick = false;
          return;
        }
        select(n, true);
      });
      circle.addEventListener('keydown', function (ev) {
        if (ev.key === 'Enter' || ev.key === ' ') {
          ev.preventDefault();
          select(n, true);
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
      n.label = label;

      nodeLayer.appendChild(g);
      n.el = g;
      n.circle = circle;
    });

    renderPositions();
    applyLabels();
  }

  function renderPositions(withEdges) {
    state.nodes.forEach(function (n) {
      if (n.el) {
        n.el.setAttribute('transform', 'translate(' + n.x.toFixed(2) + ',' + n.y.toFixed(2) + ')');
      }
    });
    if (withEdges === false) return;
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
    var generation = ++state.layoutGeneration;
    state.layoutNudges = 0;
    app.setAttribute('data-layout', 'running');

    function finishLayout() {
      if (generation !== state.layoutGeneration) return;
      app.setAttribute('data-layout-nudges', String(state.layoutNudges));
      app.setAttribute('data-layout', 'ready');
    }

    if (state.clusterSections) {
      state.totalTicks = 0;
      state.step = 0;
      normaliseLayout();
      initialView();
      renderPositions();
      renderSections();
      applyFilter();
      applyPendingFocus();
      finishLayout();
      return;
    }
    // A larger world needs fewer ticks than a cramped one, because the walk
    // already starts near the density the springs ask for.
    state.totalTicks = n > 1200 ? 300 : n > 600 ? 280 : n > 300 ? 260 : 320;
    state.step = 0;

    if (n <= 250) {
      for (; state.step < state.totalTicks; state.step++) tick();
      normaliseLayout();
      initialView();
      renderPositions();
      renderSections();
      applyFilter();
      applyPendingFocus();
      finishLayout();
      return;
    }

    setStatus('Laying out ' + n + ' nodes...');
    var paint = 0;
    function frame() {
      if (generation !== state.layoutGeneration) return;
      var end = Math.min(state.step + 4, state.totalTicks);
      for (; state.step < end; state.step++) tick();
      var done = state.step >= state.totalTicks;
      paint += 1;
      // Node and edge coordinates are most of the DOM work and cannot be read
      // while the cloud is still moving, so a dense layout writes them every
      // few frames and draws the edges once, at the end.
      if (done || n <= 1200 || paint % 4 === 0) renderPositions(done || n <= 600);
      if (!done) {
        requestAnimationFrame(frame);
        return;
      }
      normaliseLayout();
      initialView();
      renderPositions();
      renderSections();
      applyFilter();
      applyPendingFocus();
      finishLayout();
    }
    requestAnimationFrame(frame);
  }

  function applyPendingFocus() {
    if (!state.pendingFocus) return;
    var pending = state.pendingFocus;
    state.pendingFocus = null;
    select(pending.node, true, pending.hops, true);
    syncURL('replace');
  }

  function updateEdgeVisibility() {
    app.classList.toggle('hide-edges', !state.focus && 11 * state.scale < EDGE_MIN_TEXT_PX);
  }

  function applyView() {
    svg.setAttribute('viewBox', state.view.x + ' ' + state.view.y + ' ' + state.view.w + ' ' + state.view.h);
    var cw = svg.clientWidth || 1000;
    var ch = svg.clientHeight || 700;
    state.scale = Math.min(cw / state.view.w, ch / state.view.h) || 1;
    updateEdgeVisibility();
    sizeSectionLabels();
    scheduleLabels();
    updateStatus();
  }

  // A dense graph opens on a readable patch around a typical page, not on the
  // whole cloud: at this scale the circles separate and names fit. Fit view
  // remains the map, and selecting a node reframes its neighbourhood.
  function initialView() {
    if (!state.nodes.length) {
      fitView();
      return;
    }
    fitView();
    if (state.clusterSections) return;
    if (!state.anchor) return;
    // A fit that would draw the edges as specks is not a useful first screen,
    // whether the catalogue is large or the canvas is narrow.
    if (state.nodes.length <= LOCAL_VIEW_NODES && 11 * state.scale >= EDGE_MIN_TEXT_PX) return;
    var cw = svg.clientWidth || 1000;
    var ch = svg.clientHeight || 700;
    centerOn(state.anchor, cw / READABLE_SCALE, ch / READABLE_SCALE);
  }

  function centerOn(n, w, h) {
    var x = n.x - w / 2;
    var y = n.y - h / 2;
    x = W > w ? Math.max(0, Math.min(W - w, x)) : (W - w) / 2;
    y = H > h ? Math.max(0, Math.min(H - h, y)) : (H - h) / 2;
    state.overview = false;
    state.view = { x: x, y: y, w: w, h: h };
    applyView();
    updateStatus();
  }

  function fitView() {
    state.overview = true;
    if (!state.nodes.length) {
      state.view = { x: 0, y: 0, w: W, h: H };
      applyView();
      return;
    }
    var minX = Infinity;
    var minY = Infinity;
    var maxX = -Infinity;
    var maxY = -Infinity;
    if (state.nodes.length > 200) {
      // An overview should not let one stranded page decide the scale, so the
      // edges of the bounding box are trimmed to the first and last percent.
      var xr = percentileRange(state.nodes.map(function (n) { return n.x; }));
      var yr = percentileRange(state.nodes.map(function (n) { return n.y; }));
      minX = xr.min;
      maxX = xr.max;
      minY = yr.min;
      maxY = yr.max;
      var pad = 80;
      minX -= pad;
      minY -= pad;
      maxX += pad;
      maxY += pad;
    } else {
      state.nodes.forEach(function (n) {
        var r = nodeRadius(n) + 28;
        minX = Math.min(minX, n.x - r);
        minY = Math.min(minY, n.y - r);
        maxX = Math.max(maxX, n.x + r);
        maxY = Math.max(maxY, n.y + r);
      });
      minX -= 20;
      minY -= 20;
      maxX += 20;
      maxY += 20;
    }
    if (state.clusterSections && state.sectionCells) {
      Object.keys(state.sectionCells).forEach(function (name) {
        var cell = state.sectionCells[name];
        minX = Math.min(minX, cell.x);
        minY = Math.min(minY, cell.y);
        maxX = Math.max(maxX, cell.x + cell.w);
        maxY = Math.max(maxY, cell.y + cell.h);
      });
    }
    state.view = {
      x: minX,
      y: minY,
      w: Math.max(240, maxX - minX),
      h: Math.max(180, maxY - minY)
    };
    applyView();
    updateStatus();
  }

  // The padded extent of a set of nodes, in world units.
  function boundsOf(ids) {
    var minX = Infinity;
    var minY = Infinity;
    var maxX = -Infinity;
    var maxY = -Infinity;
    state.nodes.forEach(function (n) {
      if (ids[n.id] == null) return;
      var r = nodeRadius(n) + 30;
      minX = Math.min(minX, n.x - r);
      minY = Math.min(minY, n.y - r);
      maxX = Math.max(maxX, n.x + r);
      maxY = Math.max(maxY, n.y + r);
    });
    return {
      minX: minX,
      minY: minY,
      maxX: maxX,
      maxY: maxY,
      w: maxX - minX,
      h: maxY - minY
    };
  }

  // Fit the view to one neighbourhood, keeping the canvas aspect so the text
  // is not scaled down by letterboxing.
  function fitBounds(ids) {
    var b = boundsOf(ids);
    if (b.minX === Infinity) return;
    var w = Math.max(320, b.w);
    var h = Math.max(240, b.h);
    var cw = svg.clientWidth || 1000;
    var ch = svg.clientHeight || 700;
    if (w / h < cw / ch) w = h * cw / ch;
    else h = w * ch / cw;
    state.overview = false;
    state.view = {
      x: (b.minX + b.maxX) / 2 - w / 2,
      y: (b.minY + b.maxY) / 2 - h / 2,
      w: w,
      h: h
    };
    applyView();
  }

  function matches(n) {
    if (kind.value === 'page' && (n.kind !== 'page' || n.stub)) return false;
    if (kind.value === 'tag' && n.kind !== 'tag') return false;
    if (kind.value === 'stub' && !n.stub) return false;
    if (state.selectedTags.length) {
      if (n.kind === 'tag') {
        if (state.selectedTags.indexOf(tagSlugOfNode(n)) === -1) return false;
      } else {
        for (var t = 0; t < state.selectedTags.length; t++) {
          if (n.tagSlugs.indexOf(state.selectedTags[t]) === -1) return false;
        }
      }
    }
    var q = state.query;
    if (!q) return true;
    var hay = (n.title + ' ' + n.id + ' ' + n.section + ' ' + n.tags.join(' ') + ' ' + n.site).toLowerCase();
    return hay.indexOf(q) !== -1;
  }

  function applyFilter() {
    state.query = filter.value.trim().toLowerCase();
    state.nodes.forEach(function (n) {
      n.matched = matches(n);
      if (n.el) n.el.classList.toggle('is-dim', !n.matched);
    });
    state.edgeElements.forEach(function (item) {
      item.el.classList.toggle('is-dim', !(item.edge.from.matched && item.edge.to.matched));
    });
    scheduleLabels();
    ensureTabbable();
    updateStatus();
    renderList();
  }

  function updateStatus() {
    var total = state.nodes.length;
    var shown = 0;
    state.nodes.forEach(function (n) { if (n.matched) shown += 1; });
    var text = 'Showing ' + shown + ' of ' + total + (total === 1 ? ' node' : ' nodes') + '.';
    if (state.focus && state.selected) {
      text += ' Focused on ' + state.selected.title + ': ' + state.focusCount +
        (state.focusCount === 1 ? ' node' : ' nodes') + ' within ' +
        state.focusHops + (state.focusHops === 1 ? ' hop' : ' hops') + '. Press Escape to clear.';
    } else if (state.focusMissing) {
      text += ' Focus node not found; showing the default view.';
    } else if (state.overview && total > LOCAL_VIEW_NODES) {
      text += ' Overview zoom. Select a node or zoom in for its neighbourhood.';
    } else {
      text += ' Scroll to zoom, drag the background to pan, or select Fit view.';
    }
    if (labels.checked && !pagesMayLabel() && hasOrdinaryPages()) {
      text += ' Labels show hubs; zoom in for page names.';
    }
    var missing = state.selectedTags.filter(function (slug) {
      return !state.tagBySlug[slug];
    });
    if (missing.length) {
      text += ' Not in this graph: ' + missing.map(tagLabel).join(', ') + '.';
    }
    setStatus(text);
    var desc = document.getElementById('graphDesc');
    if (desc) desc.textContent = 'Pages and tags as nodes. ' + text +
      ' Use the arrow keys to move between nodes and Enter for details.';
  }

  function applyLabels() {
    app.classList.toggle('hide-labels', !labels.checked);
    placeLabels();
  }

  function scheduleLabels() {
    if (state.labelFrame) return;
    state.labelFrame = requestAnimationFrame(function () {
      state.labelFrame = 0;
      placeLabels();
    });
  }

  // An edge whose endpoints are both off-screen cannot be read, and a dense
  // graph has thousands of them crossing a local view. An edge that leaves
  // the view is kept, so the connection to what is outside stays visible.
  function cullEdges() {
    var scale = state.scale || 1;
    var view = state.view;
    var last = state.cullView;
    if (last && Math.abs(view.x - last.x) < 40 && Math.abs(view.y - last.y) < 40 &&
        Math.abs(view.w - last.w) < 1 && Math.abs(view.h - last.h) < 1 &&
        Math.abs(scale - last.scale) < 0.01) return;
    state.cullView = { x: view.x, y: view.y, w: view.w, h: view.h, scale: scale };
    var margin = 30 / scale;
    var x0 = view.x - margin;
    var x1 = view.x + view.w + margin;
    var y0 = view.y - margin;
    var y1 = view.y + view.h + margin;
    state.edgeElements.forEach(function (item) {
      var e = item.edge;
      var fromIn = e.from.x >= x0 && e.from.x <= x1 && e.from.y >= y0 && e.from.y <= y1;
      var toIn = e.to.x >= x0 && e.to.x <= x1 && e.to.y >= y0 && e.to.y <= y1;
      item.el.classList.toggle('is-culled', !fromIn && !toIn);
    });
  }

  function isLabelHub(n) {
    return n.kind === 'tag' || !!state.hubIDs[n.id];
  }

  function labelTier(n) {
    if (n === state.selected) return 0;
    return isLabelHub(n) ? 1 : 2;
  }

  // A focus, a filter or a zoomed-in view is a request for page names. The
  // map itself stays at hubs.
  function pagesMayLabel() {
    return state.scale >= PAGE_LABEL_SCALE || !!state.focus || !!state.query ||
      (kind && kind.value !== 'all') || state.selectedTags.length > 0;
  }

  function hasOrdinaryPages() {
    for (var i = 0; i < state.nodes.length; i++) {
      if (labelTier(state.nodes[i]) === 2) return true;
    }
    return false;
  }

  // Labels are placed greedily, the busiest nodes and the selection first. A
  // name is drawn only when it fits without covering another name or another
  // node, and both tests use the drawn text rather than a character count. A
  // dense view therefore shows fewer names, and the node still reveals its
  // own on hover or selection. The names keep their screen size at every zoom.
  function placeLabels() {
    cullEdges();
    var scale = state.scale || 1;
    var fontWorld = 11 / scale;
    var relayout = state.labelScale !== scale;
    state.labelScale = scale;
    var order = state.nodes.slice().sort(function (a, b) {
      return labelTier(a) - labelTier(b) || b.degree - a.degree ||
        a.title.localeCompare(b.title);
    });
    order.forEach(function (n) {
      var label = n.label;
      if (!label) return;
      label.setAttribute('data-tier', String(labelTier(n)));
      label.classList.add('is-colliding');
      if (relayout) {
        label.style.fontSize = fontWorld + 'px';
        label.setAttribute('y', String(-(nodeRadius(n) + 4 / scale + fontWorld * 0.2)));
      }
    });
    if (!labels.checked || !state.nodes.length) return;

    var grid = circleGrid();
    var boxes = [];
    if (sectionLayer) {
      Array.prototype.forEach.call(sectionLayer.querySelectorAll('.graph-section-label'), function (label) {
        var box = label.getBBox();
        if (!box.width || !box.height) return;
        boxes.push({ x: box.x - 2, y: box.y - 2, w: box.width + 4, h: box.height + 4 });
      });
    }
    var placed = 0;
    for (var i = 0; i < order.length && placed < LABEL_LIMIT; i++) {
      var n = order[i];
      if (!n.matched) continue;
      if (labelTier(n) === 2 && !pagesMayLabel()) continue;
      if (state.focus && state.focus[n.id] == null) continue;
      var view = state.view;
      var margin = 40 / scale;
      if (n.x < view.x - margin || n.x > view.x + view.w + margin ||
          n.y < view.y - margin || n.y > view.y + view.h + margin) continue;
      var label = n.label;
      if (!label) continue;
      var text = label.textContent || '';
      var w = labelWidth(label, text) / scale;
      if (!w) continue;
      var r = nodeRadius(n);
      var gap = 4 / scale;
      var boxH = fontWorld;
      var rect = {
        x: n.x - w / 2 - 1,
        y: n.y - r - gap - boxH,
        w: w + 2,
        h: boxH + 1
      };
      var chosen = n === state.selected;
      if (!chosen && coversCircle(grid, rect)) continue;
      if (!chosen && coversBox(boxes, rect)) continue;
      boxes.push(rect);
      placed += 1;
      label.classList.remove('is-colliding');
    }
  }

  var measureCanvas = null;

  function labelWidth(label, text) {
    if (state.widths[text]) return state.widths[text];
    var w = 0;
    if (!measureCanvas) measureCanvas = document.createElement('canvas');
    var ctx = measureCanvas.getContext && measureCanvas.getContext('2d');
    if (ctx) {
      var cs = window.getComputedStyle(label);
      try {
        // Always measure at the screen size the name is drawn at; the world
        // width then follows from the zoom.
        ctx.font = '11px ' + (cs.fontFamily || 'sans-serif');
        w = ctx.measureText(text).width;
      } catch (e) {
        w = 0;
      }
    }
    if (!w) w = text.length * 5.6;
    state.widths[text] = w;
    return w;
  }

  function circleGrid() {
    var cell = 64;
    var cells = Object.create(null);
    state.nodes.forEach(function (n) {
      var key = Math.floor(n.x / cell) + ',' + Math.floor(n.y / cell);
      (cells[key] || (cells[key] = [])).push(n);
    });
    return { cell: cell, cells: cells };
  }

  function coversCircle(grid, rect) {
    var cell = grid.cell;
    var x0 = Math.floor(rect.x / cell);
    var x1 = Math.floor((rect.x + rect.w) / cell);
    var y0 = Math.floor(rect.y / cell);
    var y1 = Math.floor((rect.y + rect.h) / cell);
    for (var gx = x0; gx <= x1; gx++) {
      for (var gy = y0; gy <= y1; gy++) {
        var bucket = grid.cells[gx + ',' + gy];
        if (!bucket) continue;
        for (var i = 0; i < bucket.length; i++) {
          var n = bucket[i];
          // A faded node is context, not an obstacle: names of the focus set
          // or of a filter's matches may sit over the pages it hides.
          if (n.el && n.el.classList.contains('is-dim')) continue;
          if (state.focus && state.focus[n.id] == null) continue;
          var r = nodeRadius(n) + 1;
          var cx = Math.max(rect.x, Math.min(n.x, rect.x + rect.w));
          var cy = Math.max(rect.y, Math.min(n.y, rect.y + rect.h));
          var dx = cx - n.x;
          var dy = cy - n.y;
          if (dx * dx + dy * dy < r * r) return true;
        }
      }
    }
    return false;
  }

  function coversBox(boxes, rect) {
    for (var i = 0; i < boxes.length; i++) {
      var b = boxes[i];
      if (rect.x < b.x + b.w && rect.x + rect.w > b.x &&
          rect.y < b.y + b.h && rect.y + rect.h > b.y) return true;
    }
    return false;
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
  function reachable(n) {
    return n.el && !n.el.classList.contains('is-dim') && !n.el.classList.contains('is-focus-far');
  }

  function ensureTabbable() {
    var current = state.tabbable;
    if (current && reachable(current)) return;
    for (var i = 0; i < state.nodes.length; i++) {
      var n = state.nodes[i];
      if (reachable(n)) {
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
      if (reachable(candidate)) {
        setTabbable(candidate);
        candidate.circle.focus();
        return;
      }
    }
  }

  function focusEdge(last) {
    var visible = state.nodes.filter(reachable);
    if (!visible.length) return;
    var node = last ? visible[visible.length - 1] : visible[0];
    setTabbable(node);
    node.circle.focus();
  }

  // Walk the catalogue outward from one node. Two hops is the useful size for
  // a page; a tag hub is narrower and would otherwise pull in most of the
  // bundle through its own pages.
  function bfs(node, hops) {
    var seen = Object.create(null);
    var order = [node];
    seen[node.id] = 0;
    for (var i = 0; i < order.length; i++) {
      var current = order[i];
      var depth = seen[current.id];
      if (depth >= hops) continue;
      var links = state.adj[current.id] || [];
      for (var j = 0; j < links.length; j++) {
        var next = links[j].node;
        if (seen[next.id] == null) {
          seen[next.id] = depth + 1;
          order.push(next);
        }
      }
    }
    return { ids: seen, size: order.length };
  }

  function applyFocus() {
    var ids = state.focus;
    state.nodes.forEach(function (n) {
      var keepMatch = !!ids && !!state.query && n.matched;
      if (n.el) n.el.classList.toggle('is-focus-far', !!ids && ids[n.id] == null && !keepMatch);
    });
    app.classList.toggle('has-focus', !!ids);
    if (clearFocusLink) clearFocusLink.hidden = !ids;
    updateEdgeVisibility();
    state.edgeElements.forEach(function (item) {
      var e = item.edge;
      var on = !ids || (ids[e.from.id] != null && ids[e.to.id] != null) ||
        (!!state.query && e.from.matched && e.to.matched);
      item.el.classList.toggle('is-focus-far', !on);
    });
    ensureTabbable();
    scheduleLabels();
    renderList();
  }

  function focusOn(n, fit, hopsOverride) {
    var hops = hopsOverride || (n.kind === 'tag' ? 1 : 2);
    state.focusMissing = false;
    var found = bfs(n, hops);
    if (found.size > 120 && hops > 1) {
      hops = 1;
      found = bfs(n, hops);
    }
    state.focus = found.ids;
    state.focusCount = found.size;
    state.focusHops = hops;
    applyFocus();
    if (fit && found.size <= 300) {
      var bounds = boundsOf(found.ids);
      var cw = svg.clientWidth || 1000;
      var ch = svg.clientHeight || 700;
      var focusW = cw / FOCUS_SCALE;
      var focusH = ch / FOCUS_SCALE;
      // A neighbourhood that reaches across the whole drawing cannot be fitted
      // without losing the shapes, so stay with the node and let the panel
      // carry the list of far connections.
      if (bounds.w <= focusW && bounds.h <= focusH) {
        fitBounds(found.ids);
      } else {
        centerOn(n, focusW, focusH);
      }
    }
    updateStatus();
  }

  function clearFocus() {
    state.focus = null;
    state.focusCount = 0;
    state.focusHops = 0;
    applyFocus();
  }

  function directionLabel(dirs) {
    if (dirs['in'] && dirs.out) return 'linked both ways';
    return dirs.out ? 'linked from this page' : 'links to this page';
  }

  function connections(n) {
    var links = state.adj[n.id] || [];
    var merged = Object.create(null);
    var list = [];
    var incoming = 0;
    var outgoing = 0;
    links.forEach(function (link) {
      if (link.dir === 'in') incoming += 1;
      else outgoing += 1;
      var entry = merged[link.node.id];
      if (!entry) {
        entry = { node: link.node, dirs: Object.create(null) };
        merged[link.node.id] = entry;
        list.push(entry);
      }
      entry.dirs[link.dir] = true;
    });
    list.sort(function (a, b) { return a.node.title.localeCompare(b.node.title); });
    return { list: list, incoming: incoming, outgoing: outgoing };
  }

  function select(n, withView, hops, restoring) {
    if (!restoring) state.pendingFocus = null;
    var keepFocus = details.contains(document.activeElement);
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
    title.tabIndex = -1;
    title.textContent = n.title;
    details.appendChild(title);

    var links = connections(n);
    var meta = document.createElement('p');
    meta.className = 'graph-details-meta';
    var bits = [links.incoming + ' incoming', links.outgoing + ' outgoing'];
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

    if (links.list.length) {
      var caption = document.createElement('p');
      caption.className = 'graph-details-kind';
      caption.textContent = links.list.length === 1 ? '1 connection' : links.list.length + ' connections';
      details.appendChild(caption);

      var list = document.createElement('ul');
      list.className = 'graph-neighbours';
      var shown = 0;
      function addConnection(entry) {
        var item = document.createElement('li');
        var button = document.createElement('button');
        button.type = 'button';
        button.className = 'graph-neighbour';
        button.textContent = entry.node.title;
        button.setAttribute('aria-label', entry.node.title + ', ' + directionLabel(entry.dirs));
        button.addEventListener('click', function () { select(entry.node, true); });
        item.appendChild(button);
        list.appendChild(item);
      }
      for (; shown < Math.min(40, links.list.length); shown++) addConnection(links.list[shown]);
      details.appendChild(list);
      if (shown < links.list.length) {
        var more = document.createElement('button');
        more.type = 'button';
        more.className = 'graph-more';
        more.textContent = 'Show all ' + links.list.length + ' connections';
        more.addEventListener('click', function () {
          var start = shown;
          for (; shown < links.list.length; shown++) addConnection(links.list[shown]);
          more.remove();
          var added = list.children[start];
          if (added) added.querySelector('button').focus();
        });
        details.appendChild(more);
      }
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

    var clear = document.createElement('button');
    clear.type = 'button';
    clear.className = 'graph-clear';
    clear.textContent = 'Clear selection';
    clear.addEventListener('click', function () { clearSelection(); });
    details.appendChild(clear);

    details.hidden = false;
    if (keepFocus) title.focus();
    focusOn(n, !!withView, hops);
    updateListSelection(n);
    if (!restoring) syncURL('push');
  }

  function clearSelection(restoring) {
    if (!restoring) state.pendingFocus = null;
    var restoreGraphFocus = (clearFocusLink && document.activeElement === clearFocusLink) ||
      details.contains(document.activeElement);
    state.selected = null;
    state.nodes.forEach(function (n) {
      if (n.el) n.el.classList.remove('is-selected');
    });
    clearFocus();
    details.hidden = true;
    details.textContent = '';
    updateStatus();
    updateListSelection(null);
    if (!restoring) syncURL('push');
    if (restoreGraphFocus) {
      if (state.tabbable && state.tabbable.circle) state.tabbable.circle.focus();
      else fit.focus();
    }
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
    swallowClick = false;
    state.dragging = n;
    state.dragMoved = false;
    state.dragFrom = { x: ev.clientX, y: ev.clientY };
    n.fixed = true;
    n.circle.setPointerCapture(ev.pointerId);
    svg.classList.add('is-dragging');
    // A drag should not reframe the view under the pointer, but the rest of
    // the graph can still fade to the dragged node's neighbourhood.
    select(n, false);
  }

  filter.addEventListener('input', function () {
    applyFilter();
    syncURL('replace');
  });
  kind.addEventListener('change', function () {
    applyFilter();
    syncURL('push');
  });
  labels.addEventListener('change', applyLabels);
  if (sectionsCheck) {
    sectionsCheck.addEventListener('change', function () {
      state.clusterSections = sectionsCheck.checked;
      relayoutGraph(false);
    });
  }
  if (tagHubsCheck) {
    tagHubsCheck.addEventListener('change', function () {
      state.showTagHubs = tagHubsCheck.checked;
      updateTagKindOption();
      relayoutGraph(true);
    });
  }
  fit.addEventListener('click', function () { fitView(); });
  if (clearFocusLink) {
    clearFocusLink.addEventListener('click', function (ev) {
      ev.preventDefault();
      clearSelection();
      fitView();
    });
  }
  if (tagsMenu) {
    tagsMenu.addEventListener('change', function (ev) {
      if (!ev.target || ev.target.type !== 'checkbox') return;
      var selected = [];
      Array.prototype.forEach.call(tagsMenu.querySelectorAll('input[type="checkbox"]:checked'), function (box) {
        selected.push(box.value);
      });
      state.selectedTags = selected;
      updateTagSummary();
      applyFilter();
      syncURL('push');
    });
  }
  if (listNodes) {
    listNodes.addEventListener('keydown', function (ev) {
      if (ev.key !== 'ArrowDown' && ev.key !== 'ArrowUp' &&
          ev.key !== 'Home' && ev.key !== 'End') return;
      var buttons = listButtons();
      if (!buttons.length) return;
      var at = buttons.indexOf(document.activeElement);
      if (at < 0) at = 0;
      var next = at;
      if (ev.key === 'ArrowDown') next = Math.min(buttons.length - 1, at + 1);
      else if (ev.key === 'ArrowUp') next = Math.max(0, at - 1);
      else if (ev.key === 'Home') next = 0;
      else if (ev.key === 'End') next = buttons.length - 1;
      buttons.forEach(function (button, index) {
        button.tabIndex = index === next ? 0 : -1;
      });
      buttons[next].focus();
      ev.preventDefault();
    });
  }

  if (tagsDetails) {
    tagsDetails.addEventListener('toggle', function () {
      if (tagsDetails.open) {
        positionTagsMenu();
      } else {
        tagsMenu.style.left = '';
        tagsMenu.style.right = '';
      }
    });
    document.addEventListener('click', function (ev) {
      if (tagsDetails.open && !tagsDetails.contains(ev.target)) tagsDetails.open = false;
    });
  }

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
    var previousW = v.w;
    var factor = Math.exp(ev.deltaY * 0.0015);
    var maxW = Math.max(W, 4000);
    var nextW = Math.min(maxW, Math.max(180, v.w * factor));
    var nextH = nextW * (v.h / v.w);
    var pt = toSvg(ev.clientX, ev.clientY);
    v.x = pt.x - (pt.x - v.x) * (nextW / v.w);
    v.y = pt.y - (pt.y - v.y) * (nextH / v.h);
    v.w = nextW;
    v.h = nextH;
    applyView();
    if (state.overview && nextW < previousW) {
      state.overview = false;
      updateStatus();
    }
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
      var slop = ev.pointerType === 'touch' ? 10 : 3;
      if (Math.abs(ev.clientX - state.dragFrom.x) > slop ||
          Math.abs(ev.clientY - state.dragFrom.y) > slop) {
        state.dragMoved = true;
      }
      var pt = toSvg(ev.clientX, ev.clientY);
      state.dragging.x = pt.x;
      state.dragging.y = pt.y;
      clampToSectionCell(state.dragging);
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
      state.cullView = null;
    }
    if (state.dragging) {
      var moved = state.dragMoved;
      state.dragging.fixed = false;
      state.dragging = null;
      state.dragMoved = false;
      state.cullView = null;
      svg.classList.remove('is-dragging');
      if (moved) swallowClick = true;
    }
    if (state.nodes.length) placeLabels();
    if (state.clusterSections) renderSections();
  }
  svg.addEventListener('pointerup', endPointer);
  svg.addEventListener('pointercancel', endPointer);

  document.addEventListener('keydown', function (ev) {
    if (ev.key === 'Escape' && tagsDetails && tagsDetails.open) {
      ev.preventDefault();
      tagsDetails.open = false;
      if (tagsSummary) tagsSummary.focus();
      return;
    }
    if (ev.key === 'Escape' && state.selected) clearSelection();
  });

  window.addEventListener('popstate', applyURLState);

  // A local face swaps in after first paint; widths measured against the
  // fallback would keep names overlapping for the life of the page.
  if (document.fonts && document.fonts.ready) {
    document.fonts.ready.then(function () {
      state.widths = Object.create(null);
      scheduleLabels();
    });
  }

  window.addEventListener('resize', function () {
    if (tagsDetails && tagsDetails.open) positionTagsMenu();
    applyView();
    if (!state.overview || !state.anchor || 11 * state.scale >= EDGE_MIN_TEXT_PX) return;
    var cw = svg.clientWidth || 1000;
    var ch = svg.clientHeight || 700;
    centerOn(state.anchor, cw / READABLE_SCALE, ch / READABLE_SCALE);
  });

  load();
})();
