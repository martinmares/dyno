import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../assets/syntax.js', import.meta.url), 'utf8');

function setup() {
  const calls = [];
  const mimeModes = {};
  const CodeMirror = {
    modes: { javascript: true },
    defineMode(name, mode) { this.modes[name] = mode; },
    defineMIME(mime, name) { mimeModes[mime] = name; },
    resolveMode(mime) { return { name: mimeModes[mime] || mime }; },
    runMode(text, mime, output) { calls.push({ text, mime }); output(text, 'keyword'); },
  };
  const context = vm.createContext({ CodeMirror, window: {} });
  vm.runInContext(source, context);
  return { syntax: context.window.DynoSyntax, calls };
}

function code(text, language = null, classes = []) {
  const target = {
    textContent: text,
    classList: new Set(classes),
    closest: () => language === null ? null : { dataset: { codeLanguage: language } },
    appendChild(node) { this.textContent += node.textContent; },
  };
  target.ownerDocument = {
    createTextNode: textContent => ({ textContent }),
    createElement: () => ({ textContent: '', appendChild: target.appendChild }),
  };
  return target;
}

test('static SQL uses the same registered mode and preserves literal source', () => {
  const { syntax, calls } = setup();
  const text = "SELECT\t'<img src=x onerror=alert(1)>';\n-- comment\n";
  const target = code(text);
  assert.equal(syntax.highlight(target, 'SQL'), true);
  assert.deepEqual(calls, [{ text, mime: 'text/x-sql' }]);
  assert.equal(target.textContent, text);
  assert.ok(target.classList.has('dyno-code-highlight'));
});

test('unknown or unloaded languages retain the server-rendered fallback', () => {
  const { syntax, calls } = setup();
  for (const language of ['rust', 'go']) {
    const target = code('fallback');
    assert.equal(syntax.highlight(target, language), false);
    assert.equal(target.textContent, 'fallback');
    assert.ok(!target.classList.has('dyno-code-highlight'));
  }
  assert.equal(calls.length, 0);
});

test('page highlighting resolves Chroma metadata and ordinary fenced code classes', () => {
  const { syntax, calls } = setup();
  syntax.highlightAll({ querySelectorAll: () => [
    code('SELECT 1;', 'sql'), code('SELECT 2;', null, ['language-sql']), code('plain'),
  ] });
  assert.deepEqual(calls.map(call => call.text), ['SELECT 1;', 'SELECT 2;']);
});
