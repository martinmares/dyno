// Keep the literal Markdown source separate from the editable request fields.
// Unsupported content stays in Source instead of being discarded by the form.
export function parseAPIBlock(source) {
  const lines = source.replace(/\r\n/g, '\n').replace(/\n$/, '').split('\n');
  const opening = lines.shift() || '';
  const fence = opening.match(/^( {0,3})(`{3,}|~{3,})\s*(api(?:-insecure)?)(?:\s+(.*?))?\s*$/);
  const closing = lines.pop() || '';
  if (!fence || !new RegExp('^ {0,3}' + fence[2][0] + '{' + fence[2].length + ',}[ \t]*$').test(closing)) {
    throw new Error('Use a complete api or api-insecure fenced block.');
  }
  const closes = new RegExp('^ {0,3}' + fence[2][0] + '{' + fence[2].length + ',}[ \t]*$');
  if (lines.some(line => closes.test(line))) throw new Error('The source must contain a single API block.');
  const nonempty = lines.filter(line => line.trim());
  const request = (nonempty.shift() || '').trim().match(/^(\S+)[ \t]+(.+)$/);
  if (!request) throw new Error('Enter a request method and URL on the first line of the API block.');
  const options = (fence[4] || '').split(/\s+/).filter(Boolean);
  const spec = {
    source, indent: fence[1], marker: fence[2], closing,
    newline: source.includes('\r\n') ? '\r\n' : '\n', trailingNewline: source.endsWith('\n'),
    method: request[1], url: request[2], headers: [],
    insecure: fence[3] === 'api-insecure', noAuth: options.includes('no-auth'),
    followJSONPath: '', followMethod: '', formAvailable: options.every(option => option === 'no-auth'),
  };
  const directives = new Set();
  for (const line of nonempty) {
    const directive = line.trim().match(/^(@follow-jsonpath|@follow-method):\s*(.*)$/);
    if (directive) {
      if (directives.has(directive[1])) spec.formAvailable = false;
      directives.add(directive[1]);
      spec[directive[1] === '@follow-jsonpath' ? 'followJSONPath' : 'followMethod'] = directive[2];
    } else {
      const header = line.trim().match(/^([!#$%&'*+.^_`|~0-9A-Za-z-]+):\s*(.*)$/);
      if (header) spec.headers.push([header[1], header[2]]);
      else spec.formAvailable = false;
    }
  }
  return spec;
}

export function serializeAPIBlock(base, fields) {
  if (!base.formAvailable) throw new Error('Edit this block in Source to preserve unsupported content.');
  const singleLine = (value, label) => {
    if (/[\r\n]/.test(value)) throw new Error(label + ' must fit on one line.');
    return value.trim();
  };
  const method = singleLine(fields.method, 'Method');
  const url = singleLine(fields.url, 'URL');
  if (!method || /\s/.test(method)) throw new Error('Enter a method or a method placeholder without spaces.');
  if (!url) throw new Error('Enter a URL.');
  const lines = [base.indent + base.marker + (fields.insecure ? 'api-insecure' : 'api') + (fields.noAuth ? ' no-auth' : ''), method + ' ' + url];
  for (const [name, value] of fields.headers) {
    const key = singleLine(name, 'Header name');
    const content = singleLine(value, 'Header value');
    if (!key && !content) continue;
    if (!/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(key)) throw new Error('Enter a valid name for each header.');
    lines.push(key + ': ' + content);
  }
  const path = singleLine(fields.followJSONPath, 'JSONPath');
  const followMethod = singleLine(fields.followMethod, 'Follow-up method');
  if (followMethod && /\s/.test(followMethod)) throw new Error('The follow-up method cannot contain spaces.');
  if (path) lines.push('@follow-jsonpath: ' + path);
  if (followMethod) lines.push('@follow-method: ' + followMethod);
  lines.push(base.closing);
  const source = lines.join(base.newline) + (base.trailingNewline ? base.newline : '');
  parseAPIBlock(source);
  return source;
}

export function apiBlockLabel(source) {
  try {
    const spec = parseAPIBlock(source);
    return 'API · ' + spec.method + ' ' + spec.url;
  } catch { return 'API block'; }
}

const apiEditorCleanup = new WeakMap();

export function openAPIEditor(source, onApply, onClose) {
  const newline = source.includes('\r\n') ? '\r\n' : '\n';
  const dialog = document.getElementById('visual-api-dialog');
  apiEditorCleanup.get(dialog)?.();
  const form = dialog.querySelector('form');
  const formPane = dialog.querySelector('[data-api-panel="form"]');
  const sourcePane = dialog.querySelector('[data-api-panel="source"]');
  const error = dialog.querySelector('[data-api-error]');
  const hint = dialog.querySelector('[data-api-hint]');
  const headers = dialog.querySelector('[data-api-headers]');
  const field = name => dialog.querySelector('[data-api-field="' + name + '"]');
  let current = source;
  let base;
  let snapshot;
  let active = 'source';
  let cm;
  const showError = message => { error.textContent = message; error.hidden = !message; };
  function addHeader(name = '', value = '') {
    const row = document.createElement('div');
    row.className = 'dyno-api-header-row';
    for (const [label, text] of [['Header name', name], ['Header value', value]]) {
      const input = document.createElement('input');
      input.className = 'form-control';
      input.type = 'text';
      input.autocomplete = 'off';
      input.spellcheck = false;
      input.setAttribute('aria-label', label);
      input.placeholder = label;
      input.value = text;
      row.appendChild(input);
    }
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'btn btn-icon btn-ghost-secondary';
    remove.textContent = '×';
    remove.setAttribute('aria-label', 'Remove header');
    remove.onclick = () => { row.remove(); dialog.querySelector('[data-api-add-header]').focus(); };
    row.appendChild(remove);
    headers.appendChild(row);
  }
  function readFields() {
    return {
      method: field('method').value, url: field('url').value,
      insecure: field('insecure').checked, noAuth: field('noAuth').checked,
      followJSONPath: field('followJSONPath').value, followMethod: field('followMethod').value,
      headers: Array.from(headers.children, row => Array.from(row.querySelectorAll('input'), input => input.value)),
    };
  }
  function populate(spec) {
    for (const name of ['method', 'url', 'followJSONPath', 'followMethod']) field(name).value = spec[name];
    for (const name of ['insecure', 'noAuth']) field(name).checked = spec[name];
    headers.replaceChildren();
    spec.headers.forEach(([name, value]) => addHeader(name, value));
    dialog.querySelector('details').open = spec.insecure || spec.noAuth || !!spec.followJSONPath || !!spec.followMethod;
    snapshot = JSON.stringify(readFields());
  }
  function formSource() {
    const fields = readFields();
    return JSON.stringify(fields) === snapshot ? current : serializeAPIBlock(base, fields);
  }
  function selectPanel(name) {
    try {
      if (name === 'form') {
        const candidate = cm ? cm.getValue(newline) : current;
        const parsed = parseAPIBlock(candidate);
        if (!parsed.formAvailable) throw new Error('This block contains settings the form cannot edit. Keep editing in Source to preserve them.');
        current = candidate;
        base = parsed;
        populate(base);
      } else if (active === 'form') {
        current = formSource();
        cm.setValue(current);
      }
      active = name;
      formPane.hidden = name !== 'form';
      sourcePane.hidden = name !== 'source';
      dialog.querySelectorAll('[data-api-tab]').forEach(button => {
        const selected = button.dataset.apiTab === name;
        button.classList.toggle('is-active', selected);
        button.setAttribute('aria-selected', String(selected));
        button.tabIndex = selected ? 0 : -1;
      });
      hint.hidden = true;
      showError('');
      if (name === 'source' && cm) requestAnimationFrame(() => {
        if (!cm || !dialog.open || active !== 'source') return;
        cm.refresh();
        cm.focus();
      });
      return true;
    } catch (failure) { showError(failure.message); return false; }
  }
  dialog.querySelectorAll('[data-api-tab]').forEach(button => {
    button.onclick = () => { if (button.dataset.apiTab !== active) selectPanel(button.dataset.apiTab); };
    button.onkeydown = event => {
      if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
      event.preventDefault();
      const name = event.key === 'Home' ? 'form' : event.key === 'End' ? 'source' : active === 'form' ? 'source' : 'form';
      if (name !== active && !selectPanel(name)) return;
      dialog.querySelector('[data-api-tab="' + name + '"]').focus();
    };
  });
  dialog.querySelectorAll('[data-api-cancel]').forEach(button => { button.onclick = () => dialog.close(); });
  dialog.querySelector('[data-api-add-header]').onclick = () => {
    addHeader();
    headers.lastElementChild.querySelector('input').focus();
  };
  form.onsubmit = event => {
    event.preventDefault();
    try {
      const value = active === 'form' ? formSource() : cm.getValue(newline);
      parseAPIBlock(value);
      onApply(value);
      dialog.close();
    } catch (failure) { showError(failure.message); }
  };
  const cleanup = () => {
    if (!cm) return;
    cm.toTextArea();
    cm = null;
    apiEditorCleanup.delete(dialog);
    onClose();
  };
  apiEditorCleanup.set(dialog, cleanup);
  dialog.onclose = () => { if (!dialog.open) cleanup(); };
  showError('');
  formPane.hidden = true;
  sourcePane.hidden = false;
  hint.hidden = true;
  dialog.showModal();
  const textarea = dialog.querySelector('textarea');
  textarea.value = source;
  cm = window.CodeMirror.fromTextArea(textarea, {
    mode: 'markdown', theme: document.documentElement.classList.contains('dark') ? 'dracula' : 'default',
    lineNumbers: true, lineWrapping: true, indentUnit: 2,
    extraKeys: { 'Esc': () => dialog.close(), 'Ctrl-S': () => form.requestSubmit(), 'Cmd-S': () => form.requestSubmit() },
  });
  if (!selectPanel('form')) {
    selectPanel('source');
    hint.textContent = 'This block opens in Source because the form cannot represent all its content.';
    hint.hidden = false;
  } else field('method').focus();
}
