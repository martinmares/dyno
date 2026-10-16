import { build, transform } from 'esbuild';
import { readFile, writeFile } from 'node:fs/promises';

// The reader and editor load exactly the same modes and runMode tokenizer.
const syntaxFiles = [
  'codemirror.min.js', 'codemirror-shell.min.js', 'codemirror-go.min.js',
  'codemirror-yaml.min.js', 'codemirror-javascript.min.js',
  'codemirror-python.min.js', 'codemirror-xml.min.js',
  'codemirror-overlay.min.js', 'codemirror-markdown.min.js',
  'codemirror-continuelist.min.js', 'codemirror-runmode.min.js', 'syntax.js',
];
const syntaxSource = await Promise.all(syntaxFiles.map(name => readFile('assets/' + name, 'utf8')));
const syntax = await transform(syntaxSource.join('\n;\n'), { minify: true, legalComments: 'inline' });
await writeFile('assets/syntax.min.js', syntax.code);

await build({
  entryPoints: ['assets/editor-visual.js'],
  bundle: true,
  minify: true,
  format: 'iife',
  outfile: 'assets/editor-visual.min.js',
});

await build({
  entryPoints: ['assets/editor-visual.css'],
  bundle: true,
  minify: true,
  outfile: 'assets/editor-visual.min.css',
  plugins: [{
    name: 'omit-disabled-latex-styles',
    setup(build) {
      build.onResolve({ filter: /^katex\/dist\/katex\.min\.css$/ }, () => ({
        path: 'disabled-latex.css', namespace: 'empty-css',
      }));
      build.onLoad({ filter: /.*/, namespace: 'empty-css' }, () => ({
        contents: '', loader: 'css',
      }));
    },
  }],
});
