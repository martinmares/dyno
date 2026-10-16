(function () {
  'use strict';

  CodeMirror.defineMode('dyno-diagram', function () {
    return { token: function (stream) {
      if (stream.match(/^\s*#.*/)) return 'comment';
      if (stream.match(/^[A-Za-z0-9_.\-"' ]+(?=\s*[:{}→>])/)) return 'keyword';
      if (stream.match(/:\s*.+/)) return 'string';
      stream.next();
      return 'variable-2';
    } };
  });
  CodeMirror.defineMIME('text/x-d2', 'dyno-diagram');
  CodeMirror.defineMIME('text/x-mermaid', 'dyno-diagram');

  CodeMirror.defineMode('dyno-sql', function () {
    const keywords = /^(?:ALL|ALTER|AND|AS|BEGIN|BY|CASCADE|CASE|CHECK|COMMIT|CONNECT|CREATE|DATABASE|DEFAULT|DELETE|DISTINCT|DROP|ELSE|END|EXECUTE|EXISTS|FALSE|FROM|FUNCTION|GRANT|GROUP|HAVING|IN|INDEX|INSERT|INTO|IS|JOIN|KEY|LEFT|LIMIT|NOT|NULL|ON|OR|ORDER|PRIMARY|PROCEDURE|REFERENCES|RETURNING|REVOKE|RIGHT|ROLLBACK|SCHEMA|SELECT|SEQUENCE|SET|TABLE|THEN|TO|TRUE|UNION|UPDATE|USAGE|USER|VALUES|VIEW|WHEN|WHERE|WITH)\b/i;
    return {
      startState: function () { return { comment: false }; },
      token: function (stream, state) {
        if (state.comment) {
          while (!stream.eol()) {
            if (stream.match('*/')) { state.comment = false; break; }
            stream.next();
          }
          return 'comment';
        }
        if (stream.eatSpace()) return null;
        if (stream.match('--')) { stream.skipToEnd(); return 'comment'; }
        if (stream.match('/*')) { state.comment = true; return 'comment'; }
        if (stream.match(/^'(?:''|[^'])*'?/)) return 'string';
        if (stream.match(/^"(?:""|[^"])*"?/)) return 'string-2';
        if (stream.match(/^<[^>]+>/)) return 'variable-2';
        if (stream.match(/^\d+(?:\.\d+)?\b/)) return 'number';
        if (stream.match(keywords)) return 'keyword';
        if (stream.match(/^[A-Za-z_][\w$]*/)) return 'variable';
        stream.next();
        return null;
      },
    };
  });
  CodeMirror.defineMIME('text/x-sql', 'dyno-sql');

  const languages = {
    bash: 'text/x-sh', sh: 'text/x-sh', shell: 'text/x-sh', zsh: 'text/x-sh',
    go: 'text/x-go', yaml: 'text/x-yaml', yml: 'text/x-yaml',
    javascript: 'text/javascript', js: 'text/javascript',
    typescript: 'text/typescript', ts: 'text/typescript',
    python: 'text/x-python', py: 'text/x-python',
    html: 'text/html', xml: 'application/xml', json: 'application/json',
    sql: 'text/x-sql', d2: 'text/x-d2', mermaid: 'text/x-mermaid',
    markdown: 'text/x-markdown', md: 'text/x-markdown',
  };
  CodeMirror.findModeByName = function (name) {
    const mime = languages[name.toLowerCase()];
    return mime ? { mime: mime } : null;
  };

  function highlight(element, language) {
    const mode = CodeMirror.findModeByName(language);
    if (!mode || !CodeMirror.modes[CodeMirror.resolveMode(mode.mime).name]) return false;
    const source = element.textContent;
    element.textContent = '';
    // The DOM-output variant expands tabs; token callbacks preserve source text.
    CodeMirror.runMode(source, mode.mime, function (text, style) {
      let node = element.ownerDocument.createTextNode(text);
      if (style) {
        const span = element.ownerDocument.createElement('span');
        span.className = style.split(' ').map(name => 'cm-' + name).join(' ');
        span.appendChild(node);
        node = span;
      }
      element.appendChild(node);
    });
    element.classList.add('dyno-code-highlight');
    return true;
  }

  function highlightAll(root = document) {
    root.querySelectorAll('pre > code:not(.dyno-code-highlight)').forEach(function (code) {
      const languageClass = Array.from(code.classList).find(name => name.startsWith('language-'));
      const wrapper = code.closest('[data-code-language]');
      const language = wrapper ? wrapper.dataset.codeLanguage : languageClass && languageClass.slice(9);
      if (language) highlight(code, language);
    });
  }

  window.DynoSyntax = { highlight, highlightAll };
})();
