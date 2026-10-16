import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
const themeSource = source.slice(0, source.indexOf('// ─── Page text size'));

test('shared palette has no legacy framework tokens or self-referencing variables', () => {
  const css = readFileSync(new URL('../assets/app.css', import.meta.url), 'utf8');
  const editorCSS = readFileSync(new URL('../assets/editor-visual.css', import.meta.url), 'utf8');
  const combined = css + editorCSS;
  assert.doesNotMatch(combined, /--tblr-/);
  assert.doesNotMatch(combined, /(--dyno-[\w-]+):\s*var\(\1\)/);
  const definitions = new Set(Array.from(combined.matchAll(/(--dyno-[\w-]+)\s*:/g), match => match[1]));
  for (const match of combined.matchAll(/var\((--dyno-[\w-]+)/g)) {
    assert.ok(definitions.has(match[1]), `undefined palette token ${match[1]}`);
  }
});

for (const stored of [null, 'light', 'dark']) {
  test(`Bulma theme respects stored preference ${stored} and stays in sync on toggle`, () => {
    const classes = new Set();
    const attributes = {};
    const handlers = {};
    const html = {
      classList: {
        toggle(name, on) { if (on) classes.add(name); else classes.delete(name); },
        contains: name => classes.has(name),
      },
      setAttribute(name, value) { attributes[name] = value; },
    };
    const storage = new Map(stored ? [['dyno-theme', stored]] : []);
    const button = { addEventListener(name, handler) { handlers[name] = handler; } };
    const context = vm.createContext({
      document: {
        documentElement: html,
        getElementById: id => id === 'theme-toggle' ? button : null,
        addEventListener(name, handler) { handlers[name] = handler; },
      },
      localStorage: { getItem: key => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value) },
      window: { matchMedia: () => ({ matches: true, addEventListener(name, handler) { handlers[name] = handler; } }) },
      rerenderMermaid() {}, rerenderSiteGraphs() {},
    });
    vm.runInContext(themeSource, context);
    const initial = stored !== 'light';
    assert.equal(attributes['data-theme'], initial ? 'dark' : 'light');
    assert.equal(classes.has('dark'), initial);
    assert.equal(attributes['data-bs-theme'], undefined);
    handlers.DOMContentLoaded();
    handlers.click();
    assert.equal(attributes['data-theme'], initial ? 'light' : 'dark');
    assert.equal(storage.get('dyno-theme'), attributes['data-theme']);
    assert.equal(classes.has('dark'), !initial);
    handlers.change({ matches: initial });
    assert.equal(classes.has('dark'), !initial, 'OS change must not override explicit choice');
  });
}
