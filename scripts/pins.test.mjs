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

function setupPins(initial = [], sources = [{ id: 'site-a', base: '/docs', label: 'Docs' }]) {
  const key = 'dyno:pins:v2:site-a', legacyKey = 'dyno:pins:v1:site-a';
  const storage = new Map(initial);
  const events = new Map(), windowEvents = new Map();
  let blocked = false;
  const on = (target, name, fn) => { if (!target.has(name)) target.set(name, []); target.get(name).push(fn); };
  class Element {
    constructor(tag = 'div') {
      this.tag = tag; this.dataset = {}; this.attrs = {}; this.children = []; this.events = new Map(); this.value = ''; this.className = '';
      this.classList = { add: value => { this.className += ' ' + value; }, remove: value => { this.className = this.className.split(' ').filter(name => name !== value).join(' '); } };
    }
    setAttribute(name, value) { this.attrs[name] = value; }
    addEventListener(name, fn) { on(this.events, name, fn); }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    querySelectorAll(selector) { return this.children.flatMap(child => [ ...((child.tag === selector || selector.startsWith('.') && child.className.split(' ').includes(selector.slice(1))) ? [child] : []), ...child.querySelectorAll(selector) ]); }
    querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
    contains(target) { return this === target || this.children.some(child => child.contains(target)); }
    emit(name, event = {}) { for (const fn of this.events.get(name) || []) fn({ target: this, preventDefault() {}, ...event }); }
    click() { this.emit('click'); }
    focus() { this.focused = true; }
    showModal() { this.open = true; }
    close() { this.open = false; }
  }
  const dialog = new Element(), list = new Element('ul'), empty = new Element(), status = new Element(), close = new Element('button');
  const toggle = new Element('button'), open = new Element('button'), count = new Element();
  const marker = new Element(); marker.dataset = { pinPath: '/one', pinTitle: 'One' };
  dialog.dataset.pinsSources = JSON.stringify(sources);
  const controls = Object.fromEntries(['filter', 'group-filter', 'sort', 'group-editor', 'group-name', 'group-source', 'move-editor', 'move-target', 'source-label', 'group-editor-label', 'delete-group', 'group-delete-note', 'group-error', 'move-title', 'new-group', 'editor-cancel'].map(name => [name, new Element()]));
  controls.sort.value = 'recent';
  controls['group-editor'].hidden = controls['move-editor'].hidden = true;
  dialog.querySelector = selector => ({ '[data-pins-list]': list, '[data-pins-empty]': empty, '[data-pins-status]': status, '[data-pins-close]': close }[selector] || controls[selector.slice(11, -1)]);
  dialog.querySelectorAll = selector => selector === '[data-pins-editor-cancel]' ? [controls['editor-cancel']] : list.querySelectorAll(selector);
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
  return { key, legacyKey, storage, list, empty, status, toggle, open, count, marker, dialog, controls,
    entries: () => JSON.parse(storage.get(key)).entries,
    rows: () => list.querySelectorAll('li').filter(row => row.querySelector('a')),
    buttons: prefix => list.querySelectorAll('button').filter(button => button.attrs['aria-label']?.startsWith(prefix)),
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
  assert.equal(s.entries()[0].path, '/one');
  s.marker.dataset = { pinPath: '/two', pinTitle: 'Two' }; s.refresh();
  assert.equal(s.toggle.attrs['aria-pressed'], 'false');
  s.toggle.click();
  assert.equal(s.rows().length, 2);
  s.open.click();
  assert.equal(s.dialog.open, true);
  const unpin = s.buttons('Unpin ')[0];
  assert.equal(unpin.title, 'Unpin document');
  assert.match(unpin.attrs['aria-label'], /^Unpin /);
  assert.equal(unpin.children[0].tag, 'svg');
  assert.equal(unpin.children[0].attrs['aria-hidden'], 'true');
  unpin.click();
  assert.equal(s.rows().length, 1);
  assert.equal(s.count.textContent, 1);
});

test('other documentation is ignored and storage events update the current list safely', () => {
  const s = setupPins();
  s.storage.set('dyno:pins:v1:other-site', JSON.stringify([{ path: '/secret', title: 'Other documentation', pinnedAt: 5 }]));
  s.storageEvent();
  assert.equal(s.rows().length, 0);
  s.storage.set(s.key, JSON.stringify({ groups: [], entries: [{ path: '/one', title: '<script>literal</script>', pinnedAt: 6 }, { path: '//evil.test', title: 'Bad', pinnedAt: 7 }] }));
  s.storageEvent();
  assert.equal(s.rows().length, 1);
  assert.equal(s.toggle.attrs['aria-pressed'], 'true');
  assert.equal(s.rows()[0].querySelector('a').children[0].textContent, '<script>literal</script>');
  assert.equal(s.rows()[0].querySelector('a').href, '/docs/one');
  s.storage.delete(s.key); s.storageEvent();
  assert.equal(s.rows().length, 0);
  assert.equal(s.toggle.attrs['aria-pressed'], 'false');
});

test('blocked storage keeps pins temporarily and explains the persistence limitation', () => {
  const s = setupPins(); s.blockStorage(); s.toggle.click();
  assert.equal(s.rows().length, 1);
  assert.equal(s.toggle.attrs['aria-pressed'], 'true');
  assert.equal(s.dialog.open, true);
  assert.match(s.status.textContent, /could not be saved/);
  s.refresh();
  assert.equal(s.rows().length, 1);
  s.buttons('Unpin ')[0].click();
  assert.equal(s.rows().length, 0);
  assert.equal(s.empty.hidden, false);
});

test('legacy pins migrate to Unsorted on the first edit and the old list remains intact', () => {
  const legacy = JSON.stringify([{ path: '/one', title: 'One', pinnedAt: 8 }, { path: '/two', title: 'Two', pinnedAt: 4 }]);
  const s = setupPins([['dyno:pins:v1:site-a', legacy]]);
  assert.equal(s.rows().length, 2);
  assert.equal(s.storage.has(s.key), false);
  s.controls['new-group'].click(); s.controls['group-name'].value = 'Databases'; s.controls['group-editor'].emit('submit');
  const collection = JSON.parse(s.storage.get(s.key));
  assert.equal(collection.groups[0].name, 'Databases');
  assert.equal(collection.entries.length, 2);
  assert.equal(collection.entries[0].group, '');
  assert.equal(collection.entries[0].pinnedAt, 8);
  assert.equal(s.storage.get(s.legacyKey), legacy);
});

test('groups can be assigned, renamed, and deleted without unpinning documents', () => {
  const s = setupPins(); s.toggle.click();
  s.controls['new-group'].click(); s.controls['group-name'].value = 'Work'; s.controls['group-editor'].emit('submit');
  const id = JSON.parse(s.storage.get(s.key)).groups[0].id;
  s.buttons('Move One')[0].click(); s.controls['move-target'].value = id; s.controls['move-editor'].emit('submit');
  assert.equal(s.entries()[0].group, id);
  const pinnedAt = s.entries()[0].pinnedAt;
  s.buttons('Manage group Work')[0].click(); s.controls['group-name'].value = 'Active work'; s.controls['group-editor'].emit('submit');
  assert.equal(JSON.parse(s.storage.get(s.key)).groups[0].id, id);
  assert.equal(s.entries()[0].group, id);
  s.buttons('Manage group Active work')[0].click(); s.controls['delete-group'].click();
  assert.equal(JSON.parse(s.storage.get(s.key)).groups.length, 0);
  assert.equal(s.entries().length, 1);
  assert.equal(s.entries()[0].group, '');
  assert.equal(s.entries()[0].pinnedAt, pinnedAt);
  assert.equal(s.count.textContent, 1);
});

test('duplicate group names ignore case and Czech accents', () => {
  const s = setupPins();
  s.controls['new-group'].click(); s.controls['group-name'].value = 'Příprava'; s.controls['group-editor'].emit('submit');
  s.controls['new-group'].click(); s.controls['group-name'].value = 'PRIPRAVA'; s.controls['group-editor'].emit('submit');
  assert.equal(s.controls['group-error'].hidden, false);
  assert.equal(JSON.parse(s.storage.get(s.key)).groups.length, 1);
});

test('drag and drop moves into a collapsed group and preserves the pin timestamp', () => {
  const s = setupPins(); s.toggle.click();
  s.controls['new-group'].click(); s.controls['group-name'].value = 'Database'; s.controls['group-editor'].emit('submit');
  const id = JSON.parse(s.storage.get(s.key)).groups[0].id;
  const pinnedAt = s.entries()[0].pinnedAt;
  const target = s.list.children.find(group => JSON.parse(group.dataset.pinGroup)[1] === id);
  target.querySelector('button').click();
  assert.equal(target.querySelector('ul').hidden, true);
  const dataTransfer = { setData() {} };
  s.rows()[0].children[0].emit('dragstart', { dataTransfer });
  let accepted = false;
  target.emit('dragover', { dataTransfer, preventDefault() { accepted = true; } });
  assert.equal(accepted, true);
  target.emit('drop');
  assert.equal(s.entries()[0].group, id);
  assert.equal(s.entries()[0].pinnedAt, pinnedAt);
  const moved = s.list.children.find(group => JSON.parse(group.dataset.pinGroup)[1] === id);
  assert.equal(moved.querySelector('ul').hidden, false);
});

test('external drags and cross-documentation drops are ignored', () => {
  const s = setupPins([], [{ id: 'site-a', base: '/docs', label: 'A' }, { id: 'site-b', base: '/other', label: 'B' }]);
  s.toggle.click();
  const other = s.list.children.find(group => JSON.parse(group.dataset.pinGroup)[0] === 'site-b');
  let accepted = false;
  other.emit('dragover', { preventDefault() { accepted = true; } }); other.emit('drop');
  assert.equal(accepted, false);
  s.rows()[0].children[0].emit('dragstart', { dataTransfer: { setData() {} } });
  other.emit('dragover', { preventDefault() { accepted = true; } }); other.emit('drop');
  assert.equal(accepted, false);
  assert.equal(s.entries()[0].group, '');
  assert.equal(s.storage.has('dyno:pins:v2:site-b'), false);
});

test('filtering and sorting work within groups and never modify stored pins', () => {
  const entries = [{ path: '/Česká.md', title: 'Záznam', pinnedAt: 2 }, { path: '/other', title: 'Alpha', pinnedAt: 1 }];
  assert.equal(context.filterPinnedDocuments(entries, 'CESKA', 'recent')[0].title, 'Záznam');
  assert.equal(context.filterPinnedDocuments(entries, '', 'name')[0].title, 'Alpha');
  assert.equal(context.filterPinnedDocuments(entries, '', 'recent')[0].title, 'Záznam');
  assert.equal(entries[0].title, 'Záznam');
  const s = setupPins([['dyno:pins:v1:site-a', JSON.stringify(entries)]]);
  s.controls.filter.value = 'ceska'; s.controls.filter.emit('input');
  assert.equal(s.rows().length, 1);
  assert.equal(s.count.textContent, 2);
  assert.equal(s.storage.has(s.key), false);
  assert.equal(context.filterPinnedDocuments([{ title: 'Generic', path: '/%C4%8Desk%C3%A1/guide.md' }], 'CESKA', 'name').length, 1);
});

test('missing group references fall back to Unsorted without losing pins', () => {
  const collection = context.parsePinnedCollection(JSON.stringify({ groups: [{ id: 'g', name: 'One' }, { id: 'g', name: 'Duplicate' }, { id: '', name: 'Invalid' }], entries: [{ path: '/one', title: 'One', group: 'deleted', pinnedAt: 3 }] }), null);
  assert.equal(collection.groups.length, 1);
  assert.equal(collection.entries.length, 1);
  assert.equal(collection.entries[0].group, '');
  const removed = context.deletePinnedGroup({ groups: [{ id: 'g', name: 'One' }], entries: [{ path: '/one', title: 'One', group: 'g', pinnedAt: 3 }] }, 'g');
  assert.equal(removed.entries[0].group, '');
  assert.equal(removed.entries[0].pinnedAt, 3);
});

test('an intentionally empty v2 collection never resurrects the legacy backup', () => {
  const legacy = JSON.stringify([{ path: '/old', title: 'Old pin' }]);
  assert.equal(context.parsePinnedCollection(JSON.stringify({ groups: [], entries: [] }), legacy).entries.length, 0);
  assert.equal(context.parsePinnedCollection('{broken', legacy).entries.length, 1);
});
