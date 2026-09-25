import { build } from 'esbuild';

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
