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
      querySelector: () => ({ querySelectorAll: selector => selector.startsWith('details') ? [details] : [row] }),
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
