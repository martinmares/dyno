// Explorer uses ordinary GET links/forms: filters, selection and pagination
// work without JavaScript and remain bookmarkable. Only pane scroll is local.
document.addEventListener('DOMContentLoaded', function () {
  const root = document.querySelector('[data-explorer]');
  if (!root) return;
  const state = new URL(window.location.href);
  state.search = root.dataset.explorerState;
  const selected = state.searchParams.get('selected') || '';
  state.searchParams.delete('selected');
  const key = 'dyno-explorer-scroll:' + state.pathname + '?' + state.searchParams.toString();
  const panes = Array.from(document.querySelectorAll('[data-explorer-scroll]'));
  try {
    const saved = JSON.parse(sessionStorage.getItem(key) || '{}');
    panes.forEach(function (pane) {
      const name = pane.dataset.explorerScroll;
      pane.scrollTop = name === 'preview' && saved.selected !== selected ? 0 : saved[name] || 0;
    });
  } catch (_) {}
  window.addEventListener('pagehide', function () {
    const saved = { selected: selected };
    panes.forEach(function (pane) { saved[pane.dataset.explorerScroll] = pane.scrollTop; });
    try { sessionStorage.setItem(key, JSON.stringify(saved)); } catch (_) {}
  });
  document.addEventListener('keydown', function (event) {
    if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey || event.target.closest('input, textarea, select, [contenteditable]')) return;
    event.preventDefault();
    document.getElementById('explorer-query').focus();
  });
});
