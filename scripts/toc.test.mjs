import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const app = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
const source = app.slice(app.indexOf('function initTOCScrollSpy()'), app.indexOf('function initTaskFilters()'));

function setup() {
  const listeners = new Set();
  const panel = { clientHeight: 400, scrollTop: 0, getBoundingClientRect: () => ({ top: 100, bottom: 500 }) };
  const headings = [{ id: 'one', top: 80 }, { id: 'two', top: 800 }];
  const links = headings.map(heading => ({
    top: 140, bottom: 180, attrs: {},
    classList: { toggle() {} },
    getAttribute: () => '#' + heading.id,
    setAttribute(name, value) { this.attrs[name] = value; },
    removeAttribute(name) { delete this.attrs[name]; },
    closest: () => panel,
    getBoundingClientRect() { return { top: this.top, bottom: this.bottom }; },
  }));
  let visibleLinks = links;
  const context = vm.createContext({
    document: { querySelectorAll: () => visibleLinks, getElementById: id => {
      const h = headings.find(h => h.id === id);
      return { id, getBoundingClientRect: () => ({ top: h.top }) };
    } },
    window: { addEventListener: (_, fn) => listeners.add(fn), removeEventListener: (_, fn) => listeners.delete(fn) },
  });
  vm.runInContext(source, context);
  return { context, panel, headings, links, listeners, scroll: () => listeners.forEach(fn => fn()), hideTOC: () => { visibleLinks = []; } };
}

test('TOC follows active headings down and up within its own scroll pane', () => {
  const s = setup();
  s.context.initTOCScrollSpy();
  assert.equal(s.panel.scrollTop, 0);
  s.headings[1].top = 80;
  s.links[1].top = 520; s.links[1].bottom = 560;
  s.scroll();
  assert.equal(s.panel.scrollTop, 68);
  assert.equal(s.links[1].attrs['aria-current'], 'location');
  s.scroll();
  assert.equal(s.panel.scrollTop, 68, 'do not fight manual TOC scrolling within the same section');
  s.headings[1].top = 800;
  s.links[0].top = 60; s.links[0].bottom = 100;
  s.scroll();
  assert.equal(s.panel.scrollTop, 20);
});

test('TOC reinitialization removes stale listeners, including pages without a TOC', () => {
  const s = setup();
  s.context.initTOCScrollSpy();
  s.context.initTOCScrollSpy();
  assert.equal(s.listeners.size, 1);
  s.hideTOC();
  s.context.initTOCScrollSpy();
  assert.equal(s.listeners.size, 0);
});
