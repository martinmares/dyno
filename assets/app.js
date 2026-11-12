// ─── Theme ────────────────────────────────────────────────────────────────────

(function () {
  const html = document.documentElement;
  const stored = localStorage.getItem('dyno-theme');

  function applyTheme(dark) {
    html.classList.toggle('dark', dark);
    html.setAttribute('data-theme', dark ? 'dark' : 'light');
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

// ─── Expanded article ─────────────────────────────────────────────────────────

function initArticleExpansion() {
  const panel = document.querySelector('.dyno-document');
  const dialog = document.getElementById('document-expanded');
  const button = document.getElementById('document-expand');
  if (!panel || !dialog || !button || typeof dialog.showModal !== 'function') return;
  button.hidden = false;
  let saved = null;
  let busy = false;

  function headerBottom() {
    const header = document.querySelector('.dyno-navbar');
    return header ? header.getBoundingClientRect().bottom : 0;
  }

  function readingAnchor(top, bottom) {
    const element = Array.from(panel.querySelectorAll('.dyno-article h1, .dyno-article h2, .dyno-article h3, .dyno-article p, .dyno-article pre, .dyno-article table'))
      .find(element => {
        const rect = element.getBoundingClientRect();
        return rect.bottom > top && rect.top < bottom;
      });
    return element && { element, offset: element.getBoundingClientRect().top - top };
  }

  function visibleRect(rect) {
    const top = Math.max(headerBottom(), rect.top);
    return { top, left: rect.left, width: rect.width, height: Math.max(1, Math.min(rect.bottom, innerHeight) - top) };
  }

  async function animate(from, to) {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    const frame = rect => ({ top: rect.top + 'px', left: rect.left + 'px', width: rect.width + 'px', height: rect.height + 'px' });
    const animation = dialog.animate([frame(from), frame(to)], { duration: 360, easing: 'cubic-bezier(.22, 1, .36, 1)' });
    try { await animation.finished; } catch (_) {}
  }

  function updateButton(expanded) {
    const label = expanded ? 'Restore article size (Esc)' : 'Expand article';
    button.setAttribute('aria-expanded', String(expanded));
    button.setAttribute('aria-label', label);
    button.title = label;
    button.querySelector('[data-expand-icon]').hidden = expanded;
    button.querySelector('[data-restore-icon]').hidden = !expanded;
  }

  async function expand() {
    const rect = panel.getBoundingClientRect();
    const scroll = window.scrollY;
    const anchor = readingAnchor(Math.max(rect.top, headerBottom()), innerHeight);
    const placeholder = document.createElement('div');
    placeholder.className = 'dyno-document-placeholder dyno-screen-only';
    placeholder.style.height = rect.height + 'px';
    saved = { placeholder, scroll, content: document.getElementById('page-content').firstElementChild };
    document.body.classList.add('dyno-reading-expanded');
    panel.before(placeholder);
    dialog.append(panel);
    updateButton(true);
    dialog.showModal();
    dialog.focus({ preventScroll: true });
    const target = dialog.getBoundingClientRect();
    if (anchor) panel.scrollTop += anchor.element.getBoundingClientRect().top - target.top - anchor.offset;
    await animate(visibleRect(rect), target);
    // Width changes during the animation can temporarily clamp the scroll offset.
    if (anchor) panel.scrollTop += anchor.element.getBoundingClientRect().top - target.top - anchor.offset;
    saved.initialScroll = panel.scrollTop;
  }

  async function restore() {
    const anchor = readingAnchor(dialog.getBoundingClientRect().top, innerHeight);
    const readFurther = Math.abs(panel.scrollTop - saved.initialScroll) > 2;
    await animate(dialog.getBoundingClientRect(), visibleRect(saved.placeholder.getBoundingClientRect()));
    const changedPage = saved.content !== document.getElementById('page-content').firstElementChild;
    dialog.close();
    saved.placeholder.replaceWith(panel);
    panel.scrollTop = 0;
    document.body.classList.remove('dyno-reading-expanded');
    updateButton(false);
    let top = changedPage ? 0 : saved.scroll;
    if (!changedPage && readFurther && anchor && anchor.element.isConnected) {
      top = window.scrollY + anchor.element.getBoundingClientRect().top - headerBottom() - anchor.offset;
    }
    window.scrollTo({ top, behavior: 'instant' });
    saved = null;
    button.focus({ preventScroll: true });
  }

  async function toggle() {
    if (busy) return;
    busy = true;
    try { if (saved) await restore(); else await expand(); }
    finally { busy = false; }
  }

  button.addEventListener('click', toggle);
  dialog.addEventListener('cancel', function (event) {
    event.preventDefault();
    if (saved) toggle();
  });
}

document.addEventListener('DOMContentLoaded', initArticleExpansion);

// ─── Sidebar ──────────────────────────────────────────────────────────────────

function initSidebarCollapse() {
  const collapsed = document.documentElement.classList.contains('dyno-sidebar-collapsed');
  document.querySelectorAll('[data-sidebar-collapse]').forEach(function (button) {
    button.setAttribute('aria-expanded', String(!collapsed));
    button.setAttribute('aria-label', collapsed ? 'Expand navigation' : 'Collapse navigation');
    button.title = collapsed ? 'Expand navigation' : 'Collapse navigation';
  });
}

document.addEventListener('DOMContentLoaded', initSidebarCollapse);
document.addEventListener('htmx:afterSwap', initSidebarCollapse);
document.addEventListener('click', function (event) {
  const button = event.target.closest('[data-sidebar-collapse]');
  if (!button || window.innerWidth < 1024) return;
  const sidebar = document.getElementById('sidebar');
  if (!sidebar) return;
  const collapsed = document.documentElement.classList.toggle('dyno-sidebar-collapsed');
  try {
    localStorage.setItem('dyno-sidebar-collapsed:' + (sidebar.dataset.navStorageKey || '/'), String(collapsed));
  } catch (_) {}
  initSidebarCollapse();
  const focusTarget = collapsed ? '.dyno-sidebar-restore' : '#sidebar [data-sidebar-collapse]';
  document.querySelector(focusTarget)?.focus({ preventScroll: true });
});

document.addEventListener('DOMContentLoaded', function () {
  if (window.DynoSyntax) DynoSyntax.highlightAll();
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
    if (event.target.closest('.dyno-nav-link') && window.innerWidth < 1024) closeSidebar();
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

// Browse file timestamps and sorting are local UI preferences, not site metadata.
function formatBrowseFileTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const pad = number => String(number).padStart(2, '0');
  return pad(date.getDate()) + '.' + pad(date.getMonth() + 1) + '.' + String(date.getFullYear()).slice(-2)
    + ' ' + pad(date.getHours()) + ':' + pad(date.getMinutes());
}

function compareBrowseFiles(a, b, field, direction) {
  const directoryA = a.hasAttribute('data-nav-section');
  const directoryB = b.hasAttribute('data-nav-section');
  if (directoryA !== directoryB) return directoryA ? -1 : 1;
  const name = (a.dataset.fileName || a.dataset.navTitle || '').localeCompare(
    b.dataset.fileName || b.dataset.navTitle || '', undefined, { numeric: true, sensitivity: 'base' });
  if (directoryA) return name;
  if (field === 'name') return direction === 'desc' ? -name : name;
  const key = field === 'created' ? 'fileCreated' : 'fileModified';
  const valueA = a.dataset[key], valueB = b.dataset[key];
  if (!valueA || !valueB) return valueA ? -1 : valueB ? 1 : name;
  const difference = Number(valueA) - Number(valueB);
  return (direction === 'desc' ? -difference : difference) || name;
}

function browseFileMatches(name, query) {
  const fold = value => value.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
  return fold(name).includes(fold(query.trim()));
}

function initBrowseFileFilter(sidebar) {
  const input = sidebar.querySelector('[data-browse-file-filter]');
  if (!input) return;
  const tree = sidebar.querySelector('.dyno-nav-tree');
  const key = 'dyno-browse-filter:' + Array.from(tree.children).map(node => node.dataset.navPath || node.dataset.path).sort().join('|');
  try { input.value = sessionStorage.getItem(key) || ''; } catch (_) {}
  function apply() {
    const query = input.value.trim();
    let matches = 0;
    tree.querySelectorAll('[data-file-name]').forEach(function (file) {
      file.hidden = !browseFileMatches(file.dataset.fileName, query);
      if (!file.hidden) matches++;
    });
    const collapsed = readCollapsedNavPaths(sidebar);
    tree.querySelectorAll('[data-nav-section]').forEach(function (section) {
      const hasMatch = !!section.querySelector('[data-file-name]:not([hidden])');
      section.hidden = !!query && !hasMatch;
      const active = !!section.querySelector('.dyno-nav-link[data-active="true"]');
      setNavSectionExpanded(section, query ? hasMatch : active || !collapsed.has(section.dataset.navPath));
    });
    sidebar.querySelector('[data-browse-file-empty]').hidden = matches > 0;
    try { sessionStorage.setItem(key, input.value); } catch (_) {}
  }
  input.oninput = apply;
  input.onkeydown = function (event) {
    if (event.key !== 'Escape') return;
    event.preventDefault();
    input.value = '';
    apply();
  };
  apply();
}

function initBrowseFiles() {
  const controls = document.querySelector('[data-browse-sort]');
  if (!controls) return;
  const sidebar = controls.closest('#sidebar');
  const field = controls.querySelector('[data-browse-sort-field]');
  const direction = controls.querySelector('[data-browse-sort-direction]');
  const key = 'dyno-browse-sort';
  let state = { field: 'name', direction: 'asc' };
  try {
    const saved = JSON.parse(localStorage.getItem(key));
    if (saved && ['name', 'created', 'modified'].includes(saved.field) && ['asc', 'desc'].includes(saved.direction)) state = saved;
  } catch (_) {}
  sidebar.querySelectorAll('.dyno-browse-file-times time').forEach(function (time) {
    const date = new Date(time.dateTime);
    if (!Number.isNaN(date.getTime())) {
      time.textContent = formatBrowseFileTime(time.dateTime);
      time.title = date.toLocaleString();
    }
  });
  function apply() {
    field.value = state.field;
    const label = state.direction === 'asc' ? 'Ascending' : 'Descending';
    direction.setAttribute('aria-label', label);
    direction.title = label;
    direction.dataset.direction = state.direction;
    sidebar.querySelectorAll('.dyno-nav-tree, [data-nav-children]').forEach(function (container) {
      Array.from(container.children).sort(function (a, b) {
        return compareBrowseFiles(a, b, state.field, state.direction);
      }).forEach(function (child) { container.appendChild(child); });
    });
  }
  function save() {
    try { localStorage.setItem(key, JSON.stringify(state)); } catch (_) {}
    apply();
  }
  field.onchange = function () {
    state.field = field.value;
    state.direction = state.field === 'name' ? 'asc' : 'desc';
    save();
  };
  direction.onclick = function () {
    state.direction = state.direction === 'asc' ? 'desc' : 'asc';
    save();
  };
  apply();
}

function initNavTree() {
  initBrowseFiles();
  const sidebar = document.getElementById('sidebar');
  if (!sidebar) return;
  const collapsed = readCollapsedNavPaths(sidebar);
  sidebar.querySelectorAll('[data-nav-section]').forEach(function (section) {
    const containsActivePage = !!section.querySelector('.dyno-nav-link[data-active="true"]');
    setNavSectionExpanded(section, containsActivePage || !collapsed.has(section.dataset.navPath));
  });
  initBrowseFileFilter(sidebar);
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

  // Match the light and blue-dark palettes used by renderD2.
  const dark = document.documentElement.classList.contains('dark');
  const colors = dark ? {
    fill: '#263750', secondary: '#334661', tertiary: '#496181',
    border: '#8ABAFF', text: '#E2EAF5', muted: '#A7B9D1',
    pie: ['#334661', '#496181', '#5D7BA1', '#365577', '#3F6A8A', '#627D99'],
  } : {
    fill: '#F7F8FE', secondary: '#E3E9FD', tertiary: '#EDF0FD',
    border: '#0D32B2', text: '#0A0F25', muted: '#676C7E',
    pie: ['#E3E9FD', '#DCE9E2', '#F4E5CB', '#EBDFF3', '#D9EAF1', '#F1DDE2'],
  };
  const background = dynoThemeColor('--dyno-page-bg');

  const themeVariables = {
    darkMode: dark,
    background,
    fontFamily: dynoThemeColor('--dyno-font-sans'),
    primaryColor: colors.fill,
    secondaryColor: colors.secondary,
    tertiaryColor: colors.tertiary,
    primaryTextColor: colors.text,
    secondaryTextColor: colors.text,
    tertiaryTextColor: colors.text,
    primaryBorderColor: colors.border,
    secondaryBorderColor: colors.border,
    tertiaryBorderColor: colors.border,
    lineColor: colors.border,
    textColor: colors.text,
    mainBkg: colors.fill,
    nodeBorder: colors.border,
    clusterBkg: colors.secondary,
    clusterBorder: colors.border,
    edgeLabelBackground: background,
    actorBkg: colors.fill,
    actorBorder: colors.border,
    actorTextColor: colors.text,
    actorLineColor: colors.border,
    signalColor: colors.border,
    signalTextColor: colors.text,
    labelBoxBkgColor: colors.fill,
    labelBoxBorderColor: colors.border,
    labelTextColor: colors.text,
    activationBkgColor: colors.secondary,
    activationBorderColor: colors.border,
    noteBkgColor: colors.secondary,
    noteBorderColor: colors.border,
    noteTextColor: colors.text,
    attributeBackgroundColorOdd: colors.fill,
    attributeBackgroundColorEven: colors.secondary,
    pie1: colors.pie[0],
    pie2: colors.pie[1],
    pie3: colors.pie[2],
    pie4: colors.pie[3],
    pie5: colors.pie[4],
    pie6: colors.pie[5],
    pieStrokeColor: colors.border,
    pieOuterStrokeColor: colors.border,
    pieStrokeWidth: '2px',
    pieOuterStrokeWidth: '2px',
    pieTitleTextColor: colors.text,
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

// Keep the Explorer selection when following HTMX links inside the reader.
document.addEventListener('htmx:configRequest', function (event) {
  if (!document.querySelector('[data-explorer-return]')) return;
  if (event.detail.verb !== 'get' || event.detail.target?.id !== 'page-content') return;
  const state = new URLSearchParams(window.location.search).get('explore');
  if (state) event.detail.parameters.explore = state;
});

function restoreExplorerReturnLink() {
  const link = document.querySelector('[data-explorer-return]');
  const state = new URLSearchParams(window.location.search).get('explore');
  if (!link || !state) return;
  const url = new URL(link.href);
  url.search = state;
  link.href = url.href;
}
document.addEventListener('DOMContentLoaded', restoreExplorerReturnLink);
document.addEventListener('htmx:afterSettle', restoreExplorerReturnLink);

// ─── Document printing ────────────────────────────────────────────────────────

let printPageState = null;

function setPrintTimestamp(time) {
  const now = new Date();
  time.dateTime = now.toISOString();
  time.textContent = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'long' }).format(now);
}

document.addEventListener('DOMContentLoaded', function () {
  document.querySelectorAll('[data-report-time]').forEach(setPrintTimestamp);
});

function preparePagePrint() {
  if (printPageState) return;
  const article = document.querySelector('#page-content .dyno-article');
  if (!article) return;
  printPageState = {
    details: Array.from(article.querySelectorAll('details:not([open]):not([data-report-code-full]):not([data-report-table-full])')),
    rows: Array.from(article.querySelectorAll('.dyno-table-widget tbody tr[hidden]'))
  };
  article.querySelectorAll('[data-report-time]').forEach(setPrintTimestamp);
  const summary = article.querySelector('[data-print-summary]');
  if (summary) {
    printPageState.summary = { element: summary, parent: summary.parentNode, next: summary.nextSibling };
    const time = summary.querySelector('[data-print-time]');
    setPrintTimestamp(time);
    const prose = article.querySelector('.dyno-prose');
    const title = prose && prose.querySelector('h1');
    if (title) title.after(summary);
    else if (prose) prose.prepend(summary);
  }
  printPageState.details.forEach(function (details) { details.open = true; });
  printPageState.rows.forEach(function (row) { row.hidden = false; });
}

function restorePagePrint() {
  if (!printPageState) return;
  printPageState.details.forEach(function (details) { details.open = false; });
  printPageState.rows.forEach(function (row) { row.hidden = true; });
  if (printPageState.summary) {
    const saved = printPageState.summary;
    saved.parent.insertBefore(saved.element, saved.next);
    const time = saved.element.querySelector('[data-print-time]');
    time.textContent = '';
    time.removeAttribute('datetime');
  }
  printPageState = null;
}

window.addEventListener('beforeprint', preparePagePrint);
window.addEventListener('afterprint', restorePagePrint);
document.addEventListener('click', function (event) {
  if (event.target.closest('[data-print-page]')) window.print();
});

// ─── TOC scroll spy ───────────────────────────────────────────────────────────

function initTOCScrollSpy() {
  if (initTOCScrollSpy.cleanup) initTOCScrollSpy.cleanup();
  initTOCScrollSpy.cleanup = null;
  const tocLinks = document.querySelectorAll('.toc-link');
  if (!tocLinks.length) return;

  const headings = Array.from(tocLinks).map(function (link) {
    const id = link.getAttribute('href').slice(1);
    return document.getElementById(id);
  }).filter(Boolean);

  let previousActiveLink = null;

  function onScroll() {
    let active = headings[0];
    headings.forEach(function (h) {
      if (h.getBoundingClientRect().top <= 96) active = h;
    });
    if (!active) return;
    let activeLink = null;
    tocLinks.forEach(function (link) {
      const isActive = link.getAttribute('href') === '#' + active.id;
      link.classList.toggle('active', isActive);
      if (isActive) {
        link.setAttribute('aria-current', 'location');
        activeLink = link;
      }
      else link.removeAttribute('aria-current');
    });
    if (!activeLink || activeLink === previousActiveLink) return;
    previousActiveLink = activeLink;
    const panel = activeLink.closest('.toc-scroll');
    if (!panel || !panel.clientHeight) return;
    const bounds = panel.getBoundingClientRect();
    const linkBounds = activeLink.getBoundingClientRect();
    // Move only the TOC pane, never the article or other scroll ancestors.
    if (linkBounds.top < bounds.top + 8) panel.scrollTop += linkBounds.top - bounds.top - 8;
    else if (linkBounds.bottom > bounds.bottom - 8) panel.scrollTop += linkBounds.bottom - bounds.bottom + 8;
  }

  window.addEventListener('scroll', onScroll, { passive: true });
  initTOCScrollSpy.cleanup = function () { window.removeEventListener('scroll', onScroll); };
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

function isCommentSelectionExcluded(node) {
  const element = node && node.nodeType === Node.ELEMENT_NODE ? node : node && node.parentElement;
  return !!(element && element.closest(
    'input, textarea, select, button, [role="textbox"], ' +
    '[contenteditable]:not([contenteditable="false"]), ' +
    '.CodeMirror, .cm-editor, .ProseMirror, #editor-pane, dialog:not(#document-expanded), [data-comments-root]'
  ));
}

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

    function captureSelection(event) {
      if (!article || !quoteInput || !anchorInput) return;
      // Editors may leave a previous article selection intact while handling their own selection.
      if (document.querySelector('dialog[open]:not(#document-expanded)') ||
          isCommentSelectionExcluded(event && event.target) ||
          isCommentSelectionExcluded(document.activeElement)) return;
      const sel = window.getSelection();
      if (!sel || sel.isCollapsed || sel.rangeCount === 0) return;
      const range = sel.getRangeAt(0);
      if (isCommentSelectionExcluded(range.startContainer) ||
          isCommentSelectionExcluded(range.endContainer) ||
          isCommentSelectionExcluded(range.commonAncestorContainer)) return;
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
      if (e.key === 'Shift' || e.key.startsWith('Arrow')) captureSelection(e);
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
  const actions = document.createElement('div'); actions.className = 'buttons is-flex-wrap-nowrap';
  actions.innerHTML = '<button type="button" data-highlight-go="' + highlight.id + '" class="button is-small is-ghost">Go to</button>';
  if (root.dataset.commentsManagement === 'true') {
    actions.innerHTML += '<button type="button" data-annotation-delete="' + highlight.id + '" class="button is-small is-danger is-light">Delete</button>';
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
      status.className = 'is-size-7 mb-0 mt-1';
      if (kind === 'error') {
        status.classList.add('has-text-danger');
      } else if (kind === 'success') {
        status.classList.add('has-text-success');
      } else {
        status.classList.add('has-text-grey');
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

    const originalEntries = Array.from(rows.querySelectorAll('[data-frontmatter-row]')).map(function (row) {
      return { key: row.querySelector('[data-frontmatter-key]').value, value: row.querySelector('[data-frontmatter-value]').value };
    });
    let rowsWidth = 0;
    const resizeObserver = new ResizeObserver(function (entries) {
      const width = entries[0].contentRect.width;
      if (width === rowsWidth) return;
      rowsWidth = width;
      rows.querySelectorAll('[data-frontmatter-value]').forEach(resizeValue);
    });
    dialog.addEventListener('close', function () { resizeObserver.disconnect(); });

    function setStatus(message, kind) {
      if (!status) return;
      status.textContent = message || '';
      status.className = 'is-size-7 mb-0 mt-1';
      if (kind === 'error') {
        status.classList.add('has-text-danger');
      } else if (kind === 'success') {
        status.classList.add('has-text-success');
      } else {
        status.classList.add('has-text-grey');
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
      row.querySelectorAll('[data-frontmatter-key], [data-frontmatter-value]').forEach(function (input) {
        let beforeEdit = input.value;
        input.addEventListener('focus', function () {
          beforeEdit = input.value;
          if (input.matches('[data-frontmatter-value]')) resizeValue(input);
        });
        input.addEventListener('input', function () {
          updateSummary(row);
          if (input.matches('[data-frontmatter-value]')) resizeValue(input);
        });
        input.addEventListener('blur', function () {
          updateSummary(row);
          if (input.matches('[data-frontmatter-value]')) resizeValue(input);
        });
        input.addEventListener('keydown', function (event) {
          if (event.key === 'Escape') {
            event.preventDefault();
            event.stopPropagation();
            input.value = beforeEdit;
            input.blur();
          } else if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
            event.preventDefault();
            input.blur();
          }
        });
      });
      row.querySelectorAll('[data-frontmatter-remove]').forEach(function (btn) {
        btn.addEventListener('click', function () {
          row.remove();
          if (!rows.querySelector('[data-frontmatter-row]')) addRow();
        });
      });
      return row;
    }

    function resizeValue(input) {
      if (document.activeElement !== input) {
        input.style.height = '';
        return;
      }
      input.style.height = 'auto';
      input.style.height = (input.scrollHeight + 2) + 'px';
    }

    function updateSummary(row) {
      const input = row.querySelector('[data-frontmatter-value]');
      const summary = row.querySelector('[data-frontmatter-lines]');
      const count = input.value.split('\n').length;
      summary.hidden = count < 2;
      summary.textContent = count + ' lines';
      input.title = input.value;
      input.parentElement.toggleAttribute('data-multiline', count > 1);
    }

    function addRow(key, value) {
      const row = rowTemplate();
      if (!row) return null;
      const keyInput = row.querySelector('[data-frontmatter-key]');
      const valueInput = row.querySelector('[data-frontmatter-value]');
      if (keyInput) keyInput.value = key || '';
      if (valueInput) valueInput.value = value || '';
      rows.appendChild(row);
      updateSummary(row);
      if (dialog.open && valueInput) resizeValue(valueInput);
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
      if (dialog.open) return;
      rows.replaceChildren();
      originalEntries.forEach(function (entry) { addRow(entry.key, entry.value); });
      if (!rows.querySelector('[data-frontmatter-row]')) addRow();
      setStatus('');
      dialog.showModal();
      dialog.focus();
      resizeObserver.observe(rows);
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
    form.addEventListener('submit', function (event) {
      event.preventDefault();
      if (saveBtn && saveBtn.disabled) return;
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

// Map normalized matches back to UTF-16 offsets so Czech accents and combining
// marks cannot shift highlights in the original DOM text.
function searchMatchRanges(value, query) {
  const fold = text => text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
  const terms = Array.from(query.matchAll(/"([^"]+)"|(\S+)/g), match =>
    fold(match[1] || match[2]).replace(/\s+/g, ' ').trim()).filter(Boolean).slice(0, 32);
  let normalized = '';
  let offset = 0;
  const positions = [];
  for (const character of value) {
    const start = offset;
    offset += character.length;
    let part = fold(character);
    if (/^\s+$/.test(part)) part = normalized.endsWith(' ') ? '' : ' ';
    for (let i = 0; i < part.length; i++) positions.push({ start, end: offset });
    normalized += part;
    if (!part && positions.length) positions[positions.length - 1].end = offset;
  }
  const ranges = [];
  const words = Array.from(value.matchAll(/[\p{L}\p{N}\p{M}_]+/gu), match =>
    ({ start: match.index, end: match.index + match[0].length }));
  for (const term of terms) {
    let index = normalized.indexOf(term);
    while (index !== -1) {
      const range = { start: positions[index].start, end: positions[index + term.length - 1].end };
      // Prefixes should mark the complete word, not leave its suffix outside.
      for (const word of words) {
        if (word.start < range.start && range.start < word.end) range.start = word.start;
        if (word.start < range.end && range.end < word.end) range.end = word.end;
      }
      ranges.push(range);
      index = normalized.indexOf(term, index + term.length);
    }
  }
  ranges.sort((a, b) => a.start - b.start);
  const merged = [];
  for (const range of ranges) {
    const last = merged[merged.length - 1];
    if (last && range.start <= last.end) last.end = Math.max(last.end, range.end);
    else merged.push(range);
  }
  return merged;
}

function applySearchHighlights() {
  clearSearchHighlights();

  const query = getSearchQuery();
  if (!query) return;

  const prose = document.querySelector('.dyno-prose');
  if (!prose) return;

  const walker = document.createTreeWalker(prose, NodeFilter.SHOW_TEXT, {
    acceptNode: function (node) {
      if (!node.nodeValue || !node.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
      if (shouldSkipHighlightNode(node)) return NodeFilter.FILTER_REJECT;
      if (!searchMatchRanges(node.nodeValue, query).length) return NodeFilter.FILTER_REJECT;
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
    const fragment = document.createDocumentFragment();
    let lastIndex = 0;
    searchMatchRanges(text, query).forEach(function (range) {
      if (range.start > lastIndex) {
        fragment.appendChild(document.createTextNode(text.slice(lastIndex, range.start)));
      }
      const mark = document.createElement('mark');
      mark.setAttribute('data-search-highlight', '1');
      mark.textContent = text.slice(range.start, range.end);
      if (!firstMark) firstMark = mark;
      fragment.appendChild(mark);
      lastIndex = range.end;
    });

    if (lastIndex < text.length) {
      fragment.appendChild(document.createTextNode(text.slice(lastIndex)));
    }
    if (node.parentNode) {
      node.parentNode.replaceChild(fragment, node);
    }
  });

  const anchor = searchHeadingTarget();
  if (anchor) {
    anchor.scrollIntoView({ block: 'start', behavior: 'smooth' });
  } else if (firstMark) {
    firstMark.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }
}

function searchHeadingTarget() {
  if (!window.location.hash) return null;
  try { return document.getElementById(decodeURIComponent(window.location.hash.slice(1))); }
  catch (_) { return null; }
}

function searchResultsURL(endpoint, query, currentURL) {
  const target = new URL(endpoint, currentURL);
  target.searchParams.set('q', query.trim());
  target.searchParams.delete('page');
  return target.pathname + target.search;
}

// Pins contain only document-relative paths, never an origin, query or anchor.
function pinnedDocumentURL(base, path, origin) {
  if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//') || /[\\?#\u0000-\u001f]/.test(path)) return null;
  try {
    const decoded = decodeURIComponent(path);
    if (decoded.includes('\\') || decoded.split('/').some(segment => segment === '.' || segment === '..')) return null;
    const url = new URL((base || '') + path, origin);
    if (url.origin !== new URL(origin).origin || (base && url.pathname !== base && !url.pathname.startsWith(base + '/'))) return null;
    return url.pathname;
  } catch (_) { return null; }
}

function parsePinnedDocuments(raw) {
  let entries;
  try { entries = JSON.parse(raw); } catch (_) { return []; }
  if (!Array.isArray(entries)) return [];
  const seen = new Set();
  return entries.filter(entry => entry && typeof entry.path === 'string' && typeof entry.title === 'string' && entry.title.trim())
    .map(entry => ({ path: entry.path, title: entry.title.slice(0, 500), pinnedAt: Number.isSafeInteger(entry.pinnedAt) ? entry.pinnedAt : 0, group: typeof entry.group === 'string' ? entry.group : '' }))
    .sort((a, b) => b.pinnedAt - a.pinnedAt || a.path.localeCompare(b.path))
    .filter(entry => { if (seen.has(entry.path)) return false; seen.add(entry.path); return true; });
}

function parsePinnedCollection(raw, legacy) {
  let saved;
  try { saved = JSON.parse(raw); } catch (_) {}
  if (!saved || Array.isArray(saved) || !Array.isArray(saved.entries)) {
    return { groups: [], entries: parsePinnedDocuments(legacy).map(entry => ({ ...entry, group: '' })) };
  }
  const ids = new Set();
  const groups = (Array.isArray(saved.groups) ? saved.groups : []).filter(group => {
    if (!group || typeof group.id !== 'string' || !group.id || group.id.length > 128 || ids.has(group.id) || typeof group.name !== 'string' || !group.name.trim()) return false;
    ids.add(group.id); return true;
  }).map(group => ({ id: group.id, name: group.name.trim().slice(0, 80) }));
  const entries = parsePinnedDocuments(JSON.stringify(saved.entries)).map(entry => ({ ...entry, group: ids.has(entry.group) ? entry.group : '' }));
  return { groups, entries };
}

function filterPinnedDocuments(entries, query, order) {
  const fold = value => value.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
  const terms = fold(query.trim()).split(/\s+/).filter(Boolean);
  return entries.filter(entry => {
    let path = entry.path;
    try { path = decodeURIComponent(path); } catch (_) {}
    const searchable = fold(entry.title + ' ' + path);
    return terms.every(term => searchable.includes(term));
  })
    .sort((a, b) => order === 'name'
      ? a.title.localeCompare(b.title, undefined, { sensitivity: 'base', numeric: true }) || a.path.localeCompare(b.path)
      : b.pinnedAt - a.pinnedAt || a.title.localeCompare(b.title));
}

function movePinnedDocument(collection, path, group) {
  if (group && !collection.groups.some(item => item.id === group)) return collection;
  return { ...collection, entries: collection.entries.map(entry => entry.path === path ? { ...entry, group } : entry) };
}

function deletePinnedGroup(collection, id) {
  return { groups: collection.groups.filter(group => group.id !== id), entries: collection.entries.map(entry => entry.group === id ? { ...entry, group: '' } : entry) };
}

function initPinnedDocuments() {
  const dialog = document.querySelector('[data-pins-dialog]');
  if (!dialog || dialog.dataset.pinsBound) return;
  dialog.dataset.pinsBound = '1';
  let sources;
  try { sources = JSON.parse(dialog.dataset.pinsSources); } catch (_) { return; }
  if (!Array.isArray(sources) || !sources.length) return;
  const list = dialog.querySelector('[data-pins-list]');
  const close = dialog.querySelector('[data-pins-close]');
  const filter = dialog.querySelector('[data-pins-filter]');
  const groupFilter = dialog.querySelector('[data-pins-group-filter]');
  const sort = dialog.querySelector('[data-pins-sort]');
  const editor = dialog.querySelector('[data-pins-group-editor]');
  const nameInput = dialog.querySelector('[data-pins-group-name]');
  const sourceInput = dialog.querySelector('[data-pins-group-source]');
  const moveEditor = dialog.querySelector('[data-pins-move-editor]');
  const moveTarget = dialog.querySelector('[data-pins-move-target]');
  const temporary = new Map();
  const collapsed = new Set();
  let notice = '';
  let message = '';
  let selectedGroup = 'all';
  let editing = null;
  let moving = null;
  let dragged = null;
  const key = source => 'dyno:pins:v2:' + source.id;
  const legacyKey = source => 'dyno:pins:v1:' + source.id;
  const groupKey = (source, id) => JSON.stringify([source.id, id]);
  const option = (value, label) => {
    const item = document.createElement('option'); item.value = value; item.textContent = label; return item;
  };
  const read = source => {
    if (temporary.has(source.id)) return temporary.get(source.id);
    try { return parsePinnedCollection(localStorage.getItem(key(source)), localStorage.getItem(legacyKey(source))); }
    catch (_) { notice = 'Browser storage is unavailable. Changes will last only until this page is closed.'; return { groups: [], entries: [] }; }
  };
  const write = (source, collection) => {
    // A single v2 write keeps assignments and groups atomic. The v1 list is
    // retained as a backup and never overwritten during migration.
    try { localStorage.setItem(key(source), JSON.stringify(collection)); temporary.delete(source.id); notice = ''; }
    catch (_) { temporary.set(source.id, collection); notice = 'Pins could not be saved. Changes will last only until this page is closed.'; }
  };
  const current = () => {
    const marker = document.querySelector('#page-content [data-pin-document]');
    if (!marker) return null;
    const path = marker.dataset.pinPath;
    if (!pinnedDocumentURL(sources[0].base, path, window.location.href)) return null;
    return { path, title: marker.dataset.pinTitle || path };
  };
  const icon = (path, strokeWidth = 1.7) => {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    for (const [key, value] of Object.entries({ viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': String(strokeWidth), 'aria-hidden': 'true' })) svg.setAttribute(key, value);
    const shape = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    shape.setAttribute('stroke-linecap', 'round'); shape.setAttribute('stroke-linejoin', 'round'); shape.setAttribute('d', path);
    svg.append(shape); return svg;
  };
  const iconButton = (label, path, strokeWidth = 1.7) => {
    const button = document.createElement('button'); button.type = 'button'; button.className = 'button dyno-icon-button is-ghost';
    button.title = label; button.setAttribute('aria-label', label); button.append(icon(path, strokeWidth)); return button;
  };
  const closeEditors = () => { editor.hidden = true; moveEditor.hidden = true; editing = null; moving = null; };
  const openGroupEditor = (source, group) => {
    closeEditors(); editing = { source, id: group ? group.id : '' };
    sourceInput.replaceChildren(...sources.map(item => option(item.id, item.label)));
    sourceInput.value = source.id; sourceInput.disabled = Boolean(group);
    dialog.querySelector('[data-pins-source-label]').hidden = sources.length === 1;
    dialog.querySelector('[data-pins-group-editor-label]').textContent = group ? 'Manage group' : 'New group';
    dialog.querySelector('[data-pins-delete-group]').hidden = !group;
    dialog.querySelector('[data-pins-group-delete-note]').hidden = !group;
    dialog.querySelector('[data-pins-group-error]').hidden = true;
    nameInput.value = group ? group.name : ''; editor.hidden = false; nameInput.focus();
  };
  const move = (source, path, target) => {
    const collection = read(source);
    if (!collection.entries.some(entry => entry.path === path) || (target && !collection.groups.some(group => group.id === target))) {
      message = 'The document or destination has changed. Please choose again.'; closeEditors(); refresh(); groupFilter.focus(); return;
    }
    write(source, movePinnedDocument(collection, path, target));
    collapsed.delete(groupKey(source, target));
    message = 'Document moved to ' + (collection.groups.find(group => group.id === target)?.name || 'Unsorted') + '.';
    closeEditors(); refresh(source.id + '\n' + path);
  };
  const openMoveEditor = entry => {
    closeEditors(); moving = entry;
    const collection = read(entry.source);
    moveTarget.replaceChildren(option('', 'Unsorted'), ...collection.groups.map(group => option(group.id, group.name)));
    moveTarget.value = entry.group;
    dialog.querySelector('[data-pins-move-title]').textContent = entry.title;
    moveEditor.hidden = false; moveTarget.focus();
  };
  const refresh = focusKey => {
    const page = current();
    const entries = [];
    const groups = [];
    const scopePaths = new Set();
    const scopeGroups = new Set();
    sources.forEach(source => {
      const collection = read(source);
      [{ id: '', name: 'Unsorted' }, ...collection.groups].forEach(group => {
        const key = groupKey(source, group.id);
        if (!scopeGroups.has(key)) { scopeGroups.add(key); groups.push({ ...group, source, key }); }
      });
      collection.entries.forEach(entry => {
        const url = pinnedDocumentURL(source.base, entry.path, window.location.href);
        const id = source.id + '\n' + entry.path;
        if (url && !scopePaths.has(id)) { scopePaths.add(id); entries.push({ ...entry, source, url }); }
      });
    });
    groupFilter.replaceChildren(option('all', 'All groups'), ...groups.map(group => option(group.key, (sources.length > 1 ? group.source.label + ' / ' : '') + group.name)));
    if (!groups.some(group => group.key === selectedGroup)) selectedGroup = 'all';
    groupFilter.value = selectedGroup;
    const matching = filterPinnedDocuments(entries, filter.value, sort.value);
    const pinned = page && entries.some(entry => entry.source.id === sources[0].id && entry.path === page.path);
    document.querySelectorAll('[data-pin-toggle]').forEach(button => {
      button.hidden = !page;
      button.setAttribute('aria-pressed', pinned ? 'true' : 'false');
      button.setAttribute('aria-label', pinned ? 'Unpin document' : 'Pin document');
      button.title = pinned ? 'Unpin document' : 'Pin document';
      const label = button.querySelector('[data-pin-toggle-label]');
      if (label) label.textContent = pinned ? 'Unpin current document' : 'Pin current document';
    });
    document.querySelectorAll('[data-pins-open]').forEach(button => {
      const count = button.querySelector('[data-pins-count]');
      count.textContent = entries.length;
      count.hidden = !entries.length;
      button.setAttribute('aria-label', entries.length ? 'Pinned documents (' + entries.length + ')' : 'Pinned documents');
    });
    list.replaceChildren();
    const rowLinks = new Map();
    let visible = 0;
    groups.forEach((group, index) => {
      if (selectedGroup !== 'all' && selectedGroup !== group.key) return;
      const members = matching.filter(entry => entry.source.id === group.source.id && entry.group === group.id);
      visible += members.length;
      const section = document.createElement('section'); section.className = 'dyno-pins-group'; section.dataset.pinGroup = group.key;
      const header = document.createElement('div'); header.className = 'dyno-pins-group-header';
      const toggle = document.createElement('button'); toggle.type = 'button'; toggle.className = 'dyno-pins-group-toggle';
      const groupList = document.createElement('ul'); groupList.className = 'dyno-unstyled dyno-pins-list'; groupList.id = 'dyno-pin-group-' + index;
      toggle.setAttribute('aria-controls', groupList.id); toggle.setAttribute('aria-expanded', String(!collapsed.has(group.key)));
      toggle.append(icon('M9 5l7 7-7 7'));
      const label = document.createElement('span'); label.textContent = (sources.length > 1 ? group.source.label + ' / ' : '') + group.name;
      const count = document.createElement('small');
      const total = entries.filter(entry => entry.source.id === group.source.id && entry.group === group.id).length;
      count.textContent = filter.value.trim() ? members.length + '/' + total : String(total);
      toggle.append(label, count); groupList.hidden = collapsed.has(group.key);
      toggle.addEventListener('click', () => {
        if (collapsed.has(group.key)) collapsed.delete(group.key); else collapsed.add(group.key);
        groupList.hidden = collapsed.has(group.key); toggle.setAttribute('aria-expanded', String(!groupList.hidden));
      });
      header.append(toggle);
      if (group.id) {
        const manage = iconButton('Manage group ' + group.name, 'M5 12h.01M12 12h.01M19 12h.01', 3);
        manage.addEventListener('click', () => openGroupEditor(group.source, group)); header.append(manage);
      }
      section.append(header, groupList);
      section.addEventListener('dragover', event => {
        if (!dragged || dragged.source !== group.source.id) return;
        event.preventDefault(); event.dataTransfer.dropEffect = 'move'; section.classList.add('is-drop-target');
      });
      section.addEventListener('dragleave', event => { if (!section.contains(event.relatedTarget)) section.classList.remove('is-drop-target'); });
      section.addEventListener('drop', event => {
        section.classList.remove('is-drop-target');
        if (!dragged || dragged.source !== group.source.id) return;
        event.preventDefault(); const path = dragged.path; dragged = null; move(group.source, path, group.id);
      });
      members.forEach(entry => {
        const row = document.createElement('li');
        const handle = document.createElement('span'); handle.className = 'dyno-pins-drag'; handle.draggable = true;
        handle.title = 'Drag to another group'; handle.setAttribute('aria-hidden', 'true');
        handle.append(icon('M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01', 2.6));
        handle.addEventListener('dragstart', event => {
          dragged = { source: entry.source.id, path: entry.path };
          event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('application/x-dyno-pin', entry.path);
        });
        handle.addEventListener('dragend', () => { dragged = null; dialog.querySelectorAll('.is-drop-target').forEach(target => target.classList.remove('is-drop-target')); });
        const link = document.createElement('a');
        link.href = entry.url; link.draggable = false;
        if (page && entry.source.id === sources[0].id && entry.path === page.path) link.setAttribute('aria-current', 'page');
        const title = document.createElement('span'); title.textContent = entry.title;
        const path = document.createElement('small'); path.textContent = entry.path; path.title = entry.path;
        link.append(title, path);
        const relocate = iconButton('Move ' + entry.title + ' to group', 'M5 12h.01M12 12h.01M19 12h.01', 3);
        relocate.addEventListener('click', () => openMoveEditor(entry));
        const remove = iconButton('Unpin ' + entry.title, 'M6 6l12 12M18 6L6 18'); remove.title = 'Unpin document';
        remove.addEventListener('click', () => {
          const collection = read(entry.source);
          write(entry.source, { ...collection, entries: collection.entries.filter(pin => pin.path !== entry.path) });
          refresh();
          groupFilter.focus();
        });
        link.addEventListener('click', () => dialog.close());
        row.append(handle, link, relocate, remove); groupList.append(row);
        rowLinks.set(entry.source.id + '\n' + entry.path, link);
      });
      if (!members.length) { const empty = document.createElement('li'); empty.className = 'dyno-pins-group-empty'; empty.textContent = 'Drop documents here, or use the move action.'; groupList.append(empty); }
      list.append(section);
    });
    const empty = dialog.querySelector('[data-pins-empty]');
    empty.hidden = visible > 0;
    empty.textContent = entries.length ? 'No pins match this filter.' : 'No pinned documents yet. Open a document and use the pin icon in the toolbar.';
    const status = dialog.querySelector('[data-pins-status]');
    status.textContent = [notice, message].filter(Boolean).join(' '); status.hidden = !status.textContent;
    if (typeof focusKey === 'string') (rowLinks.get(focusKey) || groupFilter).focus();
  };
  filter.addEventListener('input', () => refresh());
  groupFilter.addEventListener('change', () => { selectedGroup = groupFilter.value; refresh(); });
  sort.addEventListener('change', () => refresh());
  dialog.querySelector('[data-pins-new-group]').addEventListener('click', () => {
    const selected = selectedGroup === 'all' ? null : JSON.parse(selectedGroup);
    openGroupEditor(sources.find(source => source.id === selected?.[0]) || sources[0], null);
  });
  dialog.querySelectorAll('[data-pins-editor-cancel]').forEach(button => button.addEventListener('click', () => { closeEditors(); groupFilter.focus(); }));
  editor.addEventListener('submit', event => {
    event.preventDefault(); if (!editing) return;
    const source = sources.find(source => source.id === sourceInput.value); if (!source) return;
    const collection = read(source);
    const name = nameInput.value.trim().replace(/\s+/g, ' ').slice(0, 80);
    const normalized = name.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
    if (!name || normalized === 'unsorted' || collection.groups.some(group => group.id !== editing.id && group.name.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase() === normalized)) {
      const error = dialog.querySelector('[data-pins-group-error]'); error.textContent = 'Choose a non-empty, unique group name.'; error.hidden = false; nameInput.focus(); return;
    }
    if (editing.id && !collection.groups.some(group => group.id === editing.id)) { closeEditors(); refresh(); return; }
    const isNew = !editing.id;
    const id = editing.id || globalThis.crypto?.randomUUID?.() || Date.now().toString(36) + '-' + Math.random().toString(36).slice(2);
    const groups = editing.id ? collection.groups.map(group => group.id === id ? { ...group, name } : group) : [...collection.groups, { id, name }];
    write(source, { ...collection, groups });
    if (isNew) selectedGroup = 'all';
    message = 'Group saved.'; closeEditors(); refresh(); groupFilter.focus();
  });
  dialog.querySelector('[data-pins-delete-group]').addEventListener('click', () => {
    if (!editing?.id) return;
    const source = editing.source;
    write(source, deletePinnedGroup(read(source), editing.id));
    collapsed.delete(groupKey(source, ''));
    message = 'Group deleted. Its documents are now in Unsorted.'; closeEditors(); refresh(); groupFilter.focus();
  });
  moveEditor.addEventListener('submit', event => { event.preventDefault(); if (moving) move(moving.source, moving.path, moveTarget.value); });
  document.querySelectorAll('[data-pin-toggle]').forEach(button => button.addEventListener('click', () => {
    const page = current(); if (!page) return;
    const source = sources[0], collection = read(source);
    const exists = collection.entries.some(entry => entry.path === page.path);
    const next = collection.entries.filter(entry => entry.path !== page.path);
    if (!exists) next.unshift({ ...page, pinnedAt: Date.now(), group: '' });
    write(source, { ...collection, entries: next }); message = exists ? 'Document unpinned.' : 'Document pinned in Unsorted.'; refresh();
    if (notice && !dialog.open) dialog.showModal();
  }));
  document.querySelectorAll('[data-pins-open]').forEach(button => button.addEventListener('click', () => { refresh(); dialog.showModal(); }));
  close.addEventListener('click', () => dialog.close());
  dialog.addEventListener('close', () => { closeEditors(); dragged = null; dialog.querySelectorAll('.is-drop-target').forEach(target => target.classList.remove('is-drop-target')); });
  dialog.addEventListener('click', event => { if (event.target === dialog) dialog.close(); });
  document.addEventListener('htmx:afterSettle', refresh);
  document.addEventListener('htmx:historyRestore', refresh);
  window.addEventListener('storage', event => { if (event.key === null || sources.some(source => key(source) === event.key || legacyKey(source) === event.key)) refresh(); });
  refresh();
}

document.addEventListener('DOMContentLoaded', initPinnedDocuments);

function initMetadataFilters() {
  let restoreFocus = false;
  document.addEventListener('click', function (event) {
    const dialog = document.getElementById('metadata-filter-dialog');
    if (!dialog) return;
    if (event.target.closest('[data-metadata-filters-open]')) {
      dialog.querySelector('form').reset();
      dialog.showModal();
    } else if (event.target.closest('[data-metadata-filters-close]') || event.target === dialog) {
      dialog.close();
    } else if (event.target.closest('[data-metadata-filters-clear]')) {
      dialog.querySelectorAll('input[type="checkbox"]').forEach(input => { input.checked = false; });
    }
  });
  document.addEventListener('close', function (event) {
    if (event.target.id === 'metadata-filter-dialog') event.target.querySelector('form').reset();
  }, true);
  document.addEventListener('htmx:beforeSwap', function (event) {
    const dialog = document.getElementById('metadata-filter-dialog');
    if (event.detail.target?.id === 'page-content' && event.detail.shouldSwap && dialog?.open) {
      restoreFocus = true;
      dialog.close();
    }
  });
  document.addEventListener('htmx:afterSettle', function () {
    if (restoreFocus) {
      document.querySelector('[data-metadata-filters-open]')?.focus({ preventScroll: true });
      restoreFocus = false;
    }
  });
}
document.addEventListener('DOMContentLoaded', initMetadataFilters);

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
    if (window.DynoSyntax) DynoSyntax.highlightAll(e.target);
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
      const expanded = document.querySelector('#document-expanded[open] .dyno-document');
      (expanded || window).scrollTo({ top: 0, behavior: 'smooth' });
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
    container.querySelectorAll('.is-keyboard-active').forEach(link => link.classList.remove('is-keyboard-active'));
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
    if (e.isComposing) return;
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

    // Enter opens the focused hit, or the full results for the current input.
    if (e.key === 'Enter' && document.activeElement === input && input.value.trim()) {
      e.preventDefault();
      const active = !container.classList.contains('hidden') && container.querySelector('.search-result-link.is-keyboard-active');
      if (active) {
        active.click();
      } else {
        window.location.assign(searchResultsURL(input.getAttribute('hx-get'), input.value, window.location.href));
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

  var bg = dynoThemeColor('--dyno-page-bg');
  var nodeFill = dynoThemeColor('--dyno-primary-bg');
  var nodeStroke = dynoThemeColor('--dyno-primary');
  var nodeText = dynoThemeColor('--dyno-primary-text');
  var edgeColor = dynoThemeColor('--dyno-border');

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
    ctx.fillStyle = dynoThemeColor('--dyno-danger');
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

document.addEventListener('DOMContentLoaded', function () {
  var statusURL = document.body.dataset.browseStatus;
  if (!statusURL) return;
  var revision = Number(document.body.dataset.browseRevision);
  var scrollKey = 'dyno-browse-scroll:' + window.location.pathname;
  try {
    var previousScroll = sessionStorage.getItem(scrollKey);
    if (previousScroll !== null) {
      sessionStorage.removeItem(scrollKey);
      requestAnimationFrame(function () { window.scrollTo(0, Number(previousScroll)); });
    }
  } catch (_) {}
  function poll() {
    if (document.hidden) { setTimeout(poll, 1500); return; }
    fetch(statusURL, { cache: 'no-store' }).then(function (response) {
      if (!response.ok) throw new Error('Browse status unavailable');
      return response.json();
    }).then(function (status) {
      if (status.revision !== revision) {
        try { sessionStorage.setItem('dyno-browse-scroll:' + window.location.pathname, String(window.scrollY)); } catch (_) {}
        window.location.reload();
        return;
      }
      setTimeout(poll, 1500);
    }).catch(function () { setTimeout(poll, 1500); });
  }
  setTimeout(poll, 1500);
});
