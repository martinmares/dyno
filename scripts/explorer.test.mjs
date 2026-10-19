import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../assets/explorer.js', import.meta.url), 'utf8');

function setup(url, canonical, storage = new Map()) {
  const handlers = {};
  const panes = ['sidebar', 'results', 'preview'].map(name => ({ dataset: { explorerScroll: name }, scrollTop: 0 }));
  let focused = 0;
  vm.runInNewContext(source, {
    URL,
    window: { location: { href: url }, addEventListener: (name, fn) => { handlers[name] = fn; } },
    document: {
      querySelector: () => ({ dataset: { explorerState: canonical } }),
      querySelectorAll: () => panes,
      getElementById: () => ({ focus() { focused++; } }),
      addEventListener: (name, fn) => { handlers[name] = fn; },
    },
    sessionStorage: { getItem: key => storage.get(key), setItem: (key, value) => storage.set(key, value) },
  });
  handlers.DOMContentLoaded();
  return { panes, handlers, focused: () => focused };
}

test('canonical state restores results after the first selection and resets a different preview', () => {
  const storage = new Map();
  const first = setup('http://localhost/_explore', 'columns_set=1&page=1&sort=path', storage);
  first.panes[0].scrollTop = 120;
  first.panes[1].scrollTop = 480;
  first.panes[2].scrollTop = 230;
  first.handlers.pagehide();
  const selected = setup('http://localhost/_explore?selected=/guide', 'columns_set=1&page=1&sort=path&selected=%2Fguide', storage);
  assert.equal(selected.panes[0].scrollTop, 120);
  assert.equal(selected.panes[1].scrollTop, 480);
  assert.equal(selected.panes[2].scrollTop, 0);
  selected.panes[2].scrollTop = 180;
  selected.handlers.pagehide();
  const back = setup('http://localhost/_explore?selected=/guide', 'columns_set=1&page=1&sort=path&selected=%2Fguide', storage);
  assert.equal(back.panes[2].scrollTop, 180);
});

test('slash focuses search without interfering with editing', () => {
  const state = setup('http://localhost/_explore', '');
  let prevented = false;
  state.handlers.keydown({ key: '/', target: { closest: () => null }, preventDefault() { prevented = true; } });
  assert.equal(state.focused(), 1);
  assert.equal(prevented, true);
  state.handlers.keydown({ key: '/', target: { closest: () => ({}) } });
  assert.equal(state.focused(), 1);
});

test('reader navigation propagates Explorer context only to page GET requests', () => {
  const app = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
  const contextSource = app.slice(app.indexOf('// Keep the Explorer selection'), app.indexOf('// ─── Document printing'));
  const handlers = {};
  const link = { href: 'http://localhost/manual/_explore' };
  const context = {
    URL, URLSearchParams,
    window: { location: { search: '?explore=' + encodeURIComponent('q=database&folder=docs') } },
    document: { querySelector: () => link, addEventListener(name, fn) { handlers[name] = fn; } },
  };
  vm.runInNewContext(contextSource, context);
  const detail = { verb: 'get', target: { id: 'page-content' }, parameters: {} };
  handlers['htmx:configRequest']({ detail });
  assert.equal(detail.parameters.explore, 'q=database&folder=docs');
  const post = { verb: 'post', target: { id: 'page-content' }, parameters: {} };
  handlers['htmx:configRequest']({ detail: post });
  assert.equal(post.parameters.explore, undefined);
  handlers['htmx:afterSettle']();
  assert.equal(link.href, 'http://localhost/manual/_explore?q=database&folder=docs');
});
