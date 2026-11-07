import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const app = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
const source = app.slice(app.indexOf('function initSidebarCollapse()'), app.indexOf("  if (window.DynoSyntax) DynoSyntax.highlightAll();"));
// The following DOMContentLoaded listener belongs to other page initialization.
const handlersSource = source.slice(0, source.lastIndexOf("document.addEventListener('DOMContentLoaded'"));

function setup(width = 1440, storageFails = false) {
  const classes = new Set();
  const handlers = {};
  const saved = new Map();
  const sidebar = { dataset: { navStorageKey: '/docs' } };
  const button = () => ({ attrs: {}, focused: false, setAttribute(key, value) { this.attrs[key] = value; }, focus() { this.focused = true; } });
  const collapse = button(), restore = button();
  const context = vm.createContext({
    window: { innerWidth: width },
    document: {
      documentElement: { classList: { contains: name => classes.has(name), toggle(name) { if (classes.has(name)) { classes.delete(name); return false; } classes.add(name); return true; } } },
      addEventListener(name, fn) { handlers[name] = fn; },
      querySelectorAll: () => [collapse, restore],
      getElementById: () => sidebar,
      querySelector: selector => selector === '.dyno-sidebar-restore' ? restore : collapse,
    },
    localStorage: { setItem(key, value) { if (storageFails) throw new Error('storage unavailable'); saved.set(key, value); } },
  });
  vm.runInContext(handlersSource, context);
  return { context, classes, handlers, saved, collapse, restore, click: target => handlers.click({ target: { closest: () => target } }) };
}

test('sidebar can be restored from an external button and preserves site state and focus', () => {
  const s = setup();
  s.click(s.collapse);
  assert.equal(s.saved.get('dyno-sidebar-collapsed:/docs'), 'true');
  assert.equal(s.restore.attrs['aria-label'], 'Expand navigation');
  assert.equal(s.restore.attrs['aria-expanded'], 'false');
  assert.equal(s.restore.focused, true);
  s.handlers['htmx:afterSwap']();
  assert.equal(s.restore.attrs['aria-expanded'], 'false');
  s.click(s.restore);
  assert.equal(s.saved.get('dyno-sidebar-collapsed:/docs'), 'false');
  assert.equal(s.classes.has('dyno-sidebar-collapsed'), false);
  assert.equal(s.collapse.attrs['aria-label'], 'Collapse navigation');
  assert.equal(s.collapse.focused, true);
});

test('desktop sidebar collapse does not interfere with mobile navigation', () => {
  const s = setup(390);
  s.click(s.collapse);
  assert.equal(s.classes.size, 0);
  assert.equal(s.saved.size, 0);
});

test('sidebar restore works when local storage is unavailable', () => {
  const s = setup(1440, true);
  s.click(s.collapse);
  s.click(s.restore);
  assert.equal(s.classes.has('dyno-sidebar-collapsed'), false);
});
