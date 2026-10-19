import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
const printSource = source.slice(source.indexOf('let printPageState = null;'), source.indexOf('function initTOCScrollSpy()'));

test('printing expands closed details and hidden table rows, then restores them', () => {
  const details = { open: false };
  const row = { hidden: true };
  const listeners = {};
  let prints = 0;
  const context = vm.createContext({
    window: { addEventListener(name, handler) { listeners[name] = handler; }, print() { prints++; } },
    document: {
      querySelector: () => ({ querySelector: () => null, querySelectorAll: selector => selector.startsWith('details') ? [details] : [row] }),
      addEventListener(name, handler) { listeners[name] = handler; },
    },
  });
  vm.runInContext(printSource, context);
  listeners.beforeprint();
  listeners.beforeprint();
  assert.equal(details.open, true);
  assert.equal(row.hidden, false);
  listeners.afterprint();
  listeners.afterprint();
  assert.equal(details.open, false);
  assert.equal(row.hidden, true);
  listeners.click({ target: { closest: () => ({}) } });
  assert.equal(prints, 1);
});

test('printing a non-document page does not alter state', () => {
  const context = vm.createContext({
    window: { addEventListener() {} },
    document: { querySelector: () => null, addEventListener() {} },
  });
  vm.runInContext(printSource, context);
  context.preparePagePrint();
  context.restorePagePrint();
});

test('print information moves below the title, gets a fresh timestamp and is restored', () => {
  let placed = 0;
  let restored = 0;
  const time = { textContent: '', removeAttribute(name) { assert.equal(name, 'datetime'); delete this.dateTime; } };
  const next = {};
  const parent = { insertBefore(element, sibling) { assert.equal(element, summary); assert.equal(sibling, next); restored++; } };
  const summary = { parentNode: parent, nextSibling: next, querySelector: () => time };
  const title = { after(element) { assert.equal(element, summary); placed++; } };
  const prose = { querySelector: () => title };
  const article = {
    querySelectorAll: () => [],
    querySelector: selector => selector === '[data-print-summary]' ? summary : prose,
  };
  const context = vm.createContext({
    window: { addEventListener() {} },
    document: { querySelector: () => article, addEventListener() {} },
  });
  vm.runInContext(printSource, context);
  context.preparePagePrint();
  assert.ok(time.textContent.length > 0);
  assert.ok(Math.abs(Date.now() - Date.parse(time.dateTime)) < 5000);
  context.preparePagePrint();
  assert.equal(placed, 1);
  context.restorePagePrint();
  assert.equal(restored, 1);
  assert.equal(time.textContent, '');
  assert.equal(time.dateTime, undefined);
  context.preparePagePrint();
  assert.equal(placed, 2);
  context.restorePagePrint();
});

test('documents without a title place print information at the beginning of the prose', () => {
  let prepended = false;
  const summary = {
    parentNode: { insertBefore() {} }, nextSibling: null,
    querySelector: () => ({ removeAttribute() {} }),
  };
  const prose = { querySelector: () => null, prepend(element) { assert.equal(element, summary); prepended = true; } };
  const article = { querySelectorAll: () => [], querySelector: selector => selector === '[data-print-summary]' ? summary : prose };
  const context = vm.createContext({
    window: { addEventListener() {} }, document: { querySelector: () => article, addEventListener() {} },
  });
  vm.runInContext(printSource, context);
  context.preparePagePrint();
  assert.equal(prepended, true);
  context.restorePagePrint();
});

test('change report timestamps are refreshed by the same print handler', () => {
  const time = {};
  const article = { querySelector: () => null, querySelectorAll: selector => selector === '[data-report-time]' ? [time] : [] };
  const context = vm.createContext({ window: { addEventListener() {} }, document: { querySelector: () => article, addEventListener() {} } });
  vm.runInContext(printSource, context);
  context.preparePagePrint();
  assert.ok(time.textContent);
  assert.ok(Math.abs(Date.now() - Date.parse(time.dateTime)) < 5000);
  context.restorePagePrint();
});
