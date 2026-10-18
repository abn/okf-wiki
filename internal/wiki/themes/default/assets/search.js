(function () {
  'use strict';

  var searchIndex = null;
  // searchIndexLower mirrors searchIndex with title, doc and section lowercased,
  // so runSearch matches without lowercasing every field of every entry on every
  // keystroke. Content is the large field and dominates the index, so it is
  // lowercased once per entry by lowerContent on first scan rather than copied
  // whole at load: a session that prefetches the index but never searches then
  // never holds the second large string per entry.
  var searchIndexLower = null;
  // searchIndexPromise holds the in-flight index request. A caller that asks
  // while the first fetch is still loading must be handed that same request, not
  // a promise resolved with a searchIndex that is still null: runSearch would
  // then call fetchIndex again from its own .then, and since both would run as
  // microtasks the loop never yields to the event loop, so the tab hangs and the
  // request it is waiting for can never complete.
  var searchIndexPromise = null;
  var selectedIndex = -1;
  var currentResults = [];
  // currentGroups is null for a flat list (a single bundle, or a tag search) and
  // otherwise one entry per wiki: { site, count }. currentResults stays the flat
  // list the keyboard walks, so ArrowUp/ArrowDown/Enter are unaffected by the
  // grouping.
  var currentGroups = null;
  var lastActiveElement = null;
  var status = null;
  var announcedSet = null;
  var inputTimer = null;

  // A tree of wikis publishes one aggregate index at the tree base and names it
  // in window.__WIKI_SEARCH. Its own base is still window.__WIKI_BASE, which is
  // where "this wiki" ends and where the tag catalogue lives. When the two are
  // equal there is no tree to search across.
  var base = window.__WIKI_BASE || '/wiki/';
  var searchBase = window.__WIKI_SEARCH || base;
  var multiSite = !!window.__WIKI_SEARCH && window.__WIKI_SEARCH !== window.__WIKI_BASE;
  var scopeAll = true;

  var searchBtn = document.getElementById('searchBtn');
  var backdrop = document.getElementById('searchBackdrop');
  var input = document.getElementById('searchInput');
  var closeBtn = document.getElementById('searchCloseBtn');
  var scopeBtn = document.getElementById('searchScope');
  var resultsContainer = document.getElementById('searchResults');
  var resultsHost = null;
  var modal = document.querySelector('.search-modal') || backdrop;

  if (!backdrop || !input || !resultsContainer) {
    return;
  }

  // expandIndex turns the on-disk page table and its entries into the flat
  // entry objects the rest of the file reads. A page's url and titles are held
  // once and referenced by index, so the expansion shares the table's strings
  // rather than copying them per entry, and the deep link is the page url plus
  // the entry's anchor, an empty anchor being the page itself.
  function expandIndex(data) {
    var pages = (data && data.pages) || [];
    var entries = (data && data.entries) || [];
    var out = new Array(entries.length);
    for (var i = 0; i < entries.length; i++) {
      var entry = entries[i];
      var page = pages[entry.g] || {};
      var url = page.u || '';
      out[i] = {
        u: entry.a ? url + '#' + entry.a : url,
        p: url,
        t: entry.t || page.t,
        d: page.t,
        s: entry.s,
        c: entry.c,
        w: page.w,
        k: page.k
      };
    }
    return out;
  }

  // buildLowerIndex mirrors the index with the three small match fields already
  // lowercased. runSearch reads these instead of lowercasing the entry fields on
  // every keystroke.
  function buildLowerIndex(index) {
    var lower = new Array(index.length);
    for (var i = 0; i < index.length; i++) {
      var entry = index[i];
      lower[i] = {
        t: entry.t ? entry.t.toLowerCase() : '',
        d: entry.d ? entry.d.toLowerCase() : '',
        s: entry.s ? entry.s.toLowerCase() : '',
        c: null
      };
    }
    return lower;
  }

  // lowerContent fills in one entry's lowercased content on first use and keeps
  // it. The null in buildLowerIndex marks the not-yet-lowercased state, so an
  // entry whose content is empty is cached as '' and not recomputed.
  function lowerContent(i) {
    var cached = searchIndexLower[i].c;
    if (cached === null) {
      var content = searchIndex[i].c;
      cached = content ? content.toLowerCase() : '';
      searchIndexLower[i].c = cached;
    }
    return cached;
  }

  function fetchIndex() {
    if (searchIndex) return Promise.resolve(searchIndex);
    if (searchIndexPromise) return searchIndexPromise;
    var v = window.__WIKI_ASSET ? '?v=' + window.__WIKI_ASSET : '';
    searchIndexPromise = fetch(searchBase + 'search-index.json' + v)
      .then(function (res) {
        if (!res.ok) throw new Error('HTTP ' + res.status);
        return res.json();
      })
      .then(function (data) {
        searchIndex = expandIndex(data);
        searchIndexLower = buildLowerIndex(searchIndex);
        return searchIndex;
      })
      .catch(function (err) {
        // An empty catalogue rather than a null one, so runSearch does not treat
        // a failed load as "still loading" and retry on every keystroke.
        console.warn('Failed to load wiki search index:', err);
        searchIndex = [];
        searchIndexLower = [];
        return searchIndex;
      });
    return searchIndexPromise;
  }

  // setScope flips the All-wikis / This-wiki toggle and announces the state.
  // The button only exists in a tree, but setScope is safe to call regardless.
  function setScope(all) {
    scopeAll = all;
    if (scopeBtn) {
      scopeBtn.setAttribute('aria-pressed', all ? 'true' : 'false');
      scopeBtn.textContent = all ? 'All wikis' : 'This wiki';
    }
  }

  function openSearch() {
    lastActiveElement = document.activeElement;
    // The announced set belongs to the session that is ending. Without the reset
    // a dialog reopened on the same terms says nothing, because the key it would
    // write is the one already in the status node.
    announcedSet = null;
    backdrop.classList.add('open');
    backdrop.setAttribute('aria-hidden', 'false');
    document.body.style.overflow = 'hidden';
    input.value = '';
    selectedIndex = -1;
    currentResults = [];
    currentGroups = null;
    setScope(true);
    renderInitialState();
    fetchIndex();
    setTimeout(function () { input.focus(); }, 20);
  }

  function closeSearch() {
    if (!backdrop.classList.contains('open')) return;
    // A pending search would otherwise run after the dialog is hidden and
    // announce its count from a closed live region.
    if (inputTimer) { clearTimeout(inputTimer); inputTimer = null; }
    backdrop.classList.remove('open');
    backdrop.setAttribute('aria-hidden', 'true');
    document.body.style.overflow = '';
    if (lastActiveElement && typeof lastActiveElement.focus === 'function') lastActiveElement.focus();
    lastActiveElement = null;
  }

  function escapeHTML(str) {
    if (!str) return '';
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  function escapeRegExp(str) {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  }

  function highlight(text, terms) {
    if (!text || !terms || terms.length === 0) return escapeHTML(text);
    var escaped = escapeHTML(text);
    var pattern = terms.map(function (t) { return escapeRegExp(escapeHTML(t)); }).join('|');
    if (!pattern) return escaped;
    var regex = new RegExp('(' + pattern + ')', 'gi');
    return escaped.replace(regex, '<mark class="search-match">$1</mark>');
  }

  function extractSnippet(content, terms) {
    if (!content) return '';
    var lower = content.toLowerCase();
    var firstPos = -1;
    for (var i = 0; i < terms.length; i++) {
      var pos = lower.indexOf(terms[i]);
      if (pos !== -1 && (firstPos === -1 || pos < firstPos)) firstPos = pos;
    }
    if (firstPos === -1) {
      var snippet = content.slice(0, 110);
      return highlight(snippet + (content.length > 110 ? '...' : ''), terms);
    }
    var start = Math.max(0, firstPos - 35);
    var end = Math.min(content.length, firstPos + 95);
    var prefix = start > 0 ? '...' : '';
    var suffix = end < content.length ? '...' : '';
    return prefix + highlight(content.slice(start, end), terms) + suffix;
  }

  // Tags live in a separate file from the text index, and are only fetched
  // for hash-prefixed queries, so a text search never pays for them. The
  // in-flight request is memoized for the same reason as the text index: a
  // second caller must wait on it rather than spin on a not-yet-loaded
  // catalogue.
  var tagsIndex = null;
  var tagsPromise = null;

  function fetchTags() {
    if (tagsIndex) return Promise.resolve(tagsIndex);
    if (tagsPromise) return tagsPromise;
    var v = window.__WIKI_ASSET ? '?v=' + window.__WIKI_ASSET : '';
    var base = window.__WIKI_BASE || '/wiki/';
    tagsPromise = fetch(base + 'tags.json' + v)
      .then(function (res) {
        if (!res.ok) throw new Error('HTTP ' + res.status);
        return res.json();
      })
      .then(function (data) {
        tagsIndex = data || [];
        return tagsIndex;
      })
      .catch(function (err) {
        console.warn('Failed to load wiki tag catalogue:', err);
        tagsIndex = [];
        return tagsIndex;
      });
    return tagsPromise;
  }

  // A query starting with # searches the tag namespace rather than page
  // text. Each hit is a tag page, which itself lists every page carrying it.
  function runTagSearch(tagQuery) {
    if (!tagQuery) {
      selectedIndex = -1;
      currentResults = [];
      resultsHost.innerHTML =
        '<div class="search-empty-state"><p>Type a tag name after #.</p></div>';
      announce('Type a tag name after #');
      return;
    }
    if (!tagsIndex) {
      resultsHost.innerHTML = '<div class="search-empty-state"><p>Loading tags...</p></div>';
      announce('Loading tags');
      fetchTags().then(runSearch);
      return;
    }

    var scored = [];
    for (var i = 0; i < tagsIndex.length; i++) {
      var tag = tagsIndex[i];
      var name = tag.t ? tag.t.toLowerCase() : '';
      var score = -1;
      if (name === tagQuery) score = 100;
      else if (name.indexOf(tagQuery) === 0) score = 50;
      else if (name.indexOf(tagQuery) !== -1) score = 10;
      if (score >= 0) scored.push({ tag: tag, score: score });
    }
    scored.sort(function (a, b) {
      if (b.score !== a.score) return b.score - a.score;
      var an = a.tag.t.toLowerCase(), bn = b.tag.t.toLowerCase();
      return an < bn ? -1 : an > bn ? 1 : 0;
    });

    // Rows reuse the page-result shape, so keyboard, mouse and Enter work
    // unchanged: the section reads Tags, the doc reads the page count.
    currentResults = scored.slice(0, 12).map(function (s) {
      return {
        u: s.tag.u,
        s: 'Tags',
        d: s.tag.n === 1 ? '1 page' : s.tag.n + ' pages',
        t: '#' + s.tag.t,
        c: ''
      };
    });
    currentGroups = null;
    renderResults([tagQuery], '#' + tagQuery);
  }

  function renderInitialState() {
    currentGroups = null;
    var links = (window.__WIKI_QUICK || []).slice(0, 6);
    var html = '<div class="search-empty-state">' +
      '<p>Type a search term, or jump to a section.</p>' +
      '<p class="search-empty-hint">Prefix with # to search tags only.</p>';
    if (links.length) {
      html += '<div class="search-quick-links"><span>Jump to:</span>';
      for (var i = 0; i < links.length; i++) {
        html += '<a href="' + escapeHTML(links[i].href) + '">' + escapeHTML(links[i].label) + '</a>';
      }
      html += '</div>';
    }
    html += '</div>';
    resultsHost.innerHTML = html;
    announce('Type a search term, or jump to a section.');
  }

  function runSearch() {
    // A debounced search or a fetch continuation can outlive the dialog: Escape
    // closes it while the timer or the request is still in flight. Writing the
    // results then would speak into the live region of a hidden dialog, so a
    // closed dialog runs nothing.
    if (!backdrop.classList.contains('open')) return;
    var query = input.value.trim().toLowerCase();
    if (!query) {
      selectedIndex = -1;
      currentResults = [];
      renderInitialState();
      return;
    }
    if (query.charAt(0) === '#') {
      runTagSearch(query.slice(1).trim());
      return;
    }
    if (!searchIndex) {
      resultsHost.innerHTML = '<div class="search-empty-state"><p>Loading search index...</p></div>';
      announce('Loading search index');
      fetchIndex().then(runSearch);
      return;
    }

    var terms = query.split(/\s+/).filter(Boolean);
    if (terms.length === 0) { renderInitialState(); return; }

    var matches = [];
    for (var i = 0; i < searchIndex.length; i++) {
      var entry = searchIndex[i];
      var lower = searchIndexLower[i];
      var titleLower = lower.t;
      var docLower = lower.d;
      var secLower = lower.s;
      var contentLower = lowerContent(i);
      var allMatch = true;
      var score = 0;

      for (var j = 0; j < terms.length; j++) {
        var term = terms[j];
        var termMatched = false;
        if (titleLower.indexOf(term) !== -1) {
          termMatched = true;
          score += 45;
          if (titleLower === term || titleLower.startsWith(term + ' ')) score += 40;
        }
        if (docLower.indexOf(term) !== -1) { termMatched = true; score += 25; }
        if (secLower.indexOf(term) !== -1) { termMatched = true; score += 15; }
        if (contentLower.indexOf(term) !== -1) { termMatched = true; score += 10; }
        if (!termMatched) { allMatch = false; break; }
      }

      if (allMatch) {
        if (titleLower.indexOf(query) !== -1) score += 80;
        else if (docLower.indexOf(query) !== -1) score += 40;
        else if (contentLower.indexOf(query) !== -1) score += 30;
        matches.push({ entry: entry, score: score });
      }
    }

    matches.sort(function (a, b) { return b.score - a.score; });

    // "This wiki" keeps only the current wiki's rows. A tree names the wiki in
    // the URL, so the current wiki's base is the prefix of exactly its own rows;
    // the trailing slash stops /family/a/ from also matching a sibling /family/ab/.
    if (multiSite && !scopeAll) {
      matches = matches.filter(function (m) {
        return m.entry.u && m.entry.u.indexOf(base) === 0;
      });
    }

    var grouped = groupMatches(matches);
    if (grouped) {
      currentResults = grouped.rows;
      currentGroups = grouped.groups;
    } else {
      currentResults = matches.slice(0, MAX_FLAT).map(function (m) { return m.entry; });
      currentGroups = null;
    }
    renderResults(terms, query);
  }

  // Caps: a multi-site result list shows the best few rows of each wiki and
  // stops at a screenful, so one wiki cannot bury every other. A single bundle
  // keeps the flat list it always had.
  var MAX_FLAT = 12;
  var MAX_PER_SITE = 5;
  var MAX_TOTAL = 20;

  // groupMatches splits scored matches by wiki, orders the groups by their best
  // hit, and caps each group and the total. It returns null when the entries
  // carry no wiki, which is a single bundle: the caller then renders the flat
  // list, with no headings, exactly as before.
  function groupMatches(matches) {
    var groups = [];
    var byKey = {};
    for (var i = 0; i < matches.length; i++) {
      var entry = matches[i].entry;
      if (!entry.w && !entry.k) return null;
      var key = entry.k || entry.w;
      var g = byKey[key];
      if (!g) {
        g = { site: entry.w || '', best: matches[i].score, rows: [] };
        byKey[key] = g;
        groups.push(g);
      }
      g.rows.push(entry);
    }
    groups.sort(function (a, b) { return b.best - a.best; });

    var rows = [];
    var meta = [];
    for (var gi = 0; gi < groups.length && rows.length < MAX_TOTAL; gi++) {
      var grp = groups[gi];
      var take = Math.min(grp.rows.length, MAX_PER_SITE, MAX_TOTAL - rows.length);
      if (take <= 0) break;
      for (var j = 0; j < take; j++) rows.push(grp.rows[j]);
      meta.push({ site: grp.site, count: take });
    }
    return { rows: rows, groups: meta };
  }

  // resultItemHTML is one search row. index is its position in the flat,
  // contiguous result list, which is what the keyboard walks.
  function resultItemHTML(item, index, terms) {
    var isSelected = index === selectedIndex;
    var snippet = extractSnippet(item.c, terms);
    return '<a href="' + escapeHTML(item.u) + '" id="search-result-' + index + '" class="search-result-item' + (isSelected ? ' selected' : '') + '" role="option" aria-selected="' + isSelected + '" data-index="' + index + '">' +
      '  <div class="search-result-top">' +
      '    <span class="search-result-section">' + escapeHTML(item.s) + '</span>' +
      '    <span class="search-result-sep">/</span>' +
      '    <span class="search-result-doc">' + escapeHTML(item.d) + '</span>' +
      '  </div>' +
      '  <div class="search-result-title">' +
      '    <svg class="search-result-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>' +
      '    <span>' + highlight(item.t || item.d, terms) + '</span>' +
      '  </div>' +
      (snippet ? '  <div class="search-result-snippet">' + snippet + '</div>' : '') +
      '</a>';
  }

  // announce writes the one line a reader cannot see. The set signature is what
  // keeps it quiet: a keystroke that leaves the set alone, a change of case or
  // an extra letter that still matches the same pages, has nothing new to say.
  function announce(text, key) {
    if (!status) return;
    if (key === undefined) key = text;
    if (key === announcedSet) return;
    announcedSet = key;
    status.textContent = text;
  }

  // What a reader cannot see is how many rows are reachable, and in a tree which
  // wikis they came from. The query is not repeated back: the reader just typed
  // it, and a set that has stopped moving under further keystrokes would leave a
  // stale one behind. The count is of the capped flat list, which is what a
  // reader can reach, rather than of the matches the index holds.
  function announceResults() {
    var count = currentResults.length;
    var text;
    if (count === 0) {
      text = 'No results';
    } else {
      text = count === 1 ? '1 result' : count + ' results';
      if (currentGroups) {
        var sites = [];
        for (var g = 0; g < currentGroups.length; g++) {
          if (currentGroups[g].site) sites.push(currentGroups[g].site);
        }
        if (sites.length === 1) text += ' in ' + sites[0];
        else if (sites.length > 1) text += ' in ' + sites.join(', ');
      }
    }
    var key = String(count);
    for (var i = 0; i < currentResults.length; i++) key += '|' + currentResults[i].u;
    announce(text, key);
  }

  function renderResults(terms, query) {
    if (currentResults.length === 0) {
      selectedIndex = -1;
      resultsHost.innerHTML =
        '<div class="search-empty-state">' +
        '<p>No results found for "<b>' + escapeHTML(query) + '</b>"</p>' +
        '<p class="search-empty-hint">Try a host, a command, or a topic.</p>' +
        '</div>';
      input.removeAttribute('aria-activedescendant');
      announceResults();
      return;
    }

    selectedIndex = 0;
    var html = '<div class="search-results-list" role="listbox">';
    if (currentGroups) {
      // A wiki's rows sit in their own role="group", labelled by a heading. The
      // heading is not an option, and the rows keep a flat, contiguous
      // data-index across the groups, so arrow traversal reaches every row.
      var flat = 0;
      for (var g = 0; g < currentGroups.length; g++) {
        var grp = currentGroups[g];
        html += '<div class="search-result-group" role="group" aria-labelledby="search-group-' + g + '">';
        html += '<div class="search-group-heading" id="search-group-' + g + '">' + escapeHTML(grp.site) + '</div>';
        for (var k = 0; k < grp.count; k++) {
          html += resultItemHTML(currentResults[flat], flat, terms);
          flat++;
        }
        html += '</div>';
      }
    } else {
      for (var i = 0; i < currentResults.length; i++) {
        html += resultItemHTML(currentResults[i], i, terms);
      }
    }
    html += '</div>';
    resultsHost.innerHTML = html;
    announceResults();
  }

  function updateSelection() {
    var items = resultsContainer.querySelectorAll('.search-result-item');
    for (var i = 0; i < items.length; i++) {
      if (i === selectedIndex) {
        items[i].classList.add('selected');
        items[i].setAttribute('aria-selected', 'true');
        items[i].scrollIntoView({ block: 'nearest' });
        input.setAttribute('aria-activedescendant', 'search-result-' + selectedIndex);
      } else {
        items[i].classList.remove('selected');
        items[i].setAttribute('aria-selected', 'false');
      }
    }
  }

  function moveSelection(delta) {
    if (currentResults.length === 0) return;
    selectedIndex += delta;
    if (selectedIndex < 0) selectedIndex = currentResults.length - 1;
    else if (selectedIndex >= currentResults.length) selectedIndex = 0;
    updateSelection();
  }

  function openSelected() {
    if (selectedIndex >= 0 && selectedIndex < currentResults.length) {
      window.location.href = currentResults[selectedIndex].u;
    }
  }

  function isEditable(el) {
    if (!el) return false;
    var tag = el.tagName ? el.tagName.toLowerCase() : '';
    return tag === 'input' || tag === 'textarea' || tag === 'select' || el.isContentEditable;
  }

  if (searchBtn) {
    searchBtn.addEventListener('click', openSearch);
    searchBtn.addEventListener('mouseenter', fetchIndex, { once: true });
    searchBtn.addEventListener('focus', fetchIndex, { once: true });
  }
  if (closeBtn) closeBtn.addEventListener('click', closeSearch);

  // The scope toggle is only meaningful in a tree, where the index at the tree
  // base spans more than the current wiki. A single bundle and a group index
  // leave it hidden, since their search base is their own base.
  if (scopeBtn && multiSite) {
    scopeBtn.hidden = false;
    scopeBtn.addEventListener('click', function () {
      setScope(!scopeAll);
      runSearch();
    });
  }

  backdrop.addEventListener('click', function (e) { if (e.target === backdrop) closeSearch(); });

  // Typing runs one search a short pause after the last keystroke. The key
  // listener is separate, so Escape closes at once, and the arrows and Enter
  // flush a pending search first so they never act on the previous query's rows.
  function scheduleSearch() {
    if (inputTimer) clearTimeout(inputTimer);
    // The handle is cleared when the timer fires, so flushSearch does not read
    // a search that has already run as still pending and run it again.
    inputTimer = setTimeout(function () {
      inputTimer = null;
      runSearch();
    }, 100);
  }

  function flushSearch() {
    if (!inputTimer) return;
    clearTimeout(inputTimer);
    inputTimer = null;
    runSearch();
  }

  input.addEventListener('input', scheduleSearch);
  input.addEventListener('keydown', function (e) {
    if (e.key === 'ArrowDown') { e.preventDefault(); flushSearch(); moveSelection(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); flushSearch(); moveSelection(-1); }
    else if (e.key === 'Enter') { e.preventDefault(); flushSearch(); openSelected(); }
    else if (e.key === 'Escape') { e.preventDefault(); closeSearch(); }
  });

  // A placeholder is a hint rather than a name: it disappears as soon as a
  // reader types, and is announced as one. A name the markup already carries,
  // whether an aria-label, an aria-labelledby or a label element, wins; this
  // only fills the gap left by a theme that replaces the template.
  if (!input.getAttribute('aria-label') && !input.getAttribute('aria-labelledby') &&
      !(input.labels && input.labels.length)) {
    input.setAttribute('aria-label', 'Search documentation');
  }

  // The announced counterpart of the visible rows, set politely so it never
  // interrupts a keystroke. Both children are built once, here: the rows are
  // rewritten on every keystroke, and a live region sharing their subtree would
  // speak every time it was. So the status node holds the count inside the
  // results container the theme's contract names, and nothing else.
  status = document.createElement('p');
  status.className = 'search-status';
  status.setAttribute('role', 'status');
  status.setAttribute('aria-live', 'polite');
  status.setAttribute('aria-atomic', 'true');
  resultsHost = document.createElement('div');
  resultsContainer.appendChild(status);
  resultsContainer.appendChild(resultsHost);

  // The dialog declares aria-modal, so Tab must not leave it. The candidates
  // are collected on each keypress: the result rows are rebuilt on every
  // keystroke, and a list held over from the open would focus a detached node.
  function trapTab(e) {
    if (!modal) return;
    var candidates = Array.prototype.filter.call(
      modal.querySelectorAll('button, a[href], input'),
      function (el) {
        if (el.disabled || el.hasAttribute('hidden') || el.closest('[inert]')) return false;
        return el.offsetParent !== null;
      });
    if (!candidates.length) return;
    var first = candidates[0], last = candidates[candidates.length - 1];
    var active = document.activeElement;
    var inside = modal.contains(active);
    if (e.shiftKey && (active === first || !inside)) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && (active === last || !inside)) { e.preventDefault(); first.focus(); }
  }

  document.addEventListener('keydown', function (e) {
    if ((e.key === 'k' || e.key === 'K') && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      if (backdrop.classList.contains('open')) closeSearch(); else openSearch();
      return;
    }
    if (e.key === '/' && !backdrop.classList.contains('open') && !isEditable(document.activeElement)) {
      e.preventDefault();
      openSearch();
      return;
    }
    if (e.key === 'Tab' && backdrop.classList.contains('open')) trapTab(e);
    if (e.key === 'Escape' && backdrop.classList.contains('open')) closeSearch();
  });

  resultsContainer.addEventListener('mousemove', function (e) {
    var item = e.target.closest('.search-result-item');
    if (item && item.dataset.index) {
      var idx = parseInt(item.dataset.index, 10);
      if (idx !== selectedIndex) { selectedIndex = idx; updateSelection(); }
    }
  });
})();
