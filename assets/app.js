// ─── Theme ────────────────────────────────────────────────────────────────────

(function () {
  const html = document.documentElement;
  const stored = localStorage.getItem('dyno-theme');

  function applyTheme(dark) {
    html.classList.toggle('dark', dark);
    const sun = document.getElementById('icon-sun');
    const moon = document.getElementById('icon-moon');
    if (sun) sun.classList.toggle('hidden', !dark);
    if (moon) moon.classList.toggle('hidden', dark);
  }

  // Determine initial theme
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  const isDark = stored === 'dark' || (stored === null && prefersDark);
  applyTheme(isDark);

  // Toggle button
  document.addEventListener('DOMContentLoaded', function () {
    const btn = document.getElementById('theme-toggle');
    if (btn) {
      btn.addEventListener('click', function () {
        const nowDark = html.classList.toggle('dark');
        localStorage.setItem('dyno-theme', nowDark ? 'dark' : 'light');
        applyTheme(nowDark);
        rerenderMermaid();
      });
    }

    // Listen for OS theme changes (when no stored preference)
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function (e) {
      if (!localStorage.getItem('dyno-theme')) {
        applyTheme(e.matches);
        rerenderMermaid();
      }
    });
  });
})();

// ─── Sidebar ──────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', function () {
  const sidebar = document.getElementById('sidebar');
  const overlay = document.getElementById('sidebar-overlay');
  const toggleBtn = document.getElementById('sidebar-toggle');

  function openSidebar() {
    sidebar.classList.remove('-translate-x-full');
    overlay.classList.remove('hidden');
  }

  function closeSidebar() {
    sidebar.classList.add('-translate-x-full');
    overlay.classList.add('hidden');
  }

  if (toggleBtn) toggleBtn.addEventListener('click', openSidebar);
  if (overlay) overlay.addEventListener('click', closeSidebar);

  if (!sidebar) return;
  // Delegation survives HTMX out-of-band replacements of the sidebar content.
  sidebar.addEventListener('click', function (event) {
    if (event.target.closest('.nav-link') && window.innerWidth < 1024) closeSidebar();
  });
});

// ─── Active nav link ──────────────────────────────────────────────────────────
// Uses data-active attribute so dark/light styling is handled purely in CSS.

function navCollapsedStorageKey(sidebar) {
  return 'dyno-nav-collapsed:' + (sidebar.getAttribute('data-nav-storage-key') || '/');
}

function readCollapsedNavPaths(sidebar) {
  try {
    const value = JSON.parse(localStorage.getItem(navCollapsedStorageKey(sidebar)) || '[]');
    return new Set(Array.isArray(value) ? value : []);
  } catch (_) {
    return new Set();
  }
}

function writeCollapsedNavPaths(sidebar, paths) {
  try {
    localStorage.setItem(navCollapsedStorageKey(sidebar), JSON.stringify(Array.from(paths)));
  } catch (_) {}
}

function setNavSectionExpanded(section, expanded) {
  const toggle = section.querySelector(':scope > div > [data-nav-toggle]');
  const children = section.querySelector(':scope > [data-nav-children]');
  if (!toggle || !children) return;
  toggle.setAttribute('aria-expanded', expanded ? 'true' : 'false');
  const label = section.getAttribute('data-nav-title') || 'section';
  toggle.setAttribute('aria-label', (expanded ? 'Collapse ' : 'Expand ') + label);
  children.classList.toggle('hidden', !expanded);
  const chevron = toggle.querySelector('[data-nav-chevron]');
  if (chevron) chevron.classList.toggle('rotate-90', expanded);
}

function initNavTree() {
  const sidebar = document.getElementById('sidebar');
  if (!sidebar) return;
  const collapsed = readCollapsedNavPaths(sidebar);
  sidebar.querySelectorAll('[data-nav-section]').forEach(function (section) {
    const containsActivePage = !!section.querySelector('.nav-link[data-active="true"]');
    setNavSectionExpanded(section, containsActivePage || !collapsed.has(section.dataset.navPath));
  });
}

document.addEventListener('click', function (event) {
  const toggle = event.target.closest('[data-nav-toggle]');
  if (!toggle) return;
  const section = toggle.closest('[data-nav-section]');
  const sidebar = toggle.closest('#sidebar');
  if (!section || !sidebar) return;
  const expanded = toggle.getAttribute('aria-expanded') === 'true';
  const collapsed = readCollapsedNavPaths(sidebar);
  if (expanded) collapsed.add(section.dataset.navPath);
  else collapsed.delete(section.dataset.navPath);
  writeCollapsedNavPaths(sidebar, collapsed);
  setNavSectionExpanded(section, !expanded);
});

function updateActiveNavLink(path) {
  document.querySelectorAll('.nav-page-link, .nav-section-title').forEach(function (link) {
    const linkPath = link.getAttribute('data-path') || link.getAttribute('href');
    const isActive = linkPath === path || linkPath === path + '/';
    link.dataset.active = isActive ? 'true' : 'false';
  });
}

// ─── Mermaid ──────────────────────────────────────────────────────────────────

function initMermaid() {
  if (typeof mermaid === 'undefined') return;

  const isDark = document.documentElement.classList.contains('dark');

  const themeVariables = isDark ? {
    background: '#1e293b',
    primaryColor: '#3b82f6',
    secondaryColor: '#8b5cf6',
    tertiaryColor: '#06b6d4',
    primaryTextColor: '#f1f5f9',
    secondaryTextColor: '#f1f5f9',
    tertiaryTextColor: '#f1f5f9',
    edgeLabelBackground: '#1e293b',
    // Pie chart — explicit slice colours
    pie1: '#3b82f6',
    pie2: '#8b5cf6',
    pie3: '#06b6d4',
    pie4: '#f59e0b',
    pie5: '#10b981',
    pie6: '#f43f5e',
    pieStrokeWidth: '2px',
    pieOuterStrokeWidth: '2px',
    pieSectionTextColor: '#f1f5f9',
    pieLegendTextColor: '#cbd5e1',
  } : {
    pie1: '#3b82f6',
    pie2: '#8b5cf6',
    pie3: '#06b6d4',
    pie4: '#f59e0b',
    pie5: '#10b981',
    pie6: '#f43f5e',
    pieSectionTextColor: '#ffffff',
    pieLegendTextColor: '#1e293b',
  };

  mermaid.initialize({
    startOnLoad: false,
    theme: isDark ? 'dark' : 'default',
    themeVariables,
    securityLevel: 'loose',
    pie: { textPosition: 0.75 },
  });

  // Save original source before first render, then render unprocessed nodes
  document.querySelectorAll('pre.mermaid:not([data-processed])').forEach(function (el) {
    if (!el.hasAttribute('data-mermaid-src')) {
      el.setAttribute('data-mermaid-src', el.textContent);
    }
  });

  const unrendered = document.querySelectorAll('pre.mermaid:not([data-processed])');
  if (unrendered.length > 0) {
    mermaid.run({ nodes: unrendered });
  }
}

function rerenderMermaid() {
  if (typeof mermaid === 'undefined') return;

  // Restore original source and re-render with new theme
  document.querySelectorAll('pre.mermaid').forEach(function (el) {
    const src = el.getAttribute('data-mermaid-src');
    if (src) {
      el.removeAttribute('data-processed');
      el.textContent = src;
    }
  });

  initMermaid();
}

// ─── Copy button on code blocks ──────────────────────────────────────────────

function initCopyButtons() {
  document.querySelectorAll('pre.chroma:not([data-copy-init])').forEach(function (pre) {
    pre.setAttribute('data-copy-init', '1');
    pre.style.position = 'relative';

    const btn = document.createElement('button');
    btn.className = 'copy-btn';
    btn.setAttribute('aria-label', 'Copy code');
    btn.innerHTML = '<svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>';

    btn.addEventListener('click', function () {
      const code = pre.querySelector('code') || pre;
      navigator.clipboard.writeText(code.innerText).then(function () {
        btn.innerHTML = '<svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12"/></svg>';
        btn.classList.add('copied');
        setTimeout(function () {
          btn.innerHTML = '<svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>';
          btn.classList.remove('copied');
        }, 2000);
      });
    });

    pre.appendChild(btn);
  });
}

// ─── TOC scroll spy ───────────────────────────────────────────────────────────

function initTOCScrollSpy() {
  const tocLinks = document.querySelectorAll('.toc-link');
  if (!tocLinks.length) return;

  const headings = Array.from(tocLinks).map(function (link) {
    const id = link.getAttribute('href').slice(1);
    return document.getElementById(id);
  }).filter(Boolean);

  function onScroll() {
    let active = headings[0];
    headings.forEach(function (h) {
      if (h.getBoundingClientRect().top <= 96) active = h;
    });
    if (!active) return;
    tocLinks.forEach(function (link) {
      const isActive = link.getAttribute('href') === '#' + active.id;
      link.classList.toggle('active', isActive);
    });
  }

  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll();
}

// ─── Task filters ───────────────────────────────────────────────────────────

function initTaskFilters() {
  document.querySelectorAll('[data-task-root]:not([data-task-init])').forEach(function (root) {
    root.setAttribute('data-task-init', '1');

    const input = root.querySelector('[data-task-filter]');
    const rows = Array.from(root.querySelectorAll('[data-task-row]'));
    const pageGroups = Array.from(root.querySelectorAll('[data-task-page-group]'));
    const empty = root.querySelector('.tasks-empty');

    if (!input || !rows.length) return;

    function normalizeTaskFilterText(value) {
      return (value || '')
        .toLocaleLowerCase('cs-CZ')
        .normalize('NFD')
        .replace(/[\u0300-\u036f]/g, '');
    }

    rows.forEach(function (row) {
      row.setAttribute('data-task-search-normalized', normalizeTaskFilterText(row.getAttribute('data-task-search') || ''));
    });

    function applyFilter() {
      const query = normalizeTaskFilterText(input.value.trim());
      let visibleRows = 0;

      rows.forEach(function (row) {
        const haystack = (row.getAttribute('data-task-search-normalized') || '');
        const show = !query || haystack.indexOf(query) !== -1;
        row.hidden = !show;
        if (show) visibleRows += 1;
      });

      pageGroups.forEach(function (group) {
        const hasVisible = Array.from(group.querySelectorAll('[data-task-row]')).some(function (row) {
          return !row.hidden;
        });
        group.hidden = !hasVisible;
      });

      if (empty) {
        empty.hidden = visibleRows !== 0;
      }
    }

    input.addEventListener('input', applyFilter);
    applyFilter();
  });
}

// ─── Page comments ───────────────────────────────────────────────────────────

function initComments() {
  document.querySelectorAll('[data-comments-root]:not([data-comments-init])').forEach(function (root) {
    root.setAttribute('data-comments-init', '1');

    const form = root.querySelector('[data-comments-form]');
    const selectionBox = root.querySelector('[data-comment-selection]');
    const selectionText = root.querySelector('[data-comment-selection-text]');
    const quoteInput = root.querySelector('[data-comment-quote]');
    const anchorInput = root.querySelector('[data-comment-anchor]');
    const parentInput = root.querySelector('[data-comment-parent-id]');
    const clearBtn = root.querySelector('[data-comment-clear-selection]');
    const replyBox = root.querySelector('[data-comment-reply]');
    const replyText = root.querySelector('[data-comment-reply-text]');
    const clearReplyBtn = root.querySelector('[data-comment-clear-reply]');
    const article = document.querySelector('#page-content .prose');
    const textarea = form && form.querySelector('textarea[name="body"]');
    let commentEditor = null;

    if (textarea && typeof EasyMDE !== 'undefined' && !textarea.dataset.easymdeInit) {
      textarea.dataset.easymdeInit = '1';
      commentEditor = new EasyMDE({
        element: textarea,
        autoDownloadFontAwesome: false,
        spellChecker: false,
        status: false,
        minHeight: '120px',
        maxHeight: '260px',
        forceSync: true,
        placeholder: textarea.getAttribute('placeholder') || 'Add a note here...',
        renderingConfig: {
          singleLineBreaks: true,
          codeSyntaxHighlighting: false
        }
      });
      root.dynoCommentEditor = commentEditor;
    } else if (root.dynoCommentEditor) {
      commentEditor = root.dynoCommentEditor;
    }

    function focusCommentEditor() {
      if (commentEditor && commentEditor.codemirror) {
        commentEditor.codemirror.focus();
      } else if (textarea) {
        textarea.focus();
      }
    }

    function clearSelection() {
      if (quoteInput) quoteInput.value = '';
      if (anchorInput) anchorInput.value = '';
      if (selectionText) selectionText.textContent = '';
      if (selectionBox) selectionBox.hidden = true;
    }

    function clearReply() {
      if (parentInput) parentInput.value = '';
      if (replyText) replyText.textContent = '';
      if (replyBox) replyBox.hidden = true;
    }

    function setReply(id, author) {
      if (!parentInput) return;
      parentInput.value = id || '';
      if (replyText) replyText.textContent = 'RE: ' + (author || id || 'comment');
      if (replyBox) replyBox.hidden = false;
      if (form) {
        form.scrollIntoView({ block: 'center', behavior: 'smooth' });
        focusCommentEditor();
      }
    }

    function nearestHeadingID(node) {
      var el = node && node.nodeType === Node.ELEMENT_NODE ? node : node && node.parentElement;
      while (el && el !== article) {
        var prev = el;
        while (prev) {
          if (/^H[1-6]$/.test(prev.tagName || '') && prev.id) return prev.id;
          prev = prev.previousElementSibling;
        }
        el = el.parentElement;
      }
      return '';
    }

    function captureSelection() {
      if (!article || !quoteInput || !anchorInput) return;
      const sel = window.getSelection();
      if (!sel || sel.isCollapsed || sel.rangeCount === 0) return;
      const range = sel.getRangeAt(0);
      if (!article.contains(range.commonAncestorContainer)) return;
      const text = sel.toString().trim().replace(/\s+/g, ' ');
      if (!text) return;
      const quote = text.length > 280 ? text.slice(0, 277) + '...' : text;
      quoteInput.value = quote;
      anchorInput.value = nearestHeadingID(range.startContainer);
      if (selectionText) selectionText.textContent = quote;
      if (selectionBox) selectionBox.hidden = false;
      focusCommentEditor();
    }

    if (window.dynoCommentsMouseupHandler) {
      document.removeEventListener('mouseup', window.dynoCommentsMouseupHandler);
    }
    if (window.dynoCommentsKeyupHandler) {
      document.removeEventListener('keyup', window.dynoCommentsKeyupHandler);
    }
    window.dynoCommentsMouseupHandler = captureSelection;
    window.dynoCommentsKeyupHandler = function (e) {
      if (e.key === 'Shift' || e.key.startsWith('Arrow')) captureSelection();
    };
    document.addEventListener('mouseup', window.dynoCommentsMouseupHandler);
    document.addEventListener('keyup', window.dynoCommentsKeyupHandler);
    if (clearBtn) {
      clearBtn.addEventListener('click', clearSelection);
    }
    if (clearReplyBtn) {
      clearReplyBtn.addEventListener('click', clearReply);
    }
    root.querySelectorAll('[data-comment-reply-to]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        setReply(btn.getAttribute('data-comment-reply-to'), btn.getAttribute('data-comment-reply-author'));
      });
    });
    if (form) {
      form.addEventListener('submit', function (e) {
        if (!window.fetch) return;
        e.preventDefault();
        if (commentEditor && commentEditor.codemirror) {
          commentEditor.codemirror.save();
        }
        const btn = form.querySelector('button[type="submit"]');
        if (btn) btn.disabled = true;
        fetch(form.action, {
          method: 'POST',
          body: new FormData(form),
          headers: { 'Accept': 'application/json' }
        }).then(function (res) {
          if (!res.ok) throw new Error('comment failed');
          window.location.reload();
        }).catch(function () {
          form.submit();
        }).finally(function () {
          if (btn) btn.disabled = false;
        });
      });
    }
  });
}

// ─── Search highlight on destination page ───────────────────────────────────

function clearSearchHighlights() {
  document.querySelectorAll('mark[data-search-highlight="1"]').forEach(function (mark) {
    const parent = mark.parentNode;
    if (!parent) return;
    parent.replaceChild(document.createTextNode(mark.textContent), mark);
    parent.normalize();
  });
}

function getSearchQuery() {
  const params = new URLSearchParams(window.location.search);
  return (params.get('q') || '').trim();
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function shouldSkipHighlightNode(node) {
  const parent = node.parentElement;
  if (!parent) return true;
  return Boolean(parent.closest('pre, code, script, style, svg, .mermaid, .d2-diagram, .api-widget'));
}

function applySearchHighlights() {
  clearSearchHighlights();

  const query = getSearchQuery();
  if (!query) return;

  const prose = document.querySelector('.prose');
  if (!prose) return;

  const pattern = new RegExp('(' + escapeRegExp(query) + ')', 'gi');
  const walker = document.createTreeWalker(prose, NodeFilter.SHOW_TEXT, {
    acceptNode: function (node) {
      if (!node.nodeValue || !node.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
      if (shouldSkipHighlightNode(node)) return NodeFilter.FILTER_REJECT;
      if (!pattern.test(node.nodeValue)) return NodeFilter.FILTER_REJECT;
      pattern.lastIndex = 0;
      return NodeFilter.FILTER_ACCEPT;
    }
  });

  const textNodes = [];
  while (walker.nextNode()) {
    textNodes.push(walker.currentNode);
  }

  let firstMark = null;
  textNodes.forEach(function (node) {
    const text = node.nodeValue;
    pattern.lastIndex = 0;
    if (!pattern.test(text)) return;
    pattern.lastIndex = 0;

    const fragment = document.createDocumentFragment();
    let lastIndex = 0;
    text.replace(pattern, function (match, _group, offset) {
      if (offset > lastIndex) {
        fragment.appendChild(document.createTextNode(text.slice(lastIndex, offset)));
      }
      const mark = document.createElement('mark');
      mark.setAttribute('data-search-highlight', '1');
      mark.textContent = match;
      if (!firstMark) firstMark = mark;
      fragment.appendChild(mark);
      lastIndex = offset + match.length;
      return match;
    });

    if (lastIndex < text.length) {
      fragment.appendChild(document.createTextNode(text.slice(lastIndex)));
    }
    if (node.parentNode) {
      node.parentNode.replaceChild(fragment, node);
    }
  });

  if (firstMark) {
    firstMark.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }
}

document.addEventListener('DOMContentLoaded', function () {
  initNavTree();
  initMermaid();
  updateActiveNavLink(window.location.pathname);
  initTOCScrollSpy();
  initTaskFilters();
  initComments();
  initCopyButtons();
  initLightbox();
  applySearchHighlights();
});

// ─── HTMX hooks ───────────────────────────────────────────────────────────────

document.addEventListener('htmx:afterSwap', function (e) {
  if (e.target.id === 'page-content') {
    // Update search URL from response header so search stays scoped to current book.
    var searchURL = e.detail.xhr && e.detail.xhr.getResponseHeader('X-Search-URL');
    if (searchURL) {
      var input = document.getElementById('search-input');
      if (input) input.setAttribute('hx-get', searchURL);
    }
    initMermaid();
    initTOCScrollSpy();
    initTaskFilters();
    initComments();
    initCopyButtons();
    initLightbox();
    applySearchHighlights();
    requestAnimationFrame(initNavTree);
    if (!getSearchQuery()) {
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
  }
  if (e.target.id === 'page-content' || e.target.id === 'search-results-container') {
    updateActiveNavLink(window.location.pathname);
  }
});

document.addEventListener('htmx:oobAfterSwap', function (e) {
  if (e.target && e.target.id === 'sidebar') requestAnimationFrame(initNavTree);
});

document.addEventListener('htmx:afterSettle', function () {
  // OOB content may be inserted after the main target's afterSwap callback.
  // Re-apply persisted collapse state once the complete HTMX transaction settles.
  initNavTree();
});

// ─── Search UX ────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', function () {
  const input = document.getElementById('search-input');
  const container = document.getElementById('search-results-container');

  if (!input || !container) return;

  // Show/hide results container
  input.addEventListener('input', function () {
    if (input.value.trim()) {
      container.classList.remove('hidden');
    } else {
      container.classList.add('hidden');
      container.innerHTML = '';
    }
  });

  // Hide on click outside
  document.addEventListener('click', function (e) {
    if (!input.contains(e.target) && !container.contains(e.target)) {
      container.classList.add('hidden');
    }
  });

  // Keyboard shortcut Ctrl+K / Cmd+K + arrow navigation
  document.addEventListener('keydown', function (e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
      e.preventDefault();
      input.focus();
      input.select();
      return;
    }
    if (e.key === 'Escape') {
      container.classList.add('hidden');
      input.blur();
      return;
    }

    // Arrow navigation inside results
    if (!container.classList.contains('hidden') && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault();
      const links = Array.from(container.querySelectorAll('.search-result-link'));
      if (!links.length) return;
      const active = container.querySelector('.search-result-link.ring-2');
      let idx = active ? links.indexOf(active) : -1;
      if (active) active.classList.remove('ring-2', 'ring-brand-500', 'bg-gray-50', 'dark:bg-gray-800');
      idx = e.key === 'ArrowDown' ? Math.min(idx + 1, links.length - 1) : Math.max(idx - 1, 0);
      links[idx].classList.add('ring-2', 'ring-brand-500', 'bg-gray-50', 'dark:bg-gray-800');
      links[idx].scrollIntoView({ block: 'nearest' });
      return;
    }

    // Enter — navigate to focused result
    if (e.key === 'Enter' && !container.classList.contains('hidden')) {
      const active = container.querySelector('.search-result-link.ring-2');
      if (active) {
        e.preventDefault();
        active.click();
      }
    }
  });

  // Close search results when a result link is clicked (HTMX will swap content)
  container.addEventListener('click', function (e) {
    const link = e.target.closest('.search-result-link');
    if (link) {
      container.classList.add('hidden');
      input.value = '';
    }
  });
});

// ─── Lightbox ─────────────────────────────────────────────────────────────────

function initLightbox() {
  // Create overlay once
  if (!document.getElementById('lightbox-overlay')) {
    const overlay = document.createElement('div');
    overlay.id = 'lightbox-overlay';
    overlay.innerHTML = '<img id="lightbox-img" src="" alt="">';
    document.body.appendChild(overlay);

    // Close on overlay click (outside image)
    overlay.addEventListener('click', function (e) {
      if (e.target === overlay) closeLightbox();
    });

    // Close on Escape
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') closeLightbox();
    });
  }

  // Attach to all prose images not yet initialized
  document.querySelectorAll('.prose img:not([data-lightbox-init])').forEach(function (img) {
    img.setAttribute('data-lightbox-init', '1');
    img.style.cursor = 'zoom-in';
    img.addEventListener('click', function () {
      const overlay = document.getElementById('lightbox-overlay');
      const lbImg = document.getElementById('lightbox-img');
      lbImg.src = img.src;
      lbImg.alt = img.alt;
      overlay.classList.add('open');
      document.body.style.overflow = 'hidden';
    });
  });
}

function closeLightbox() {
  const overlay = document.getElementById('lightbox-overlay');
  if (overlay) overlay.classList.remove('open');
  document.body.style.overflow = '';
}

// ── API widget ────────────────────────────────────────────────────────────

function apiToggle(widget) {
  if (!widget) return;
  const panel = widget.querySelector('.api-panel');
  const btn = widget.querySelector('.api-toggle');
  const open = panel.hasAttribute('hidden');
  if (open) {
    panel.removeAttribute('hidden');
    btn.setAttribute('aria-expanded', 'true');
    widget.classList.add('open');
  } else {
    panel.setAttribute('hidden', '');
    btn.setAttribute('aria-expanded', 'false');
    widget.classList.remove('open');
  }
}

function apiAuthTab(widget, mode, btn) {
  if (!widget) return;
  widget.querySelectorAll('.api-auth-tab').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  widget.querySelectorAll('.api-auth-panel').forEach(p => {
    p.style.display = p.dataset.auth === mode ? '' : 'none';
  });
}

function apiRespTab(widget, tab, btn) {
  if (!widget) return;
  widget.querySelectorAll('.api-resp-tab').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  widget.querySelectorAll('.api-resp-panel').forEach(p => {
    p.style.display = p.dataset.resp === tab ? '' : 'none';
  });
}

function apiSend(widget) {
  if (!widget) return;
  const id = widget.id;
  const spec = (window.__apiWidgets || {})[id];
  if (!spec) return;

  // Resolve {{var}} placeholders
  const vars = {};
  widget.querySelectorAll('[data-var]').forEach(el => {
    vars[el.dataset.var] = el.value;
  });
  function resolve(s) {
    return s.replace(/\{\{(\w+)\}\}/g, (_, k) => vars[k] || '');
  }

  let url = resolve(spec.url);
  let headers = {};

  // Static headers from the block
  (spec.headers || []).forEach(([k, v]) => { headers[resolve(k)] = resolve(v); });

  // Auth
  const activeAuth = widget.querySelector('.api-auth-tab.active');
  const authMode = activeAuth ? activeAuth.textContent.trim().toLowerCase() : 'none';
  if (authMode === 'bearer') {
    const tok = widget.querySelector('[data-role="bearer-token"]');
    if (tok && tok.value) headers['Authorization'] = 'Bearer ' + tok.value;
  } else if (authMode === 'basic') {
    const u = widget.querySelector('[data-role="basic-user"]');
    const p = widget.querySelector('[data-role="basic-pass"]');
    if (u && p) headers['Authorization'] = 'Basic ' + btoa(u.value + ':' + p.value);
  }

  // Body
  let body = '';
  const bodyEl = widget.querySelector('[data-role="body"]');
  if (bodyEl) {
    body = bodyEl.value;
    if (body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
  }

  // UI: show loading state
  const sendBtn = widget.querySelector('.api-send-btn');
  const statusBadge = widget.querySelector('[data-role="status"]');
  const responseDiv = widget.querySelector('[data-role="response"]');
  sendBtn.disabled = true;
  sendBtn.textContent = 'Sending…';
  statusBadge.className = 'api-status-badge';
  statusBadge.textContent = '';
  responseDiv.style.display = 'none';

  fetch('/api-proxy', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ method: spec.method, url, headers, body }),
  })
    .then(r => r.text())
    .then(html => {
      // The response is a <script> tag that calls applyProxyResponse()
      // We need to set up the current widget context first.
      window.__currentApiWidget = id;
      const tmp = document.createElement('div');
      tmp.innerHTML = html;
      const scripts = tmp.querySelectorAll('script');
      scripts.forEach(s => {
        const el = document.createElement('script');
        el.textContent = s.textContent;
        document.head.appendChild(el).remove();
      });
    })
    .catch(err => {
      statusBadge.textContent = 'Error';
      statusBadge.className = 'api-status-badge visible api-status-5xx';
      responseDiv.style.display = '';
      const bodyPre = widget.querySelector('[data-role="resp-body"]');
      if (bodyPre) bodyPre.textContent = err.message;
    })
    .finally(() => {
      sendBtn.disabled = false;
      sendBtn.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M5 12h14M12 5l7 7-7 7"/></svg> Send';
    });
}

function applyProxyResponse(data) {
  const id = window.__currentApiWidget;
  if (!id) return;
  const widget = document.getElementById(id);
  if (!widget) return;

  const statusBadge = widget.querySelector('[data-role="status"]');
  const responseDiv = widget.querySelector('[data-role="response"]');
  const bodyPre = widget.querySelector('[data-role="resp-body"]');
  const headersTable = widget.querySelector('[data-role="resp-headers"]');

  statusBadge.textContent = data.status + ' · ' + data.elapsed + 'ms';
  statusBadge.className = 'api-status-badge visible ' + data.statusClass;

  if (bodyPre) bodyPre.textContent = data.body;

  if (headersTable) {
    headersTable.innerHTML = (data.headers || [])
      .map(([k, v]) => `<tr><td>${escHtml(k)}</td><td>${escHtml(v)}</td></tr>`)
      .join('');
  }

  responseDiv.style.display = '';
  // Show body tab by default
  widget.querySelectorAll('.api-resp-tab').forEach(b => b.classList.remove('active'));
  const bodyTab = widget.querySelector('.api-resp-tab');
  if (bodyTab) bodyTab.classList.add('active');
  widget.querySelectorAll('.api-resp-panel').forEach(p => {
    p.style.display = p.dataset.resp === 'body' ? '' : 'none';
  });
}

function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

document.addEventListener('click', function (e) {
  const btn = e.target.closest('[data-api-action]');
  if (!btn) return;

  const widget = btn.closest('.api-widget');
  if (!widget) return;

  const action = btn.getAttribute('data-api-action');
  if (action === 'toggle') {
    apiToggle(widget);
    return;
  }
  if (action === 'auth-tab') {
    apiAuthTab(widget, btn.getAttribute('data-api-auth') || 'none', btn);
    return;
  }
  if (action === 'resp-tab') {
    apiRespTab(widget, btn.getAttribute('data-api-resp') || 'body', btn);
    return;
  }
  if (action === 'send') {
    apiSend(widget);
  }
});

// ── Ego-graph toggle ──────────────────────────────────────────────────────
document.addEventListener('click', function(e) {
  var btn = e.target.closest('.backlinks-graph-btn');
  if (!btn) return;
  var container = document.getElementById('ego-graph-container');
  if (!container) return;
  if (!container.hidden) {
    container.hidden = true;
    container.innerHTML = '';
    return;
  }
  var url = btn.getAttribute('data-ego-url');
  if (!url) return;
  container.hidden = false;
  container.innerHTML = '<div style="padding:1rem;color:#9ca3af;font-size:0.8rem">Loading graph...</div>';
  fetch(url).then(function(r) { return r.text(); }).then(function(html) {
    container.innerHTML = html;
  }).catch(function() {
    container.innerHTML = '<div style="padding:1rem;color:#ef4444;font-size:0.8rem">Failed to load graph.</div>';
  });
});

// ── Canvas site graph ─────────────────────────────────────────────────────
function initSiteGraph(container) {
  var dataUrl = container.getAttribute('data-graph-data-url');
  if (!dataUrl) return;

  var dark = document.documentElement.classList.contains('dark');
  var bg = dark ? '#111827' : '#ffffff';
  var nodeFill = dark ? '#1e3a5f' : '#dbeafe';
  var nodeStroke = dark ? '#3b82f6' : '#2563eb';
  var nodeText = dark ? '#93c5fd' : '#1d4ed8';
  var edgeColor = dark ? '#374151' : '#d1d5db';

  container.innerHTML = '<canvas id="site-graph-canvas" style="width:100%;height:600px;display:block;border-radius:0.5rem;background:' + bg + '"></canvas>';
  var canvas = document.getElementById('site-graph-canvas');
  var W = canvas.offsetWidth; var H = 600;
  canvas.width = W; canvas.height = H;
  var ctx = canvas.getContext('2d');

  fetch(dataUrl).then(function(r) { return r.json(); }).then(function(data) {
    var nodes = data.nodes || [];
    var edges = data.edges || [];
    if (nodes.length === 0) {
      ctx.fillStyle = nodeText;
      ctx.font = '14px sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText('No pages found.', W/2, H/2);
      return;
    }

    // Assign random initial positions
    var pos = {};
    nodes.forEach(function(n) {
      pos[n.id] = { x: Math.random() * (W - 80) + 40, y: Math.random() * (H - 80) + 40, vx: 0, vy: 0 };
    });

    // Build adjacency for degree (node size)
    var degree = {};
    nodes.forEach(function(n) { degree[n.id] = 0; });
    edges.forEach(function(e) {
      degree[e.source] = (degree[e.source] || 0) + 1;
      degree[e.target] = (degree[e.target] || 0) + 1;
    });

    // Force-directed simulation
    var alpha = 1.0;
    var REPEL = 1800, ATTRACT = 0.04, CENTER = 0.008, DAMPING = 0.85;

    function tick() {
      if (alpha < 0.005) return;
      alpha *= 0.97;

      // Repulsion between all pairs (O(n²) — ok for <300 nodes)
      for (var i = 0; i < nodes.length; i++) {
        for (var j = i + 1; j < nodes.length; j++) {
          var a = pos[nodes[i].id], b = pos[nodes[j].id];
          var dx = b.x - a.x, dy = b.y - a.y;
          var d2 = dx*dx + dy*dy + 1;
          var f = REPEL / d2;
          a.vx -= f * dx; a.vy -= f * dy;
          b.vx += f * dx; b.vy += f * dy;
        }
      }
      // Attraction along edges
      edges.forEach(function(e) {
        var a = pos[e.source], b = pos[e.target];
        if (!a || !b) return;
        var dx = b.x - a.x, dy = b.y - a.y;
        a.vx += dx * ATTRACT; a.vy += dy * ATTRACT;
        b.vx -= dx * ATTRACT; b.vy -= dy * ATTRACT;
      });
      // Center gravity
      nodes.forEach(function(n) {
        var p = pos[n.id];
        p.vx += (W/2 - p.x) * CENTER;
        p.vy += (H/2 - p.y) * CENTER;
        p.vx *= DAMPING; p.vy *= DAMPING;
        p.x = Math.max(20, Math.min(W-20, p.x + p.vx));
        p.y = Math.max(20, Math.min(H-20, p.y + p.vy));
      });
    }

    function draw() {
      ctx.clearRect(0, 0, W, H);
      ctx.fillStyle = bg;
      ctx.fillRect(0, 0, W, H);

      // Edges
      ctx.strokeStyle = edgeColor;
      ctx.lineWidth = 1;
      edges.forEach(function(e) {
        var a = pos[e.source], b = pos[e.target];
        if (!a || !b) return;
        ctx.beginPath();
        ctx.moveTo(a.x, a.y);
        ctx.lineTo(b.x, b.y);
        ctx.stroke();
      });

      // Nodes
      nodes.forEach(function(n) {
        var p = pos[n.id];
        var r = Math.min(5 + (degree[n.id] || 0), 14);
        ctx.beginPath();
        ctx.arc(p.x, p.y, r, 0, 2*Math.PI);
        ctx.fillStyle = nodeFill;
        ctx.fill();
        ctx.strokeStyle = nodeStroke;
        ctx.lineWidth = 1.5;
        ctx.stroke();
      });

      // Labels for high-degree nodes only
      ctx.font = '10px sans-serif';
      ctx.textAlign = 'center';
      nodes.forEach(function(n) {
        if ((degree[n.id] || 0) < 2) return;
        var p = pos[n.id];
        ctx.fillStyle = nodeText;
        var label = n.label.length > 20 ? n.label.slice(0, 18) + '…' : n.label;
        ctx.fillText(label, p.x, p.y - Math.min(5 + (degree[n.id] || 0), 14) - 3);
      });
    }

    var raf;
    function loop() {
      tick(); draw();
      if (alpha >= 0.005) raf = requestAnimationFrame(loop);
    }
    loop();

    // Click to navigate
    canvas.addEventListener('click', function(e) {
      var rect = canvas.getBoundingClientRect();
      var mx = (e.clientX - rect.left) * (W / rect.width);
      var my = (e.clientY - rect.top) * (H / rect.height);
      for (var i = 0; i < nodes.length; i++) {
        var n = nodes[i]; var p = pos[n.id];
        var r = Math.min(5 + (degree[n.id] || 0), 14) + 4;
        var dx = mx - p.x, dy = my - p.y;
        if (dx*dx + dy*dy <= r*r) {
          window.location.href = n.url;
          return;
        }
      }
    });

    // Tooltip
    var tooltip = document.createElement('div');
    tooltip.style.cssText = 'position:fixed;background:#1f2937;color:#f9fafb;padding:4px 8px;border-radius:4px;font-size:12px;pointer-events:none;display:none;z-index:9999';
    document.body.appendChild(tooltip);
    canvas.addEventListener('mousemove', function(e) {
      var rect = canvas.getBoundingClientRect();
      var mx = (e.clientX - rect.left) * (W / rect.width);
      var my = (e.clientY - rect.top) * (H / rect.height);
      var found = false;
      for (var i = 0; i < nodes.length; i++) {
        var n = nodes[i]; var p = pos[n.id];
        var r = Math.min(5 + (degree[n.id] || 0), 14) + 4;
        if ((mx-p.x)*(mx-p.x)+(my-p.y)*(my-p.y) <= r*r) {
          tooltip.style.display = 'block';
          tooltip.style.left = (e.clientX + 12) + 'px';
          tooltip.style.top = (e.clientY - 8) + 'px';
          tooltip.textContent = n.label;
          canvas.style.cursor = 'pointer';
          found = true; break;
        }
      }
      if (!found) { tooltip.style.display = 'none'; canvas.style.cursor = 'default'; }
    });
    canvas.addEventListener('mouseleave', function() { tooltip.style.display = 'none'; });
  }).catch(function() {
    ctx.fillStyle = '#ef4444';
    ctx.font = '14px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText('Failed to load graph data.', W/2, H/2);
  });
}

function initGraphPages() {
  var el = document.querySelector('.graph-page[data-graph-data-url]');
  if (el) initSiteGraph(el);
}

document.addEventListener('DOMContentLoaded', initGraphPages);
document.addEventListener('htmx:afterSwap', initGraphPages);
