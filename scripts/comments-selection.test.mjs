import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
const excludedHelper = source.slice(source.indexOf('function isCommentSelectionExcluded('), source.indexOf('function initComments('));
const captureHandler = source.slice(source.indexOf('    function captureSelection('), source.indexOf('    if (window.dynoCommentsMouseupHandler)'));

function element(excluded = false) {
  return { nodeType: 1, closest: () => excluded ? {} : null };
}

function setup() {
  const paragraph = element();
  const text = { nodeType: 3, parentElement: paragraph };
  const range = {
    startContainer: text, endContainer: text, commonAncestorContainer: paragraph,
    cloneRange() { return this; },
  };
  let opens = 0;
  const context = vm.createContext({
    Node: { ELEMENT_NODE: 1 },
    document: { activeElement: element(), querySelector: () => null },
    window: { getSelection: () => ({ isCollapsed: false, rangeCount: 1, getRangeAt: () => range, toString: () => 'Selected text' }) },
    article: { contains: () => true },
    root: { contains: () => false },
    quoteInput: {}, anchorInput: {}, preview: {},
    actionDialog: { open: false, showModal() { opens++; } },
    nearestHeadingID: () => 'heading',
    selectedRange: null, selectedText: '', selectedQuote: '', selectedAnchor: '',
  });
  vm.runInContext(excludedHelper + captureHandler, context);
  return { context, range, paragraph, opens: () => opens };
}

test('reading selection still opens annotation actions', () => {
  const state = setup();
  state.context.captureSelection({ target: state.paragraph });
  assert.equal(state.opens(), 1);
  assert.equal(state.context.selectedQuote, 'Selected text');
});

for (const location of ['event target', 'focused editor', 'selection start', 'selection end', 'selection ancestor', 'open dialog']) {
  test(`annotation actions ignore ${location}`, () => {
    const state = setup();
    const event = { target: state.paragraph };
    const editor = element(true);
    if (location === 'event target') event.target = editor;
    if (location === 'focused editor') state.context.document.activeElement = editor;
    if (location === 'selection start') state.range.startContainer = { nodeType: 3, parentElement: editor };
    if (location === 'selection end') state.range.endContainer = editor;
    if (location === 'selection ancestor') state.range.commonAncestorContainer = editor;
    if (location === 'open dialog') state.context.document.querySelector = () => ({});
    state.context.captureSelection(event);
    assert.equal(state.opens(), 0);
    assert.equal(state.context.selectedRange, null);
  });
}

test('exclusion selector covers native, visual and code editors', () => {
  const state = setup();
  let selector;
  state.context.isCommentSelectionExcluded({ nodeType: 1, closest(value) { selector = value; return null; } });
  for (const required of ['input', 'textarea', '[contenteditable]', '.CodeMirror', '.cm-editor', '.ProseMirror', '#editor-pane', 'dialog', '[data-comments-root]']) {
    assert.ok(selector.includes(required), `missing ${required}`);
  }
});
