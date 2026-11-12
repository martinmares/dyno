import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const app = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');

function setup() {
  const events = new Map();
  const inputs = [{ checked: true, defaultChecked: true }, { checked: false, defaultChecked: false }];
  let resets = 0, focused = false;
  const form = { reset() { resets++; inputs.forEach(input => { input.checked = input.defaultChecked; }); } };
  const dialog = {
    id: 'metadata-filter-dialog', open: false,
    querySelector: () => form, querySelectorAll: () => inputs,
    showModal() { this.open = true; },
    close() { this.open = false; emit('close', { target: this }); },
  };
  const document = {
    addEventListener(name, fn) { events.set(name, fn); },
    getElementById: () => dialog,
    querySelector: () => ({ focus() { focused = true; } }),
  };
  const emit = (name, event) => events.get(name)(event);
  const click = attr => emit('click', { target: { closest: selector => selector === `[data-metadata-filters-${attr}]` } });
  vm.runInNewContext(app.slice(app.indexOf('function initMetadataFilters()'), app.indexOf("document.addEventListener('DOMContentLoaded', initMetadataFilters);")) + '\ninitMetadataFilters();', { document });
  return { dialog, inputs, emit, click, resets: () => resets, focused: () => focused };
}

test('metadata selection is draft state until Apply; cancel and Escape discard changes', () => {
  const state = setup();
  state.click('open');
  assert.equal(state.dialog.open, true);
  state.click('clear');
  assert.deepEqual(state.inputs.map(input => input.checked), [false, false]);
  assert.equal(state.dialog.open, true);
  state.click('close');
  assert.deepEqual(state.inputs.map(input => input.checked), [true, false]);
  state.click('open');
  state.inputs[1].checked = true;
  state.dialog.close();
  assert.equal(state.inputs[1].checked, false);
});

test('metadata dialog survives failed requests and closes before successful HTMX replacement', () => {
  const state = setup();
  state.click('open');
  state.emit('htmx:beforeSwap', { detail: { target: { id: 'page-content' }, shouldSwap: false } });
  assert.equal(state.dialog.open, true);
  state.emit('htmx:beforeSwap', { detail: { target: { id: 'search-results-container' }, shouldSwap: true } });
  assert.equal(state.dialog.open, true);
  state.emit('htmx:beforeSwap', { detail: { target: { id: 'page-content' }, shouldSwap: true } });
  assert.equal(state.dialog.open, false);
  state.emit('htmx:afterSettle', {});
  assert.equal(state.focused(), true);
});
