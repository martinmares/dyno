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

  // Close sidebar on nav link click (mobile)
  sidebar.querySelectorAll('.nav-link').forEach(function (link) {
    link.addEventListener('click', function () {
      if (window.innerWidth < 1024) closeSidebar();
    });
  });
});

// ─── Active nav link ──────────────────────────────────────────────────────────
// Uses data-active attribute so dark/light styling is handled purely in CSS.

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

document.addEventListener('DOMContentLoaded', function () {
  initMermaid();
  updateActiveNavLink(window.location.pathname);
  initTOCScrollSpy();
  initCopyButtons();
  initLightbox();
});

// ─── HTMX hooks ───────────────────────────────────────────────────────────────

document.addEventListener('htmx:afterSwap', function (e) {
  if (e.target.id === 'page-content') {
    initMermaid();
    initTOCScrollSpy();
    initCopyButtons();
    initLightbox();
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }
  if (e.target.id === 'page-content' || e.target.id === 'search-results-container') {
    updateActiveNavLink(window.location.pathname);
  }
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
