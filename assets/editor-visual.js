import { Crepe } from '@milkdown/crepe';
import { imageSchema } from '@milkdown/kit/preset/commonmark';

const opaqueTokenPrefix = 'DYNOOPAQUEBLOCK';

function splitFrontmatter(source) {
  const match = source.match(/^---[ \t]*\r?\n[\s\S]*?\r?\n(?:---|\.\.\.)[ \t]*(?:\r?\n|$)/);
  return match ? [match[0], source.slice(match[0].length)] : ['', source];
}

function protectBlocks(body) {
  const lines = body.match(/[^\n]*\n|[^\n]+$/g) || [];
  const blocks = [];
  const protectedLines = [];

  function protect(start, end, label, kind = 'other') {
    const token = opaqueTokenPrefix + String(blocks.length).padStart(6, '0');
    const source = lines.slice(start, end).join('');
    blocks.push({ token, source, kind, originalSource: source });
    protectedLines.push('[' + label + (kind === 'other' ? ' · edit in Source' : ' · edit block') + '](#' + token + ')' + (source.endsWith('\n') ? '\n' : ''));
  }

  for (let i = 0; i < lines.length;) {
    const line = lines[i].replace(/\r?\n$/, '');
    const fence = line.match(/^ {0,3}(`{3,}|~{3,})(.*)$/);
    if (fence) {
      const start = i++;
      const marker = fence[1][0];
      const minimum = fence[1].length;
      while (i < lines.length) {
        const closing = lines[i].trim().match(/^(`+|~+)$/);
        i++;
        if (closing && closing[1][0] === marker && closing[1].length >= minimum) break;
      }
      const language = fence[2].trim().split(/\s+/)[0].toLowerCase();
      const dynoFences = ['api', 'api-insecure', 'd2', 'mermaid', 'tasks', 'file-download', 'table'];
      protect(start, i, (language || 'Code') + ' block', dynoFences.includes(language) ? 'other' : 'fence');
      continue;
    }
    if (/^ {0,3}>\s*\[!(?:NOTE|TIP|IMPORTANT|WARNING|WARN|CAUTION|DANGER|INFO)\]/i.test(line)) {
      const start = i++;
      while (i < lines.length && /^ {0,3}>/.test(lines[i])) i++;
      protect(start, i, 'Callout', 'callout');
      continue;
    }
    if (/^ {0,3}(?:!{3}|\?{3})\s/.test(line)) {
      const start = i++;
      while (i < lines.length && (/^(?: {4}|\t)/.test(lines[i]) || (lines[i].trim() === '' && i + 1 < lines.length && /^(?: {4}|\t)/.test(lines[i + 1])))) i++;
      protect(start, i, 'Admonition');
      continue;
    }
    if (/^\[\^[^\]]+\]:/.test(line)) {
      const start = i++;
      while (i < lines.length && /^(?: {4}|\t)/.test(lines[i])) i++;
      protect(start, i, 'Footnote');
      continue;
    }
    if (i + 1 < lines.length && /^:\s/.test(lines[i + 1]) && line.trim()) {
      const start = i++;
      while (i < lines.length && (/^:\s/.test(lines[i]) || /^(?: {4}|\t)/.test(lines[i]))) i++;
      protect(start, i, 'Definition list');
      continue;
    }
    if (/\[\^[^\]]+\]/.test(line)) {
      protect(i, ++i, 'Footnote reference');
      continue;
    }
    if (/^ {0,3}<(?:!--|\/?[A-Za-z][\w:-]*\b)/.test(line)) {
      const start = i++;
      while (i < lines.length && lines[i].trim() !== '') i++;
      protect(start, i, 'HTML block');
      continue;
    }
    protectedLines.push(lines[i++]);
  }
  return { markdown: protectedLines.join(''), blocks };
}

function restoreBlocks(markdown, blocks) {
  let restored = markdown;
  for (const block of blocks) {
    const link = new RegExp('\\[[^\\]]+\\]\\(#' + block.token + '\\)(?:\\r?\\n)?', 'g');
    if (restored.split(block.token).length !== 2 || !link.test(restored)) {
      throw new Error('A protected block was changed. Use Source mode to edit it.');
    }
    restored = restored.replace(link, block.source);
  }
  return restored;
}

function patchSource(previous, next, source) {
  let prefix = 0;
  while (prefix < previous.length && prefix < next.length && previous[prefix] === next[prefix]) prefix++;
  if (prefix === previous.length && prefix === next.length) return source;

  let suffix = 0;
  while (suffix < previous.length - prefix && suffix < next.length - prefix &&
    previous[previous.length - 1 - suffix] === next[next.length - 1 - suffix]) suffix++;

  const replacement = next.slice(prefix, next.length - suffix);
  for (const width of [48, 32, 20, 12, 6]) {
    const before = previous.slice(Math.max(0, prefix - width), prefix);
    const after = previous.slice(previous.length - suffix, Math.min(previous.length, previous.length - suffix + width));
    if (prefix > 0 && !before) continue;
    if (suffix > 0 && !after) continue;
    const start = before ? source.indexOf(before) : 0;
    if (start < 0 || (before && source.indexOf(before, start + 1) !== -1)) continue;
    const end = after ? source.indexOf(after, start + before.length) : source.length;
    if (end < 0 || (after && source.indexOf(after, end + 1) !== -1)) continue;
    return source.slice(0, start + before.length) + replacement + source.slice(end);
  }

  // Table serializers pad cells; a unique edited word can still map to its source cell.
  const word = character => /[\p{L}\p{N}_-]/u.test(character || '');
  let left = prefix;
  while (left > 0 && word(previous[left - 1])) left--;
  let oldEnd = previous.length - suffix;
  while (oldEnd < previous.length && word(previous[oldEnd])) oldEnd++;
  let newEnd = next.length - suffix;
  while (newEnd < next.length && word(next[newEnd])) newEnd++;
  const oldWord = previous.slice(left, oldEnd);
  const newWord = next.slice(left, newEnd);
  if (oldWord && newWord && oldWord !== newWord &&
    source.indexOf(oldWord) >= 0 && source.indexOf(oldWord) === source.lastIndexOf(oldWord)) {
    return source.replace(oldWord, newWord);
  }
  throw new Error('This edit cannot be mapped safely to the Markdown source. Switch to Source mode.');
}

async function create(root, source, onChange, resolveImage = src => src) {
  const [frontmatter, body] = splitFrontmatter(source);
  const { markdown, blocks } = protectBlocks(body);
  const editor = new Crepe({
    root,
    defaultValue: markdown,
    features: {
      [Crepe.Feature.AI]: false,
      [Crepe.Feature.TopBar]: false,
      [Crepe.Feature.ImageBlock]: false,
    },
  });
  // Remark emits null for omitted image titles; the schema requires strings.
  editor.editor.config(ctx => ctx.update(imageSchema.key, previous => context => {
    const schema = previous(context);
    return {
      ...schema,
      toDOM(node) {
        const dom = schema.toDOM(node);
        return [dom[0], { ...dom[1], src: resolveImage(node.attrs.src) }, ...dom.slice(2)];
      },
      parseMarkdown: {
        ...schema.parseMarkdown,
        runner(state, node, type) {
          schema.parseMarkdown.runner(state, { ...node, alt: node.alt ?? '', title: node.title ?? '' }, type);
        },
      },
    };
  }));
  let normalized = '';
  let patched = markdown;
  let patchError = null;
  let ready = false;
  const fenceBlocks = blocks.filter(block => block.kind === 'fence');
  const fenceStyle = document.createElement('style');
  document.head.appendChild(fenceStyle);
  function cssString(value) {
    return '"' + Array.from(value.replace(/\r\n/g, '\n'), character => {
      if (character === '\\' || character === '"') return '\\' + character;
      const code = character.charCodeAt(0);
      return code < 32 || code === 127 ? '\\' + code.toString(16) + ' ' : character;
    }).join('') + '"';
  }
  function fenceRule(block) {
    const lines = block.source.replace(/\r\n/g, '\n').replace(/\n$/, '').split('\n');
    const language = lines.shift().replace(/^ {0,3}(?:`{3,}|~{3,})/, '').trim().split(/\s+/)[0] || 'Text';
    lines.pop();
    const preview = lines.join('\n') || ' ';
    const selector = '.dyno-visual-editor .ProseMirror a[href="#' + block.token + '"]';
    return selector + ' { display: block; max-height: 22rem; overflow: auto; padding: .5rem .75rem .75rem; border: 1px solid var(--tblr-border-color); border-radius: var(--tblr-border-radius); background: var(--tblr-bg-surface-secondary); color: transparent; font-size: 0; }\n' +
      selector + '::before { display: block; margin-bottom: .5rem; padding-bottom: .4rem; border-bottom: 1px solid var(--tblr-border-color); color: var(--tblr-secondary-color); content: ' + cssString(language.toUpperCase() + ' · CLICK TO EDIT') + '; font-family: var(--tblr-font-sans-serif); font-size: .75rem; }\n' +
      selector + '::after { display: block; color: var(--tblr-body-color); content: ' + cssString(preview) + '; font-family: var(--tblr-font-monospace); font-size: .875rem; line-height: 1.45; white-space: pre; }';
  }
  function renderFenceStyles() {
    fenceStyle.textContent = fenceBlocks.map(fenceRule).join('\n');
  }
  renderFenceStyles();
  editor.on(listener => listener.markdownUpdated(() => {
    if (!ready) return;
    const next = editor.getMarkdown();
    try {
      patched = patchSource(normalized, next, patched);
      normalized = next;
      patchError = null;
      restoreBlocks(patched, blocks);
    } catch (error) { patchError = error; }
    onChange();
  }));
  try {
    await editor.create();
    normalized = editor.getMarkdown();
    restoreBlocks(patched, blocks);
    ready = true;
    return {
      getSource() {
        if (patchError) throw patchError;
        return frontmatter + restoreBlocks(patched, blocks);
      },
      focusHeading(index) {
        const heading = root.querySelectorAll('.ProseMirror h1,.ProseMirror h2,.ProseMirror h3,.ProseMirror h4,.ProseMirror h5,.ProseMirror h6')[index];
        if (heading) heading.scrollIntoView({ block: 'center' });
      },
      blockLine(token) {
        const block = blocks.find(item => item.token === token);
        if (!block) return -1;
        const offset = source.indexOf(block.originalSource);
        return offset < 0 ? -1 : source.slice(0, offset).split('\n').length - 1;
      },
      block(token) {
        const block = blocks.find(item => item.token === token);
        return block && { kind: block.kind, source: block.source };
      },
      updateBlock(token, value) {
        const block = blocks.find(item => item.token === token);
        if (!block || !['fence', 'callout'].includes(block.kind)) throw new Error('Block is not editable here.');
        const previous = block.source;
        block.source = value;
        try { restoreBlocks(patched, blocks); }
        catch (error) { block.source = previous; throw error; }
        if (block.kind === 'fence') renderFenceStyles();
        onChange();
      },
      async destroy() {
        fenceStyle.remove();
        await editor.destroy();
      },
    };
  } catch (error) {
    fenceStyle.remove();
    await editor.destroy();
    throw error;
  }
}

window.DynoVisualEditor = { create };
