// Lightweight DOM interaction checks; not a substitute for browser layout or accessibility testing.
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const script = fs.readFileSync(__dirname + '/viewer.js', 'utf8');

class Element {
  constructor(tag) {
    this.tag = tag; this.children = []; this.attributes = {}; this.handlers = {};
    this.classes = new Set(); this.textContent = '';
    this.classList = {
      toggle: (name, on) => { if (on) this.classes.add(name); else this.classes.delete(name); },
      contains: name => this.classes.has(name)
    };
  }
  appendChild(child) { this.children.push(child); return child; }
  replaceChildren() { this.children = []; }
  setAttribute(key, value) {
    this.attributes[key] = String(value);
    if (key === 'class') this.classes = new Set(String(value).split(' '));
  }
  addEventListener(name, handler) { this.handlers[name] = handler; }
  fire(name, props = {}) { this.handlers[name]({...props, preventDefault() { this.prevented = true; }}); }
  querySelectorAll(selector) {
    return this.children.flatMap(child => [child, ...child.querySelectorAll(selector)])
      .filter(child => child.classList.contains(selector.slice(1)));
  }
  closest(selector) { return this.classList.contains(selector.slice(1)) ? this : null; }
  getBoundingClientRect() { return {width: 1000, height: 600}; }
  setPointerCapture() {}
}
function start(model) {
  const ids = Object.fromEntries(['graph', 'viewport', 'details', 'globals', 'title', 'description',
    'search', 'zoom-in', 'zoom-out', 'reset'].map(id => [id, new Element(id)]));
  ids['model-data'] = {textContent: JSON.stringify(model)};
  vm.runInNewContext(script, {document: {
    getElementById: id => ids[id],
    createElement: tag => new Element(tag),
    createElementNS: (ns, tag) => new Element(tag)
  }});
  const all = root => [root, ...root.children.flatMap(all)];
  const nodes = () => ids.viewport.querySelectorAll('.node');
  const details = () => all(ids.details).map(e => e.textContent);
  return {ids, all, nodes, details};
}
const rel = (entity, role, cardinality, ownership = 'referenced') => ({entity, role, cardinality, ownership});

test('empty model initializes controls and global empty states', () => {
  const {ids, nodes, details, all} = start({entities: []});
  assert.equal(nodes().length, 0);
  assert.equal(ids.graph.attributes.viewBox, '0 0 500 280');
  assert.equal(all(ids.globals).filter(e => e.textContent === 'None').length, 5);
  assert.deepEqual(details(), ['']);
});

test('populated graph routes directed parallel edges and lists self roles', () => {
  const {ids, nodes, all, details} = start({title: 'Example', entities: [
    {name: 'Child', subtypeOf: 'Parent', derived: true, relationships: [
      rel('Parent', 'owner', '1:1', 'owned'), rel('Parent', 'reviewer', '0..1:n'),
      rel('Child', 'previous', '1:0..1'), rel('Child', 'next', '0..1:1')]},
    {name: 'Parent', relationships: [rel('Child', 'children', '1:n')]}
  ]});
  const graph = all(ids.viewport);
  const paths = graph.filter(e => e.tag === 'path');
  assert.equal(paths.length, 4);
  assert.equal(new Set(paths.map(p => p.attributes.d)).size, 4);
  assert.ok(paths.every(p => p.attributes['marker-end'] === 'url(#arrow)'));
  const edgeLabels = graph.filter(e => e.tag === 'text' && e.classList.contains('edge-label'));
  assert.deepEqual(edgeLabels.map(e => e.textContent), ['derived', 'previous · 1:0..1', 'next · 0..1:1', 'entity',
    'owner · 1:1', 'reviewer · 0..1:n', 'is-a', 'children · 1:n']);
  assert.equal(new Set(edgeLabels.slice(-4).map(e => `${e.attributes.x},${e.attributes.y}`)).size, 4);
  assert.deepEqual(paths.map(p => p.attributes.class), ['edge owned', 'edge referenced', 'edge isa', 'edge referenced']);
  assert.equal(nodes()[0].children.find(e => e.tag === 'rect').attributes.height, '110');
  nodes()[0].fire('click');
  assert.ok(details().includes('Child → Child · previous · 1:0..1 · referenced'));
  assert.ok(details().includes('Parent → Child · children · 1:n · referenced'));
  nodes()[1].fire('keydown', {key: 'Enter'});
  assert.equal(details()[1], 'Parent');
  assert.equal(nodes()[1].classList.contains('selected'), true);
});

test('external incoming, global values and hostile text remain text only', () => {
  const hostile = `<img src=x onerror=alert(1)> " ' &`;
  const {ids, nodes, all, details} = start({title: hostile, description: hostile,
    invariants: [{id: 'i', statement: hostile}], enums: [{name: 'e', values: [{name: hostile}]}],
    glossary: [{name: hostile, definition: hostile}], scenarios: [{name: hostile}],
    imports: [{scope: hostile, path: hostile}], entities: [
      {name: hostile, relationships: [rel('remote.Target', hostile, '1:n')]},
      {name: 'remote.Target', external: true}
    ]});
  assert.equal(ids.title.textContent, hostile);
  assert.equal(ids.description.textContent, hostile);
  assert.ok(all(ids.globals).some(e => e.textContent.includes(hostile)));
  assert.ok(nodes()[0].children.some(e => e.tag === 'title' && e.textContent === hostile));
  assert.ok(all(ids.viewport).every(e => !Object.values(e.attributes).some(v => v.includes(hostile))));
  nodes()[1].fire('keydown', {key: ' '});
  assert.ok(details().includes(`${hostile} → remote.Target · ${hostile} · 1:n · referenced`));
  assert.ok(details().includes('Defined by an imported model; details are not included in this file.'));
});

test('search, zoom, reset, keyboard pan and pointer pan', () => {
  const {ids, nodes} = start({entities: [{name: 'Alpha'}, {name: 'Beta'}]});
  const initial = ids.graph.attributes.viewBox;
  ids.search.value = 'alp'; ids.search.fire('input');
  assert.deepEqual(nodes().map(n => n.classList.contains('dim')), [false, true]);
  ids['zoom-in'].fire('click');
  assert.notEqual(ids.graph.attributes.viewBox, initial);
  ids['zoom-out'].fire('click');
  ids.reset.fire('click');
  assert.equal(ids.graph.attributes.viewBox, initial);
  ids.graph.fire('keydown', {key: 'ArrowRight'});
  assert.notEqual(ids.graph.attributes.viewBox, initial);
  ids.graph.fire('keydown', {key: '0'});
  assert.equal(ids.graph.attributes.viewBox, initial);
  ids.graph.fire('pointerdown', {target: ids.graph, clientX: 10, clientY: 10, pointerId: 1});
  ids.graph.fire('pointermove', {clientX: 20, clientY: 10});
  assert.notEqual(ids.graph.attributes.viewBox, initial);
  ids.graph.fire('pointerup');
});
