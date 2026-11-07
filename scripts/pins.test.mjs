import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const app = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
const context = vm.createContext({ URL });
vm.runInContext(app.slice(app.indexOf('function pinnedDocumentURL('), app.indexOf('function initPinnedDocuments(')), context);

test('pin URLs use the current mount and cannot navigate outside the documentation', () => {
  const resolve = (path, base = '/docs') => context.pinnedDocumentURL(base, path, 'http://localhost:3000/docs/');
  assert.equal(resolve('/guide'), '/docs/guide');
  assert.equal(resolve('/'), '/docs/');
  assert.equal(resolve('/guide', ''), '/guide');
  assert.equal(resolve('/České%20poznámky'), '/docs/%C4%8Cesk%C3%A9%20pozn%C3%A1mky');
  for (const path of ['//evil.test/path', 'https://evil.test', '/../../secret', '/%2e%2e/secret', '/x/%2f../secret', '/x\\secret', '/guide?q=private', '/guide#section', '/a\nb', '/bad%']) {
    assert.equal(resolve(path), null, path);
  }
});

test('pin entries tolerate broken storage and sort and deduplicate recent pins', () => {
  for (const raw of [null, '', '{broken', '{}', 'null', '42']) assert.equal(context.parsePinnedDocuments(raw).length, 0);
  const parsed = context.parsePinnedDocuments(JSON.stringify([
    { path: '/one', title: 'Old', pinnedAt: 1 },
    { path: '/two', title: '<script>literal title</script>', pinnedAt: 3 },
    { path: '/one', title: 'Updated', pinnedAt: 5 },
    { path: '/ignored', title: '' }, null,
  ]));
  assert.equal(parsed.length, 2);
  assert.equal(parsed[0].title, 'Updated');
  assert.equal(parsed[1].title, '<script>literal title</script>');
});

test('pin labels are bounded without silently dropping older pins or trusting record properties', () => {
  const records = Array.from({ length: 550 }, (_, i) => ({ path: '/'+i, title: 'X'.repeat(800), pinnedAt: i, href: 'javascript:alert(1)' }));
  const parsed = context.parsePinnedDocuments(JSON.stringify(records));
  assert.equal(parsed.length, 550);
  assert.equal(parsed[0].path, '/549');
  assert.equal(parsed[0].title.length, 500);
  assert.equal(parsed[0].href, undefined);
});

function setupPins() {
  const key = 'dyno:pins:v1:site-a';
  const storage = new Map();
  const events = new Map(), windowEvents = new Map();
  let blocked = false;
  const on = (target, name, fn) => { if (!target.has(name)) target.set(name, []); target.get(name).push(fn); };
  class Element {
    constructor(tag = 'div') { this.tag = tag; this.dataset = {}; this.attrs = {}; this.children = []; this.events = new Map(); }
    setAttribute(name, value) { this.attrs[name] = value; }
    addEventListener(name, fn) { on(this.events, name, fn); }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    querySelectorAll(tag) { return this.children.flatMap(child => [ ...(child.tag === tag ? [child] : []), ...child.querySelectorAll(tag) ]); }
    querySelector() { return null; }
    click() { for (const fn of this.events.get('click') || []) fn({ target: this }); }
    focus() { this.focused = true; }
    showModal() { this.open = true; }
    close() { this.open = false; }
  }
  const dialog = new Element(), list = new Element('ul'), empty = new Element(), status = new Element(), close = new Element('button');
  const toggle = new Element('button'), open = new Element('button'), count = new Element();
  const marker = new Element(); marker.dataset = { pinPath: '/one', pinTitle: 'One' };
  dialog.dataset.pinsSources = JSON.stringify([{ id: 'site-a', base: '/docs', label: 'Docs' }]);
  dialog.querySelector = selector => ({ '[data-pins-list]': list, '[data-pins-empty]': empty, '[data-pins-status]': status, '[data-pins-close]': close }[selector]);
  open.querySelector = () => count;
  const document = {
    querySelector: selector => selector === '[data-pins-dialog]' ? dialog : selector.includes('data-pin-document') ? marker : null,
    querySelectorAll: selector => selector === '[data-pin-toggle]' ? [toggle] : selector === '[data-pins-open]' ? [open] : [],
    createElement: tag => new Element(tag), createElementNS: (_, tag) => new Element(tag), addEventListener: (name, fn) => on(events, name, fn),
  };
  const ctx = vm.createContext({ URL, document, window: { location: { href: 'http://localhost:3000/docs/one?q=ignored#chapter' }, addEventListener: (name, fn) => on(windowEvents, name, fn) },
    localStorage: { getItem: name => { if (blocked) throw Error('blocked'); return storage.get(name) || null; }, setItem: (name, value) => { if (blocked) throw Error('blocked'); storage.set(name, value); } },
  });
  vm.runInContext(app.slice(app.indexOf('function pinnedDocumentURL('), app.indexOf("document.addEventListener('DOMContentLoaded', initPinnedDocuments)")), ctx);
  ctx.initPinnedDocuments();
  return { key, storage, list, empty, status, toggle, open, count, marker, dialog,
    refresh: () => { for (const fn of events.get('htmx:afterSettle') || []) fn(); },
    blockStorage: () => { blocked = true; },
    storageEvent: () => { for (const fn of windowEvents.get('storage') || []) fn({ key }); },
  };
}

test('pin/unpin follows HTMX documents and never saves filters or anchors', () => {
  const s = setupPins();
  s.toggle.click();
  assert.equal(s.toggle.attrs['aria-pressed'], 'true');
  assert.equal(s.count.textContent, 1);
  assert.equal(JSON.parse(s.storage.get(s.key))[0].path, '/one');
  s.marker.dataset = { pinPath: '/two', pinTitle: 'Two' }; s.refresh();
  assert.equal(s.toggle.attrs['aria-pressed'], 'false');
  s.toggle.click();
  assert.equal(s.list.children.length, 2);
  s.open.click();
  assert.equal(s.dialog.open, true);
  const unpin = s.list.querySelectorAll('button')[0];
  assert.equal(unpin.title, 'Unpin document');
  assert.match(unpin.attrs['aria-label'], /^Unpin /);
  assert.equal(unpin.children[0].tag, 'svg');
  assert.equal(unpin.children[0].attrs['aria-hidden'], 'true');
  s.list.querySelectorAll('button')[0].click();
  assert.equal(s.list.children.length, 1);
  assert.equal(s.count.textContent, 1);
});

test('other documentation is ignored and storage events update the current list safely', () => {
  const s = setupPins();
  s.storage.set('dyno:pins:v1:other-site', JSON.stringify([{ path: '/secret', title: 'Other documentation', pinnedAt: 5 }]));
  s.storageEvent();
  assert.equal(s.list.children.length, 0);
  s.storage.set(s.key, JSON.stringify([{ path: '/one', title: '<script>literal</script>', pinnedAt: 6 }, { path: '//evil.test', title: 'Bad', pinnedAt: 7 }]));
  s.storageEvent();
  assert.equal(s.list.children.length, 1);
  assert.equal(s.toggle.attrs['aria-pressed'], 'true');
  assert.equal(s.list.children[0].children[0].children[0].textContent, '<script>literal</script>');
  assert.equal(s.list.children[0].children[0].href, '/docs/one');
  s.storage.delete(s.key); s.storageEvent();
  assert.equal(s.list.children.length, 0);
  assert.equal(s.toggle.attrs['aria-pressed'], 'false');
});

test('blocked storage keeps pins temporarily and explains the persistence limitation', () => {
  const s = setupPins(); s.blockStorage(); s.toggle.click();
  assert.equal(s.list.children.length, 1);
  assert.equal(s.toggle.attrs['aria-pressed'], 'true');
  assert.equal(s.dialog.open, true);
  assert.match(s.status.textContent, /could not be saved/);
  s.refresh();
  assert.equal(s.list.children.length, 1);
  s.list.querySelectorAll('button')[0].click();
  assert.equal(s.list.children.length, 0);
  assert.equal(s.empty.hidden, false);
});
