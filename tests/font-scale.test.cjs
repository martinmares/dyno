const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const { runInNewContext } = require('node:vm');

// Exercise the production initializer without loading unrelated UI modules.
const source = readFileSync(join(__dirname, '../assets/app.js'), 'utf8');
const start = source.indexOf('// ─── Page text size');
const end = source.indexOf('// ─── Sidebar', start);
assert.ok(start >= 0 && end > start);
const initializer = source.slice(start, end);

function initialize(stored, unavailable = false) {
  const controls = {};
  for (const name of ['decrease', 'reset', 'increase']) {
    controls['font-size-' + name] = {
      dataset: {}, handlers: {},
      addEventListener(event, handler) { this.handlers[event] = handler; },
      setAttribute() {},
    };
  }
  const document = {
    documentElement: { style: {} },
    getElementById(id) { return controls[id]; },
    addEventListener(event, handler) { this.ready = handler; },
  };
  const sessionStorage = {
    getItem() { if (unavailable) throw new Error('Storage unavailable'); return stored; },
    setItem(key, value) { if (unavailable) throw new Error('Storage unavailable'); stored = value; },
  };
  runInNewContext(initializer, { document, sessionStorage });
  const initial = document.documentElement.style.fontSize;
  document.ready();
  return { document, controls, initial, stored: () => stored };
}

for (const [stored, expected] of [
  [null, '100%'], ['', '100%'], ['  ', '100%'],
  ['invalid', '100%'], ['Infinity', '100%'],
  ['1', '100%'], ['0.8', '80%'], ['1.2', '120%'],
  ['0.5', '80%'], ['2', '150%'],
]) {
  test(`initial scale for ${JSON.stringify(stored)} is ${expected}`, () => {
    const ui = initialize(stored);
    assert.equal(ui.initial, expected);
    assert.equal(ui.document.documentElement.style.fontSize, expected);
  });
}

test('unavailable storage falls back to 100%', () => {
  assert.equal(initialize(null, true).document.documentElement.style.fontSize, '100%');
});

test('controls update and persist scale, reset returns to 100%', () => {
  const ui = initialize(null);
  ui.controls['font-size-increase'].handlers.click();
  assert.equal(ui.document.documentElement.style.fontSize, '110%');
  assert.equal(ui.stored(), '1.1');
  ui.controls['font-size-reset'].handlers.click();
  assert.equal(ui.document.documentElement.style.fontSize, '100%');
  assert.equal(ui.stored(), '1');
  assert.equal(ui.controls['font-size-reset'].disabled, true);
  ui.controls['font-size-decrease'].handlers.click();
  assert.equal(ui.document.documentElement.style.fontSize, '90%');
  assert.equal(ui.stored(), '0.9');
});
