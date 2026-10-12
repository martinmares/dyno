import assert from 'node:assert/strict';
import test from 'node:test';
import { apiBlockLabel, parseAPIBlock, serializeAPIBlock } from '../assets/editor-api.mjs';

const source = '```api-insecure no-auth\n{{HTTP_METHOD_FOR_GET}} {{HTTPBIN_URL}}/get\nAccept: application/json\nAuthorization: Bearer {{token}}\n@follow-jsonpath: $[*].labels.__metrics_path__\n@follow-method: GET\n```\n';

test('parses placeholders, headers, fence options and follow-up directives without expanding them', () => {
  const spec = parseAPIBlock(source);
  assert.equal(spec.formAvailable, true);
  assert.equal(spec.method, '{{HTTP_METHOD_FOR_GET}}');
  assert.equal(spec.url, '{{HTTPBIN_URL}}/get');
  assert.equal(spec.insecure, true);
  assert.equal(spec.noAuth, true);
  assert.deepEqual(spec.headers, [['Accept', 'application/json'], ['Authorization', 'Bearer {{token}}']]);
  assert.equal(spec.followJSONPath, '$[*].labels.__metrics_path__');
  assert.equal(spec.followMethod, 'GET');
  assert.equal(serializeAPIBlock(spec, spec), source);
  assert.equal(apiBlockLabel(source), 'API · {{HTTP_METHOD_FOR_GET}} {{HTTPBIN_URL}}/get');
});

test('form updates preserve fence width, indentation, closing fence, CRLF and missing final newline', () => {
  const original = '  ~~~~api\r\nGET {{BASE}}/items\r\nAccept: application/json\r\n  ~~~~~';
  const spec = parseAPIBlock(original);
  const edited = serializeAPIBlock(spec, { ...spec, method: 'POST', url: '{{BASE}}/new', noAuth: true });
  assert.equal(edited, '  ~~~~api no-auth\r\nPOST {{BASE}}/new\r\nAccept: application/json\r\n  ~~~~~');
});

test('unsupported content remains available in Source and cannot be silently converted to Form', () => {
  for (const body of ['@custom: value', 'Content without a colon', '@follow-method: GET\n@follow-method: POST', '{"json": "body"}']) {
    const raw = '```api\nGET {{BASE}}/items\n' + body + '\n```\n';
    const spec = parseAPIBlock(raw);
    assert.equal(spec.formAvailable, false);
    assert.equal(spec.source, raw);
    assert.throws(() => serializeAPIBlock(spec, spec));
  }
  assert.equal(parseAPIBlock('```api custom-flag\nGET /items\n```').formAvailable, false);
});

test('preserves duplicate headers and values containing colons', () => {
  const raw = '```api\nGET /items\nX-Tag: one\nX-Tag: two\nOrigin: https://example.com\nX-Empty:\n```';
  const spec = parseAPIBlock(raw);
  assert.deepEqual(spec.headers, [['X-Tag', 'one'], ['X-Tag', 'two'], ['Origin', 'https://example.com'], ['X-Empty', '']]);
  assert.deepEqual(parseAPIBlock(serializeAPIBlock(spec, spec)).headers, spec.headers);
});

test('rejects incomplete fences, multiple blocks, missing requests and invalid form values', () => {
  for (const raw of ['```api\nGET /items', '```javascript\nGET /items\n```', '```api\nGET\n```', '```api\n```', '```api\nGET /one\n```\n```api\nGET /two\n```']) {
    assert.throws(() => parseAPIBlock(raw));
  }
  const spec = parseAPIBlock(source);
  for (const fields of [{ method: '' }, { method: 'GET POST' }, { url: '' }, { url: '/one\n/another' }, { headers: [['', 'value']] }, { headers: [['Bad name', 'value']] }, { headers: [['Accept', 'one\nX-Injected: two']] }, { followMethod: 'GET POST' }]) {
    assert.throws(() => serializeAPIBlock(spec, { ...spec, ...fields }));
  }
});
