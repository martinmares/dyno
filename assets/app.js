// ─── Theme ────────────────────────────────────────────────────────────────────

(function () {
  const html = document.documentElement;
  const stored = localStorage.getItem('dyno-theme');

  function applyTheme(dark) {
    html.classList.toggle('dark', dark);
    html.setAttribute('data-bs-theme', dark ? 'dark' : 'light');
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
        const nowDark = !html.classList.contains('dark');
        localStorage.setItem('dyno-theme', nowDark ? 'dark' : 'light');
        applyTheme(nowDark);
        rerenderMermaid();
        rerenderSiteGraphs();
      });
    }

    // Listen for OS theme changes (when no stored preference)
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function (e) {
      if (!localStorage.getItem('dyno-theme')) {
        applyTheme(e.matches);
        rerenderMermaid();
        rerenderSiteGraphs();
      }
    });
  });
})();

// ─── Page text size ──────────────────────────────────────────────────────────

(function () {
  const storageKey = 'dyno-font-scale';
  const minScale = 0.8;
  const maxScale = 1.5;
  const step = 0.1;
  const defaultScale = 1;
  let scale = defaultScale;

  function clampScale(value) {
    return Math.round(Math.max(minScale, Math.min(maxScale, value)) * 10) / 10;
  }

  function readStoredScale() {
    try {
      const stored = sessionStorage.getItem(storageKey);
      if (stored === null || stored.trim() === '') return defaultScale;
      const value = Number(stored);
      return Number.isFinite(value) ? clampScale(value) : defaultScale;
    } catch (_) {
      return defaultScale;
    }
  }

  function setRootFontSize(value) {
    document.documentElement.style.fontSize = Math.round(value * 100) + '%';
  }

  scale = readStoredScale();
  setRootFontSize(scale);

  function applyFontScale(value) {
    scale = clampScale(value);

    const percent = Math.round(scale * 100) + '%';
    document.documentElement.style.fontSize = percent;
    try {
      sessionStorage.setItem(storageKey, String(scale));
    } catch (_) {}
    const decrease = document.getElementById('font-size-decrease');
    const reset = document.getElementById('font-size-reset');
    const increase = document.getElementById('font-size-increase');
    if (decrease) {
      decrease.disabled = scale <= minScale;
      decrease.title = 'Decrease text size (currently ' + percent + ')';
    }
    if (reset) {
      reset.disabled = scale === defaultScale;
      reset.title = 'Reset text size (currently ' + percent + ')';
      reset.setAttribute('aria-label', 'Reset text size (currently ' + percent + ')');
    }
    if (increase) {
      increase.disabled = scale >= maxScale;
      increase.title = 'Increase text size (currently ' + percent + ')';
    }
  }

  function initFontSizeControls() {
    const decrease = document.getElementById('font-size-decrease');
    const reset = document.getElementById('font-size-reset');
    const increase = document.getElementById('font-size-increase');
    if (!decrease || !reset || !increase || decrease.dataset.fontSizeInit) return;

    decrease.dataset.fontSizeInit = '1';
    decrease.addEventListener('click', function () { applyFontScale(scale - step); });
    reset.addEventListener('click', function () { applyFontScale(defaultScale); });
    increase.addEventListener('click', function () { applyFontScale(scale + step); });
    applyFontScale(scale);
  }

  document.addEventListener('DOMContentLoaded', initFontSizeControls);
})();

// ─── Sidebar ──────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', function () {
  const sidebar = document.getElementById('sidebar');
  const overlay = document.getElementById('sidebar-overlay');
  const toggleBtn = document.getElementById('sidebar-toggle');

  function openSidebar() {
    sidebar.classList.add('is-open');
    overlay.classList.add('is-open');
  }

  function closeSidebar() {
    sidebar.classList.remove('is-open');
    overlay.classList.remove('is-open');
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
  children.hidden = !expanded;
  const chevron = toggle.querySelector('[data-nav-chevron]');
  if (chevron) chevron.classList.toggle('is-expanded', expanded);
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
  const expandAll = event.target.closest('[data-nav-expand-all]');
  if (expandAll) {
    const sidebar = expandAll.closest('#sidebar');
    if (!sidebar) return;
    const collapsed = new Set();
    writeCollapsedNavPaths(sidebar, collapsed);
    sidebar.querySelectorAll('[data-nav-section]').forEach(function (section) {
      setNavSectionExpanded(section, true);
    });
    return;
  }

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

function dynoThemeColor(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

function initMermaid() {
  if (typeof mermaid === 'undefined') return;

  const colors = {
    background: dynoThemeColor('--tblr-body-bg'),
    primary: dynoThemeColor('--tblr-primary'),
    indigo: dynoThemeColor('--tblr-indigo'),
    info: dynoThemeColor('--tblr-info'),
    warning: dynoThemeColor('--tblr-warning'),
    success: dynoThemeColor('--tblr-success'),
    danger: dynoThemeColor('--tblr-danger'),
    surface: dynoThemeColor('--tblr-bg-surface'),
    text: dynoThemeColor('--tblr-body-color'),
    muted: dynoThemeColor('--tblr-secondary-color'),
  };

  const themeVariables = {
    background: colors.background,
    primaryColor: colors.primary,
    secondaryColor: colors.indigo,
    tertiaryColor: colors.info,
    primaryTextColor: colors.text,
    secondaryTextColor: colors.text,
    tertiaryTextColor: colors.text,
    edgeLabelBackground: colors.surface,
    pie1: colors.primary,
    pie2: colors.indigo,
    pie3: colors.info,
    pie4: colors.warning,
    pie5: colors.success,
    pie6: colors.danger,
    pieStrokeWidth: '2px',
    pieOuterStrokeWidth: '2px',
    pieSectionTextColor: colors.text,
    pieLegendTextColor: colors.muted,
  };

  mermaid.initialize({
    startOnLoad: false,
    theme: 'base',
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

// ─── Interactive Markdown tables ─────────────────────────────────────────────

function initTableWidgets() {
  document.querySelectorAll('.dyno-table-widget:not([data-table-init])').forEach(function (widget) {
    widget.setAttribute('data-table-init', '1');

    const table = widget.querySelector('table');
    const body = table && table.tBodies[0];
    const headerRow = table && table.tHead && table.tHead.rows[0];
    if (!table || !body || !headerRow) return;

    const rows = Array.from(body.rows);
    const headerCells = Array.from(headerRow.cells);
    const sortable = widget.dataset.tableSortable === 'true';
    const filterable = widget.dataset.tableFilter === 'true';
    const input = widget.querySelector('.dyno-table-filter');
    const status = widget.querySelector('.dyno-table-status');
    const empty = widget.querySelector('.dyno-table-empty');
    const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });
    let sortColumn = -1;
    let sortDirection = 1;

    function normalizeFilterText(value) {
      return String(value || '')
        .toLocaleLowerCase('cs-CZ')
        .normalize('NFD')
        .replace(/[\u0300-\u036f]/g, '');
    }

    function clearTableHighlights() {
      table.querySelectorAll('mark.dyno-table-match').forEach(function (mark) {
        mark.replaceWith(document.createTextNode(mark.textContent));
      });
      table.normalize();
    }

    function normalizedTextWithMap(value) {
      let normalized = '';
      const map = [];
      for (let index = 0; index < value.length;) {
        const codePoint = value.codePointAt(index);
        const character = String.fromCodePoint(codePoint);
        const normalizedCharacter = normalizeFilterText(character);
        for (let offset = 0; offset < normalizedCharacter.length; offset += 1) {
          map.push({ start: index, end: index + character.length });
        }
        normalized += normalizedCharacter;
        index += character.length;
      }
      return { value: normalized, map: map };
    }

    function highlightRow(row, query) {
      const walker = document.createTreeWalker(row, NodeFilter.SHOW_TEXT);
      const textNodes = [];
      let node = walker.nextNode();
      while (node) {
        textNodes.push(node);
        node = walker.nextNode();
      }

      textNodes.forEach(function (textNode) {
        const text = textNode.nodeValue || '';
        const normalized = normalizedTextWithMap(text);
        if (!normalized.value || !normalized.map.length) return;

        let searchFrom = 0;
        let matchStart = normalized.value.indexOf(query, searchFrom);
        if (matchStart === -1) return;

        const fragment = document.createDocumentFragment();
        let originalFrom = 0;
        while (matchStart !== -1) {
          const first = normalized.map[matchStart];
          const last = normalized.map[matchStart + query.length - 1];
          if (!first || !last) break;
          const originalTo = last.end;
          fragment.appendChild(document.createTextNode(text.slice(originalFrom, first.start)));
          const mark = document.createElement('mark');
          mark.className = 'dyno-table-match';
          mark.textContent = text.slice(first.start, originalTo);
          fragment.appendChild(mark);
          originalFrom = originalTo;
          searchFrom = matchStart + query.length;
          matchStart = normalized.value.indexOf(query, searchFrom);
        }
        fragment.appendChild(document.createTextNode(text.slice(originalFrom)));
        textNode.replaceWith(fragment);
      });
    }

    function updateSortIndicators() {
      if (!sortable) return;
      headerCells.forEach(function (cell, index) {
        cell.setAttribute('aria-sort', index === sortColumn
          ? (sortDirection === 1 ? 'ascending' : 'descending')
          : 'none');
      });
    }

    function applyFilter() {
      const query = filterable && input ? normalizeFilterText(input.value.trim()) : '';
      let visibleRows = 0;

      clearTableHighlights();
      rows.forEach(function (row) {
        const show = !query || normalizeFilterText(row.textContent).indexOf(query) !== -1;
        row.hidden = !show;
        if (show) {
          visibleRows += 1;
          if (query) highlightRow(row, query);
        }
      });

      if (empty) empty.hidden = visibleRows !== 0;
      if (status) {
        status.textContent = query
          ? visibleRows + ' of ' + rows.length + ' rows'
          : rows.length + (rows.length === 1 ? ' row' : ' rows');
      }
    }

    function sortByColumn(index) {
      if (sortColumn === index) {
        sortDirection *= -1;
      } else {
        sortColumn = index;
        sortDirection = 1;
      }

      const currentOrder = new Map(rows.map(function (row, position) {
        return [row, position];
      }));
      rows.sort(function (left, right) {
        const comparison = collator.compare(
          (left.cells[index] && left.cells[index].textContent.trim()) || '',
          (right.cells[index] && right.cells[index].textContent.trim()) || ''
        );
        return comparison * sortDirection || currentOrder.get(left) - currentOrder.get(right);
      });
      rows.forEach(function (row) { body.appendChild(row); });
      updateSortIndicators();
      applyFilter();
    }

    if (sortable) {
      headerCells.forEach(function (cell, index) {
        cell.setAttribute('scope', 'col');
        cell.setAttribute('aria-sort', 'none');

        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'dyno-table-sort';
        button.setAttribute('aria-label', 'Sort by ' + cell.textContent.trim());
        while (cell.firstChild) button.appendChild(cell.firstChild);
        cell.appendChild(button);
        button.addEventListener('click', function () { sortByColumn(index); });
      });
    }

    if (filterable && input) input.addEventListener('input', applyFilter);
    updateSortIndicators();
    applyFilter();
  });
}

// ─── Page comments ───────────────────────────────────────────────────────────

function initComments() {
  document.querySelectorAll('[data-comments-root]:not([data-comments-init])').forEach(function (root) {
    root.setAttribute('data-comments-init', '1');
    const form = root.querySelector('[data-comments-form]');
    const actionDialog = root.querySelector('[data-selection-actions-dialog]');
    const editorDialog = root.querySelector('[data-comment-editor-dialog]');
    const deleteDialog = root.querySelector('[data-delete-dialog]');
    const deleteCancel = root.querySelector('[data-delete-cancel]');
    const deleteConfirm = root.querySelector('[data-delete-confirm]');
    const preview = root.querySelector('[data-selection-preview]');
    const selectionBox = root.querySelector('[data-comment-selection]');
    const selectionText = root.querySelector('[data-comment-selection-text]');
    const quoteInput = root.querySelector('[data-comment-quote]');
    const anchorInput = root.querySelector('[data-comment-anchor]');
    const parentInput = root.querySelector('[data-comment-parent-id]');
    const authorInput = form && form.querySelector('input[name="author"]');
    const clearBtn = root.querySelector('[data-comment-clear-selection]');
    const replyBox = root.querySelector('[data-comment-reply]');
    const replyText = root.querySelector('[data-comment-reply-text]');
    const clearReplyBtn = root.querySelector('[data-comment-clear-reply]');
    const article = document.querySelector('#page-content .dyno-prose');
    const textarea = form && form.querySelector('textarea[name="body"]');
    let commentEditor = null;
    let selectedRange = null;
    let selectedText = '';
    let selectedQuote = '';
    let selectedAnchor = '';
    let editingID = '';
    let editingKind = '';
    let pendingDeleteID = '';

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

    function openEditor() {
      if (!editorDialog) return;
      editorDialog.showModal();
      if (commentEditor && commentEditor.codemirror) commentEditor.codemirror.refresh();
      setTimeout(focusCommentEditor, 0);
    }

    function closeEditor() {
      if (editorDialog && editorDialog.open) editorDialog.close();
    }

    function clearSelection() {
      selectedRange = null;
      selectedText = '';
      selectedQuote = '';
      selectedAnchor = '';
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
        openEditor();
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
      if (root.contains(range.commonAncestorContainer)) return;
      const text = sel.toString().trim();
      if (!text) return;
      selectedRange = range.cloneRange();
      selectedText = text;
      selectedQuote = text.length > 500 ? text.slice(0, 500) : text;
      selectedAnchor = nearestHeadingID(range.startContainer);
      if (preview) preview.textContent = selectedQuote;
      if (actionDialog && !actionDialog.open) actionDialog.showModal();
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
    root.querySelectorAll('[data-comment-editor-cancel]').forEach(function (btn) {
      btn.addEventListener('click', closeEditor);
    });
    const newComment = root.querySelector('[data-comment-new]');
    if (newComment) newComment.addEventListener('click', function () {
      editingID = ''; editingKind = ''; clearSelection(); clearReply();
      if (authorInput) authorInput.value = '';
      if (commentEditor) commentEditor.value('');
      openEditor();
    });
    const actionCancel = root.querySelector('[data-selection-cancel]');
    if (actionCancel) actionCancel.addEventListener('click', function () { actionDialog.close(); editingID = ''; editingKind = ''; });
    const actionComment = root.querySelector('[data-selection-comment]');
    if (actionComment) actionComment.addEventListener('click', function () {
      quoteInput.value = selectedQuote;
      anchorInput.value = selectedAnchor;
      if (selectionText) selectionText.textContent = selectedQuote;
      if (selectionBox) selectionBox.hidden = false;
      actionDialog.close();
      openEditor();
    });
    const actionHighlight = root.querySelector('[data-selection-highlight]');
    if (actionHighlight) actionHighlight.addEventListener('click', function () {
      const data = new FormData();
      data.set('page_path', root.dataset.pagePath || '');
      data.set('kind', 'highlight');
      data.set('quote', selectedQuote);
      data.set('anchor', selectedAnchor);
      actionHighlight.disabled = true;
      const method = editingKind === 'highlight' && editingID ? 'PATCH' : 'POST';
      const endpoint = method === 'PATCH' ? annotationURL(root, editingID) : form.action;
      fetch(endpoint, { method: method, body: data, headers: { 'Accept': 'application/json' } })
        .then(function (res) { if (!res.ok) throw new Error('highlight failed'); return res.json(); })
        .then(function (highlight) {
          if (editingID) removeHighlightMarks(editingID);
          if (selectedRange) markCommentRange(selectedRange, highlight.id);
          if (!editingID) appendSavedHighlight(root, highlight);
          actionDialog.close();
          if (editingID) {
            const row = root.querySelector('[data-highlight-row="' + CSS.escape(editingID) + '"] p');
            if (row) { row.textContent = highlight.quote; row.title = highlight.quote; }
          }
          editingID = ''; editingKind = '';
        })
        .finally(function () { actionHighlight.disabled = false; });
    });
    const actionCopy = root.querySelector('[data-selection-copy]');
    if (actionCopy) actionCopy.addEventListener('click', function () {
      if (!selectedText) return;
      actionCopy.disabled = true;
      copyTextToClipboard(selectedText)
        .then(function () {
          actionDialog.close();
        })
        .catch(function () {
          actionCopy.textContent = 'Copy failed';
        })
        .finally(function () {
          setTimeout(function () {
            actionCopy.disabled = false;
            actionCopy.textContent = 'Copy to clipboard';
          }, 600);
        });
    });
    root.addEventListener('click', function (event) {
      const reply = event.target.closest('[data-comment-reply-to]');
      if (reply) {
        setReply(reply.getAttribute('data-comment-reply-to'), reply.getAttribute('data-comment-reply-author'));
        return;
      }
      const go = event.target.closest('[data-highlight-go]');
      if (go) {
        const mark = document.querySelector('[data-highlight-mark="' + CSS.escape(go.dataset.highlightGo) + '"]');
        if (mark) mark.scrollIntoView({ block: 'center', behavior: 'smooth' });
        return;
      }
      const edit = event.target.closest('[data-annotation-edit]');
      if (edit) {
        editingID = edit.dataset.annotationEdit || '';
        editingKind = edit.dataset.annotationKind || '';
        if (editingKind === 'highlight') {
          const mark = document.querySelector('[data-highlight-mark="' + CSS.escape(editingID) + '"]');
          if (mark) mark.scrollIntoView({ block: 'center', behavior: 'smooth' });
          edit.textContent = 'Select new text…';
          return;
        }
        clearSelection(); clearReply();
        if (commentEditor) commentEditor.value(edit.dataset.annotationBody || '');
        if (authorInput) authorInput.value = edit.dataset.annotationAuthor || '';
        openEditor();
        return;
      }
      const remove = event.target.closest('[data-annotation-delete]');
      if (remove) {
        pendingDeleteID = remove.dataset.annotationDelete || '';
        if (pendingDeleteID && deleteDialog) deleteDialog.showModal();
      }
    });
    if (deleteCancel) deleteCancel.addEventListener('click', function () {
      pendingDeleteID = '';
      deleteDialog.close();
    });
    if (deleteConfirm) deleteConfirm.addEventListener('click', function () {
      const id = pendingDeleteID;
      if (!id) return;
      deleteConfirm.disabled = true;
      fetch(annotationURL(root, id), { method: 'DELETE' }).then(function (res) {
          if (!res.ok) throw new Error('delete failed');
          const comment = document.getElementById('comment-' + id);
          const highlight = root.querySelector('[data-highlight-row="' + CSS.escape(id) + '"]');
          if (comment) comment.remove();
          if (highlight) highlight.remove();
          removeHighlightMarks(id);
          const count = root.querySelector(highlight ? '[data-highlights-count]' : '[data-comments-count]');
          if (count) count.textContent = String(Math.max(0, (parseInt(count.textContent, 10) || 0) - 1));
          pendingDeleteID = '';
          deleteDialog.close();
        }).finally(function () { deleteConfirm.disabled = false; });
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
        const method = editingKind === 'comment' && editingID ? 'PATCH' : 'POST';
        const endpoint = method === 'PATCH' ? annotationURL(root, editingID) : form.action;
        fetch(endpoint, {
	          method: method,
          body: new FormData(form),
          headers: { 'Accept': 'application/json' }
        }).then(function (res) {
          if (!res.ok) throw new Error('comment failed');
          return res.json();
        }).then(function (comment) {
              if (method === 'PATCH') {
            const target = document.getElementById('comment-' + CSS.escape(editingID));
            if (target && comment.comment_html) {
              target.outerHTML = comment.comment_html;
            } else if (target) {
              const body = target.querySelector('.dyno-prose');
              if (body) body.innerHTML = comment.body_html || '';
            }
          } else {
            appendSavedComment(root, comment);
          }
          form.reset();
          if (commentEditor) commentEditor.value('');
          clearSelection(); clearReply(); closeEditor(); editingID = ''; editingKind = '';
        }).catch(function () {
          if (btn) btn.textContent = 'Save failed';
        }).finally(function () {
          if (btn) btn.disabled = false;
        });
      });
    }
  });
}

function copyTextToClipboard(text) {
  if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
    return navigator.clipboard.writeText(text).catch(function () {
      return copyTextWithLegacyFallback(text);
    });
  }
  return copyTextWithLegacyFallback(text);
}

function copyTextWithLegacyFallback(text) {
  return new Promise(function (resolve, reject) {
    const copyArea = document.createElement('textarea');
    copyArea.value = text;
    copyArea.setAttribute('readonly', '');
    copyArea.style.position = 'fixed';
    copyArea.style.opacity = '0';
    document.body.appendChild(copyArea);
    copyArea.select();
    let copied = false;
    try {
      copied = document.execCommand('copy');
    } catch (_) {
      copied = false;
    }
    copyArea.remove();
    if (copied) resolve();
    else reject(new Error('clipboard unavailable'));
  });
}

function annotationURL(root, id) {
  return (root.dataset.commentsUrl || '') + '/' + encodeURIComponent(id) + '?page_path=' + encodeURIComponent(root.dataset.pagePath || '');
}

function removeHighlightMarks(id) {
  document.querySelectorAll('[data-highlight-mark="' + CSS.escape(id) + '"]').forEach(function (mark) {
    const parent = mark.parentNode;
    if (!parent) return;
    parent.replaceChild(document.createTextNode(mark.textContent), mark);
    parent.normalize();
  });
}

function markCommentRange(range, highlightID) {
  const nodes = [];
  const walker = document.createTreeWalker(range.commonAncestorContainer, NodeFilter.SHOW_TEXT);
  while (walker.nextNode()) if (range.intersectsNode(walker.currentNode)) nodes.push(walker.currentNode);
  if (range.commonAncestorContainer.nodeType === Node.TEXT_NODE) nodes.push(range.commonAncestorContainer);
  nodes.reverse().forEach(function (node) {
    const start = node === range.startContainer ? range.startOffset : 0;
    const end = node === range.endContainer ? range.endOffset : node.nodeValue.length;
    if (end <= start) return;
    const middle = node.splitText(start);
    middle.splitText(end - start);
    const mark = document.createElement('mark');
    mark.className = 'comment-highlight';
    if (highlightID) mark.dataset.highlightMark = highlightID;
    middle.parentNode.replaceChild(mark, middle);
    mark.appendChild(middle);
  });
}

function applyCommentHighlights() {
  const article = document.querySelector('#page-content .dyno-prose');
  if (!article) return;
  document.querySelectorAll('[data-comment-highlight]').forEach(function (item) {
    const quote = item.dataset.quote || '';
    if (!quote) return;
    const walker = document.createTreeWalker(article, NodeFilter.SHOW_TEXT);
    const nodes = [];
    let text = '';
    while (walker.nextNode()) {
      nodes.push({ node: walker.currentNode, start: text.length });
      text += walker.currentNode.nodeValue;
    }
    const index = text.indexOf(quote);
    if (index < 0) return;
    const endIndex = index + quote.length;
    const startPart = nodes.find(function (part) { return index >= part.start && index <= part.start + part.node.nodeValue.length; });
    const endPart = nodes.find(function (part) { return endIndex >= part.start && endIndex <= part.start + part.node.nodeValue.length; });
    if (!startPart || !endPart) return;
    const range = document.createRange();
    range.setStart(startPart.node, index - startPart.start);
    range.setEnd(endPart.node, endIndex - endPart.start);
    markCommentRange(range, item.dataset.highlightId || '');
  });
}

function appendSavedComment(root, comment) {
  const html = comment.comment_html || '';
  if (!html) return;
  const parser = document.createElement('template');
  parser.innerHTML = html.trim();
  const item = parser.content.firstElementChild;
  if (!item) return;
  const list = root.querySelector('[data-comments-list]');
  if (!list) return;
  const parentID = comment.parent_id || '';
  if (parentID) {
    const parent = root.querySelector('#comment-' + CSS.escape(parentID));
    if (parent) {
      let childList = parent.querySelector('[data-comment-children]');
      if (!childList) {
        childList = document.createElement('div');
        childList.className = 'dyno-comment-children';
        childList.setAttribute('data-comment-children', '');
        parent.appendChild(childList);
      }
      childList.appendChild(item);
    } else {
      list.appendChild(item);
    }
  } else {
    const empty = list.querySelector('p');
    if (empty) empty.remove();
    list.appendChild(item);
  }
  const count = root.querySelector('[data-comments-count]');
  if (count) count.textContent = String((parseInt(count.textContent, 10) || 0) + 1);
}

function appendSavedHighlight(root, highlight) {
  const list = root.querySelector('[data-highlights-list]');
  if (!list) return;
  const empty = list.querySelector('p');
  if (empty) empty.remove();
  const row = document.createElement('article');
  row.className = 'dyno-highlight-row';
  row.dataset.highlightRow = highlight.id;
  const quote = document.createElement('p');
  quote.className = 'dyno-highlight-quote';
  quote.textContent = highlight.quote; quote.title = highlight.quote;
  const actions = document.createElement('div'); actions.className = 'btn-list flex-nowrap';
  actions.innerHTML = '<button type="button" data-highlight-go="' + highlight.id + '" class="btn btn-sm btn-ghost-primary">Go to</button>';
  if (root.dataset.commentsManagement === 'true') {
    actions.innerHTML += '<button type="button" data-annotation-delete="' + highlight.id + '" class="btn btn-sm btn-ghost-danger">Delete</button>';
  }
  row.append(quote, actions); list.appendChild(row);
  const count = root.querySelector('[data-highlights-count]');
  if (count) count.textContent = String((parseInt(count.textContent, 10) || 0) + 1);
}

// ─── Bulk frontmatter editor ─────────────────────────────────────────────────

function initBulkFrontmatterEditor() {
  if (window.dynoBulkFrontmatterInit) return;
  window.dynoBulkFrontmatterInit = true;

  async function openBulkEditor(url) {
    if (!url) return;
    const existing = document.querySelector('[data-bulk-frontmatter-dialog]');
    if (existing) existing.remove();
    const response = await fetch(url, { headers: { Accept: 'text/html' } });
    if (!response.ok) {
      throw new Error('Failed to load bulk editor.');
    }
    const html = await response.text();
    const tpl = document.createElement('template');
    tpl.innerHTML = html.trim();
    const dialog = tpl.content.querySelector('[data-bulk-frontmatter-dialog]');
    if (!dialog) throw new Error('Bulk editor markup missing.');
    document.body.appendChild(dialog);
    bindBulkEditor(dialog);
    dialog.showModal();
  }

  function bindBulkEditor(dialog) {
    if (dialog.dataset.bulkInit === '1') return;
    dialog.dataset.bulkInit = '1';
    const rowsRoot = dialog.querySelector('[data-bulk-frontmatter-rows]');
    const status = dialog.querySelector('[data-bulk-frontmatter-status]');
    const saveBtn = dialog.querySelector('[data-bulk-frontmatter-save]');
    const closeButtons = dialog.querySelectorAll('[data-bulk-frontmatter-close]');
    const fillBtn = dialog.querySelector('[data-bulk-frontmatter-fill]');

    function setStatus(message, kind) {
      if (!status) return;
      status.textContent = message || '';
      status.className = 'small mb-0 mt-1';
      if (kind === 'error') {
        status.classList.add('text-danger');
      } else if (kind === 'success') {
        status.classList.add('text-success');
      } else {
        status.classList.add('text-secondary');
      }
    }

    function markDirty(input) {
      input.dataset.dirty = 'true';
    }

    dialog.querySelectorAll('[data-bulk-frontmatter-input]').forEach(function (input) {
      input.addEventListener('input', function () { markDirty(input); });
      input.addEventListener('change', function () { markDirty(input); });
    });

    if (fillBtn) {
      fillBtn.addEventListener('click', function () {
        rowsRoot.querySelectorAll('[data-bulk-frontmatter-row]').forEach(function (row) {
          row.querySelectorAll('[data-bulk-frontmatter-input]').forEach(function (input) {
            if ((input.value || '').trim() !== '') return;
            const def = input.getAttribute('data-default-value') || '';
            if (!def) return;
            if (input.tagName === 'SELECT') {
              if (input.value === '') {
                input.value = def;
                markDirty(input);
              }
            } else {
              input.value = def;
              markDirty(input);
            }
          });
        });
        setStatus('Defaults filled. Review changes and save.');
      });
    }

    closeButtons.forEach(function (btn) {
      btn.addEventListener('click', function () {
        if (dialog.open) dialog.close();
        dialog.remove();
      });
    });
    dialog.addEventListener('close', function () {
      dialog.remove();
    });
    dialog.addEventListener('click', function (event) {
      if (event.target === dialog && dialog.open) {
        dialog.close();
      }
    });

    if (saveBtn) {
      saveBtn.addEventListener('click', async function () {
        const rows = [];
        rowsRoot.querySelectorAll('[data-bulk-frontmatter-row]').forEach(function (row) {
          const values = {};
          row.querySelectorAll('[data-bulk-frontmatter-input][data-dirty="true"]').forEach(function (input) {
            values[input.dataset.fieldName] = input.value;
          });
          if (Object.keys(values).length === 0) return;
          rows.push({
            page_path: row.dataset.pagePath || '',
            revision: row.dataset.revision || '',
            values: values,
          });
        });
        if (rows.length === 0) {
          setStatus('No changes to save.');
          return;
        }
        saveBtn.disabled = true;
        setStatus('Saving...');
        try {
          const fd = new FormData();
          fd.set('rows', JSON.stringify(rows));
          const response = await fetch(dialog.getAttribute('data-save-url') || '', {
            method: 'POST',
            body: fd,
            headers: { Accept: 'application/json' },
          });
          const data = await response.json();
          if (!response.ok) throw data;
          const errors = [];
          const results = new Map((data.rows || []).map(function (row) {
            return [row.page_path, row];
          }));
          rowsRoot.querySelectorAll('[data-bulk-frontmatter-row]').forEach(function (row) {
            const result = results.get(row.dataset.pagePath || '');
            row.classList.remove('dyno-bulk-row-error');
            if (!result) return;
            if (result.ok) {
              row.querySelectorAll('[data-bulk-frontmatter-input]').forEach(function (input) {
                delete input.dataset.dirty;
              });
            } else {
              errors.push((row.dataset.pagePath || 'document') + ': ' + (result.error || 'save failed'));
              row.classList.add('dyno-bulk-row-error');
            }
          });
          if (errors.length > 0) {
            setStatus(errors.join(' | '), 'error');
            return;
          }
          setStatus('Saved. Reloading...');
          dialog.close();
          window.location.reload();
        } catch (error) {
          setStatus(error && error.error ? error.error : 'Failed to save bulk edits.', 'error');
        } finally {
          saveBtn.disabled = false;
        }
      });
    }
  }

  document.addEventListener('click', function (event) {
    const btn = event.target.closest('[data-bulk-frontmatter-open]');
    if (!btn) return;
    event.preventDefault();
    openBulkEditor(btn.getAttribute('data-bulk-frontmatter-url') || '').catch(function (err) {
      console.error(err);
      alert(err && err.message ? err.message : 'Failed to open bulk editor.');
    });
  });
}

// ─── Frontmatter editor ──────────────────────────────────────────────────────

function initFrontmatterEditor() {
  document.querySelectorAll('[data-frontmatter-dialog]:not([data-frontmatter-init])').forEach(function (dialog) {
    dialog.setAttribute('data-frontmatter-init', '1');

    const form = dialog.querySelector('[data-frontmatter-form]');
    const rows = dialog.querySelector('[data-frontmatter-rows]');
    const template = dialog.querySelector('[data-frontmatter-row-template]');
    const openBtn = document.querySelector('[data-frontmatter-open]');
    const addBtn = dialog.querySelector('[data-frontmatter-add]');
    const status = dialog.querySelector('[data-frontmatter-status]');
    const cancelButtons = dialog.querySelectorAll('[data-frontmatter-cancel]');
    const entriesInput = dialog.querySelector('[data-frontmatter-entries]');
    const saveBtn = form && form.querySelector('button[type="submit"]');

    if (!form || !rows || !template || !entriesInput) return;

    function setStatus(message, kind) {
      if (!status) return;
      status.textContent = message || '';
      status.className = 'small mb-0 mt-1';
      if (kind === 'error') {
        status.classList.add('text-danger');
      } else if (kind === 'success') {
        status.classList.add('text-success');
      } else {
        status.classList.add('text-secondary');
      }
    }

    function rowTemplate() {
      const fragment = template.content.cloneNode(true);
      const row = fragment.querySelector('[data-frontmatter-row]');
      if (!row) return null;
      bindRow(row);
      return row;
    }

    function bindRow(row) {
      row.querySelectorAll('[data-frontmatter-remove]').forEach(function (btn) {
        btn.addEventListener('click', function () {
          row.remove();
          if (!rows.querySelector('[data-frontmatter-row]')) addRow();
        });
      });
      return row;
    }

    function addRow(key, value) {
      const row = rowTemplate();
      if (!row) return null;
      const keyInput = row.querySelector('[data-frontmatter-key]');
      const valueInput = row.querySelector('[data-frontmatter-value]');
      if (keyInput) keyInput.value = key || '';
      if (valueInput) valueInput.value = value || '';
      rows.appendChild(row);
      return row;
    }

    function collectEntries() {
      const payload = [];
      const seen = new Set();
      const rowList = Array.from(rows.querySelectorAll('[data-frontmatter-row]'));
      for (const row of rowList) {
        const keyInput = row.querySelector('[data-frontmatter-key]');
        const valueInput = row.querySelector('[data-frontmatter-value]');
        const key = keyInput ? keyInput.value.trim() : '';
        const value = valueInput ? valueInput.value : '';
        if (!key && !value.trim()) continue;
        if (!key) return { error: 'Frontmatter key cannot be empty.' };
        if (seen.has(key)) return { error: 'Duplicate frontmatter key: ' + key };
        seen.add(key);
        payload.push({ key: key, value: value });
      }
      return { entries: payload };
    }

    function open() {
      if (!dialog.open) dialog.showModal();
      if (!rows.querySelector('[data-frontmatter-row]')) addRow();
      const firstKey = dialog.querySelector('[data-frontmatter-key]');
      if (firstKey) firstKey.focus();
    }

    function close() {
      if (dialog.open) dialog.close();
    }

    if (openBtn) {
      openBtn.addEventListener('click', open);
    }
    cancelButtons.forEach(function (btn) {
      btn.addEventListener('click', close);
    });
    if (addBtn) {
      addBtn.addEventListener('click', function () {
        const row = addRow();
        const keyInput = row && row.querySelector('[data-frontmatter-key]');
        if (keyInput) keyInput.focus();
      });
    }
    dialog.addEventListener('click', function (event) {
      if (event.target === dialog) close();
    });
    if (!rows.querySelector('[data-frontmatter-row]')) {
      addRow();
    }
    rows.querySelectorAll('[data-frontmatter-row]').forEach(bindRow);

    form.addEventListener('submit', function (event) {
      event.preventDefault();
      const result = collectEntries();
      if (result.error) {
        setStatus(result.error, 'error');
        return;
      }
      setStatus('Saving frontmatter...');
      if (saveBtn) saveBtn.disabled = true;

      const fd = new FormData();
      fd.set('page_path', form.querySelector('input[name="page_path"]').value);
      fd.set('revision', form.querySelector('input[name="revision"]').value);
      fd.set('entries', JSON.stringify(result.entries || []));

      fetch(form.action, {
        method: 'POST',
        body: fd,
        headers: { Accept: 'application/json' },
      }).then(function (res) {
        return res.json().then(function (data) {
          if (!res.ok) throw data;
          return data;
        });
      }).then(function () {
        close();
        window.location.reload();
      }).catch(function (error) {
        setStatus(error && error.error ? error.error : 'Failed to save frontmatter.', 'error');
      }).finally(function () {
        if (saveBtn) saveBtn.disabled = false;
      });
    });
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

  const prose = document.querySelector('.dyno-prose');
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
  initTableWidgets();
  initComments();
  initBulkFrontmatterEditor();
  initFrontmatterEditor();
  applyCommentHighlights();
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
    initTableWidgets();
    initComments();
    initBulkFrontmatterEditor();
    initFrontmatterEditor();
    applyCommentHighlights();
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
      const active = container.querySelector('.search-result-link.is-keyboard-active');
      let idx = active ? links.indexOf(active) : -1;
      if (active) active.classList.remove('is-keyboard-active');
      idx = e.key === 'ArrowDown' ? Math.min(idx + 1, links.length - 1) : Math.max(idx - 1, 0);
      links[idx].classList.add('is-keyboard-active');
      links[idx].scrollIntoView({ block: 'nearest' });
      return;
    }

    // Enter — navigate to focused result
    if (e.key === 'Enter' && !container.classList.contains('hidden')) {
      const active = container.querySelector('.search-result-link.is-keyboard-active');
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
  document.querySelectorAll('.dyno-prose img:not([data-lightbox-init])').forEach(function (img) {
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

function apiOwnElements(widget, selector) {
  return Array.from(widget.querySelectorAll(selector))
    .filter(el => el.closest('.api-widget') === widget);
}

function apiOwnElement(widget, selector) {
  return apiOwnElements(widget, selector)[0] || null;
}

function apiToggle(widget) {
  if (!widget) return;
  const panel = apiOwnElement(widget, '.api-panel');
  const btn = apiOwnElement(widget, '.api-toggle');
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
  apiOwnElements(widget, '.api-auth-tab').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  apiOwnElements(widget, '.api-auth-panel').forEach(p => {
    p.style.display = p.dataset.auth === mode ? '' : 'none';
  });
}

function apiRespTab(widget, tab, btn) {
  if (!widget) return;
  apiOwnElements(widget, '.api-resp-tab').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  apiOwnElements(widget, '.api-resp-panel').forEach(p => {
    p.style.display = p.dataset.resp === tab ? '' : 'none';
  });
}

function apiCopyResponse(widget, btn) {
  if (!widget || !btn) return;
  const body = apiOwnElement(widget, '[data-role="resp-body"]');
  if (!body) return;

  navigator.clipboard.writeText(body.textContent || '').then(() => {
    const original = btn.innerHTML;
    btn.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>';
    btn.classList.add('copied');
    btn.setAttribute('aria-label', 'Response copied');
    btn.setAttribute('title', 'Response copied');
    setTimeout(() => {
      btn.innerHTML = original;
      btn.classList.remove('copied');
      btn.setAttribute('aria-label', 'Copy response body');
      btn.setAttribute('title', 'Copy response body');
    }, 2000);
  });
}

function apiJSONPathValues(value, path) {
  if (typeof path !== 'string' || !path.startsWith('$')) {
    throw new Error('JSONPath must start with $');
  }

  const tokens = [];
  let pos = 1;
  while (pos < path.length) {
    if (path[pos] === '.') {
      const match = path.slice(pos + 1).match(/^[A-Za-z0-9_$-]+/);
      if (!match) throw new Error('Invalid property at position ' + pos);
      tokens.push({ type: 'property', value: match[0] });
      pos += match[0].length + 1;
      continue;
    }
    if (path[pos] === '[') {
      const end = path.indexOf(']', pos + 1);
      if (end < 0) throw new Error('Unclosed bracket at position ' + pos);
      const content = path.slice(pos + 1, end).trim();
      if (content === '*') {
        tokens.push({ type: 'wildcard' });
      } else if (/^\d+$/.test(content)) {
        tokens.push({ type: 'index', value: Number(content) });
      } else if ((content.startsWith('"') && content.endsWith('"')) ||
                 (content.startsWith("'") && content.endsWith("'"))) {
        tokens.push({ type: 'property', value: content.slice(1, -1) });
      } else {
        throw new Error('Unsupported bracket expression [' + content + ']');
      }
      pos = end + 1;
      continue;
    }
    throw new Error('Unsupported JSONPath syntax at position ' + pos);
  }

  let values = [value];
  tokens.forEach(token => {
    const next = [];
    values.forEach(item => {
      if (token.type === 'wildcard' && Array.isArray(item)) {
        next.push(...item);
      } else if (token.type === 'index' && Array.isArray(item) && token.value < item.length) {
        next.push(item[token.value]);
      } else if (token.type === 'property' && item !== null && typeof item === 'object' &&
                 Object.prototype.hasOwnProperty.call(item, token.value)) {
        next.push(item[token.value]);
      }
    });
    values = next;
  });
  return values;
}

function apiResolveFollowURL(rawValue, parentURL) {
  const value = String(rawValue || '').trim();
  if (!value) return '';
  if (/^https?:\/\//i.test(value)) return new URL(value).href;

  const parent = new URL(parentURL, window.location.href);
  const rootPath = '/' + value.replace(/^\/+/, '');
  return new URL(rootPath, parent.origin).href;
}

function apiCreateFollowupWidget(parentSpec, method, url) {
  window.__apiFollowupCounter = (window.__apiFollowupCounter || 0) + 1;
  const id = 'api-followup-' + window.__apiFollowupCounter;
  const widget = document.createElement('div');
  widget.className = 'api-widget api-followup-widget';
  widget.id = id;

  const auth = parentSpec.noAuth ? '' : `
    <div class="api-section">
      <div class="api-section-title">Auth</div>
      <div class="api-auth-tabs">
        <button type="button" class="api-auth-tab active" data-api-action="auth-tab" data-api-auth="none">None</button>
        <button type="button" class="api-auth-tab" data-api-action="auth-tab" data-api-auth="bearer">Bearer</button>
        <button type="button" class="api-auth-tab" data-api-action="auth-tab" data-api-auth="basic">Basic</button>
      </div>
      <div class="api-auth-panel" data-auth="bearer" style="display:none">
        <input class="api-input" data-role="bearer-token" placeholder="Bearer token">
      </div>
      <div class="api-auth-panel" data-auth="basic" style="display:none">
        <input class="api-input" data-role="basic-user" placeholder="Username" style="margin-bottom:0.4rem">
        <input class="api-input" data-role="basic-pass" placeholder="Password" type="password">
      </div>
    </div>`;
  const requestBody = ['POST', 'PUT', 'PATCH'].includes(method) ? `
    <div class="api-section">
      <div class="api-section-title">Body <span class="api-hint">JSON</span></div>
      <textarea class="api-textarea" data-role="body" rows="4" placeholder='{"key": "value"}'></textarea>
    </div>` : '';

  widget.innerHTML = `
    <div class="api-titlebar">
      <span class="api-method"></span>
      <span class="api-url"></span>
      <button type="button" class="api-toggle" data-api-action="toggle" aria-expanded="false">
        <span class="api-toggle-label">Try it</span>
        <svg class="api-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg>
      </button>
    </div>
    <div class="api-panel" hidden>
      ${auth}
      <div data-role="followup-headers-anchor"></div>
      ${requestBody}
      <div class="api-send-row">
        <button type="button" class="api-send-btn" data-api-action="send"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M5 12h14M12 5l7 7-7 7"/></svg> Send</button>
        <span class="api-status-badge" data-role="status"></span>
      </div>
      <div class="api-response" data-role="response" style="display:none">
        <div class="api-response-tabs">
          <button type="button" class="api-resp-tab active" data-api-action="resp-tab" data-api-resp="body">Body</button>
          <button type="button" class="api-resp-tab" data-api-action="resp-tab" data-api-resp="headers">Headers</button>
        </div>
        <div class="api-resp-panel api-resp-body" data-resp="body">
          <button type="button" class="api-resp-copy" data-api-action="copy-response" aria-label="Copy response body" title="Copy response body"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg></button>
          <pre class="api-resp-pre" data-role="resp-body"></pre>
        </div>
        <div class="api-resp-panel" data-resp="headers" style="display:none"><table class="api-resp-headers" data-role="resp-headers"></table></div>
      </div>
    </div>`;

  const methodBadge = widget.querySelector('.api-method');
  methodBadge.textContent = method;
  methodBadge.classList.add('api-method-' + (['get', 'post', 'put', 'patch', 'delete'].includes(method.toLowerCase()) ? method.toLowerCase() : 'other'));
  widget.querySelector('.api-url').textContent = url;

  const headers = Array.isArray(parentSpec.headers) ? parentSpec.headers : [];
  if (headers.length > 0) {
    const section = document.createElement('div');
    section.className = 'api-section';
    const title = document.createElement('div');
    title.className = 'api-section-title';
    title.textContent = 'Headers';
    section.appendChild(title);
    headers.forEach(([key, value]) => {
      const row = document.createElement('div');
      row.className = 'api-field-row';
      const label = document.createElement('span');
      label.className = 'api-field-label api-field-label--fixed';
      label.textContent = key;
      const fieldValue = document.createElement('span');
      fieldValue.className = 'api-field-value';
      fieldValue.textContent = value;
      row.append(label, fieldValue);
      section.appendChild(row);
    });
    widget.querySelector('[data-role="followup-headers-anchor"]').replaceWith(section);
  }

  window.__apiWidgets = window.__apiWidgets || {};
  window.__apiWidgets[id] = {
    method,
    url,
    headers,
    insecure: parentSpec.insecure === true,
    noAuth: parentSpec.noAuth === true,
    followJsonPath: '',
    followMethod: ''
  };
  return widget;
}

function apiRenderFollowups(widget, responseBody) {
  const spec = (window.__apiWidgets || {})[widget.id];
  if (!spec || !spec.followJsonPath) return;

  const container = widget.querySelector(':scope > .api-panel > [data-role="followups"]');
  if (!container) return;
  const list = container.querySelector('[data-role="followup-list"]');
  const message = container.querySelector('[data-role="followup-message"]');
  const count = container.querySelector('[data-role="followup-count"]');
  list.querySelectorAll('.api-followup-widget').forEach(child => {
    delete window.__apiWidgets[child.id];
  });
  list.replaceChildren();
  message.textContent = '';
  count.textContent = '';
  container.hidden = false;

  try {
    const parsed = JSON.parse(responseBody);
    const values = apiJSONPathValues(parsed, spec.followJsonPath);
    const parentURL = widget.dataset.apiRequestUrl || spec.url;
    const urls = [];
    const seen = new Set();
    values.forEach(value => {
      if (typeof value !== 'string') return;
      const resolved = apiResolveFollowURL(value, parentURL);
      if (resolved && !seen.has(resolved)) {
        seen.add(resolved);
        urls.push(resolved);
      }
    });

    const limited = urls.slice(0, 50);
    count.textContent = '(' + limited.length + ')';
    if (limited.length === 0) {
      message.textContent = 'No follow-up requests discovered.';
      return;
    }
    const method = String(spec.followMethod || 'GET').toUpperCase();
    limited.forEach(url => list.appendChild(apiCreateFollowupWidget(spec, method, url)));
    if (urls.length > limited.length) {
      message.textContent = 'Showing the first 50 of ' + urls.length + ' discovered requests.';
    }
  } catch (err) {
    message.textContent = 'Follow-up discovery failed: ' + err.message;
  }
}

function apiSend(widget) {
  if (!widget) return;
  const id = widget.id;
  const spec = (window.__apiWidgets || {})[id];
  if (!spec) return;

  // Resolve {{var}} placeholders
  const vars = {};
  apiOwnElements(widget, '[data-var]').forEach(el => {
    vars[el.dataset.var] = el.value;
  });
  function resolve(s) {
    return s.replace(/\{\{(\w+)\}\}/g, (_, k) => vars[k] || '');
  }

  let url = resolve(spec.url);
  widget.dataset.apiRequestUrl = url;
  let headers = {};

  // Static headers from the block
  (spec.headers || []).forEach(([k, v]) => { headers[resolve(k)] = resolve(v); });

  // Auth
  const activeAuth = apiOwnElement(widget, '.api-auth-tab.active');
  const authMode = activeAuth ? activeAuth.textContent.trim().toLowerCase() : 'none';
  if (authMode === 'bearer') {
    const tok = apiOwnElement(widget, '[data-role="bearer-token"]');
    if (tok && tok.value) headers['Authorization'] = 'Bearer ' + tok.value;
  } else if (authMode === 'basic') {
    const u = apiOwnElement(widget, '[data-role="basic-user"]');
    const p = apiOwnElement(widget, '[data-role="basic-pass"]');
    if (u && p) headers['Authorization'] = 'Basic ' + btoa(u.value + ':' + p.value);
  }

  // Body
  let body = '';
  const bodyEl = apiOwnElement(widget, '[data-role="body"]');
  if (bodyEl) {
    body = bodyEl.value;
    if (body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
  }

  // UI: show loading state
  const sendBtn = apiOwnElement(widget, '.api-send-btn');
  const statusBadge = apiOwnElement(widget, '[data-role="status"]');
  const responseDiv = apiOwnElement(widget, '[data-role="response"]');
  sendBtn.disabled = true;
  sendBtn.textContent = 'Sending…';
  statusBadge.className = 'api-status-badge';
  statusBadge.textContent = '';
  responseDiv.style.display = 'none';

  fetch('/api-proxy', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ method: spec.method, url, headers, body, insecure: spec.insecure === true }),
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
      const bodyPre = apiOwnElement(widget, '[data-role="resp-body"]');
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

  const statusBadge = apiOwnElement(widget, '[data-role="status"]');
  const responseDiv = apiOwnElement(widget, '[data-role="response"]');
  const bodyPre = apiOwnElement(widget, '[data-role="resp-body"]');
  const headersTable = apiOwnElement(widget, '[data-role="resp-headers"]');

  statusBadge.textContent = data.status + ' · ' + data.elapsed + 'ms';
  statusBadge.className = 'api-status-badge visible ' + data.statusClass;

  if (bodyPre) bodyPre.textContent = data.body;

  apiRenderFollowups(widget, data.body);

  if (headersTable) {
    headersTable.innerHTML = (data.headers || [])
      .map(([k, v]) => `<tr><td>${escHtml(k)}</td><td>${escHtml(v)}</td></tr>`)
      .join('');
  }

  responseDiv.style.display = '';
  // Show body tab by default
  apiOwnElements(widget, '.api-resp-tab').forEach(b => b.classList.remove('active'));
  const bodyTab = apiOwnElement(widget, '.api-resp-tab');
  if (bodyTab) bodyTab.classList.add('active');
  apiOwnElements(widget, '.api-resp-panel').forEach(p => {
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
  if (action === 'copy-response') {
    apiCopyResponse(widget, btn);
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
  container.innerHTML = '<div class="ego-graph-status">Loading graph...</div>';
  fetch(url).then(function(r) { return r.text(); }).then(function(html) {
    container.innerHTML = html;
  }).catch(function() {
    container.innerHTML = '<div class="ego-graph-status is-error">Failed to load graph.</div>';
  });
});

// ── Canvas site graph ─────────────────────────────────────────────────────
function initSiteGraph(container) {
  var dataUrl = container.getAttribute('data-graph-data-url');
  if (!dataUrl) return;
  if (container._dynoGraphTooltip) {
    container._dynoGraphTooltip.remove();
    container._dynoGraphTooltip = null;
  }

  var bg = dynoThemeColor('--tblr-body-bg');
  var nodeFill = dynoThemeColor('--tblr-primary-bg-subtle');
  var nodeStroke = dynoThemeColor('--tblr-primary');
  var nodeText = dynoThemeColor('--tblr-primary-text-emphasis');
  var edgeColor = dynoThemeColor('--tblr-border-color');

  container.innerHTML = '<canvas id="site-graph-canvas" class="site-graph-canvas"></canvas>';
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
    tooltip.className = 'site-graph-tooltip';
    tooltip.style.display = 'none';
    document.body.appendChild(tooltip);
    container._dynoGraphTooltip = tooltip;
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
    ctx.fillStyle = dynoThemeColor('--tblr-danger');
    ctx.font = '14px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText('Failed to load graph data.', W/2, H/2);
  });
}

function initGraphPages() {
  var el = document.querySelector('.graph-page[data-graph-data-url]');
  if (el) initSiteGraph(el);
}

function rerenderSiteGraphs() {
  document.querySelectorAll('.graph-page[data-graph-data-url]').forEach(initSiteGraph);
}

document.addEventListener('DOMContentLoaded', initGraphPages);
document.addEventListener('htmx:afterSwap', initGraphPages);
