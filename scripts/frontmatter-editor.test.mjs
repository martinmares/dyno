import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
const editorStart = source.indexOf('function initFrontmatterEditor()');
const bindRow = source.slice(source.indexOf('    function bindRow(row)', editorStart), source.indexOf('    function resizeValue(', editorStart));

function cell() {
  const handlers = {};
  let blurs = 0;
  const input = {
    value: 'NEW',
    matches: () => true,
    addEventListener: (name, handler) => { handlers[name] = handler; },
    blur() { blurs++; handlers.blur(); },
  };
  const row = { querySelectorAll: selector => selector.includes('remove') ? [] : [input] };
  vm.runInNewContext(bindRow + '\nbindRow(row);', { row, resizeValue() {}, updateSummary() {} });
  handlers.focus();
  return { input, handlers, blurs: () => blurs };
}

function key(key, overrides = {}) {
  return { key, prevented: false, stopped: false,
    preventDefault() { this.prevented = true; },
    stopPropagation() { this.stopped = true; }, ...overrides };
}

test('Enter finishes the cell edit without submitting the whole form', () => {
  const state = cell();
  state.input.value = 'REVISION';
  const event = key('Enter');
  state.handlers.keydown(event);
  assert.equal(event.prevented, true);
  assert.equal(state.blurs(), 1);
  assert.equal(state.input.value, 'REVISION');
});

test('Shift+Enter and IME confirmation keep editing the value', () => {
  for (const overrides of [{ shiftKey: true }, { isComposing: true }]) {
    const state = cell();
    const event = key('Enter', overrides);
    state.handlers.keydown(event);
    assert.equal(event.prevented, false);
    assert.equal(state.blurs(), 0);
  }
});

test('Escape reverts only the current cell and does not close the dialog', () => {
  const state = cell();
  state.input.value = 'REVISION';
  const event = key('Escape');
  state.handlers.keydown(event);
  assert.equal(state.input.value, 'NEW');
  assert.equal(event.prevented, true);
  assert.equal(event.stopped, true);
  assert.equal(state.blurs(), 1);
  state.input.value = 'DRAFT';
  state.handlers.focus();
  state.input.value = 'FINAL';
  state.handlers.keydown(key('Escape'));
  assert.equal(state.input.value, 'DRAFT');
});
