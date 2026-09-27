// Lightweight DOM interaction and geometry checks; the getBBox implementation below is a deterministic
// measurement mock, not a substitute for real browser font/layout or accessibility testing.
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const script = fs.readFileSync(__dirname + '/viewer.js', 'utf8');

class Element {
  constructor(tag, measure) {
    this.tag = tag; this.measure = measure; this.children = []; this.attributes = {}; this.handlers = {};
    this.classes = new Set(); this.textContent = '';
    this.classList = {
      toggle: (name, on) => { if (on) this.classes.add(name); else this.classes.delete(name); },
      contains: name => this.classes.has(name)
    };
  }
  appendChild(child) { child.parentNode = this; this.children.push(child); return child; }
  insertBefore(child, reference) {
    child.parentNode = this;
    this.children.splice(this.children.indexOf(reference), 0, child);
    return child;
  }
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
  getBBox() {
    const number = name => Number(this.attributes[name] || 0);
    if (this.tag === 'rect') return {x: number('x'), y: number('y'), width: number('width'), height: number('height')};
    if (this.tag === 'text') {
      const custom = this.measure?.(this);
      if (custom) return custom;
      const width = this.textContent.length * (this.classList.contains('node-label') ? 8 : 7);
      return {x: number('x') - width / 2, y: number('y') - 10, width, height: 14};
    }
    return {x: 0, y: 0, width: 0, height: 0};
  }
  setPointerCapture() {}
}
function start(model, measure) {
  const ids = Object.fromEntries(['graph', 'viewport', 'details', 'globals', 'title', 'description',
    'search', 'zoom-in', 'zoom-out', 'reset'].map(id => [id, new Element(id, measure)]));
  ids['model-data'] = {textContent: JSON.stringify(model)};
  vm.runInNewContext(script, {document: {
    getElementById: id => ids[id],
    createElement: tag => new Element(tag, measure),
    createElementNS: (ns, tag) => new Element(tag, measure)
  }});
  const all = root => [root, ...root.children.flatMap(all)];
  const nodes = () => ids.viewport.querySelectorAll('.node');
  const details = () => all(ids.details).map(e => e.textContent);
  return {ids, all, nodes, details};
}
const rel = (entity, role, cardinality, ownership = 'referenced') => ({entity, role, cardinality, ownership});
const intersects = (a, b) => a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;
const nodeGeometry = layout => layout.nodes().flatMap(node => node.children
  .filter(child => child.tag === 'rect' || child.tag === 'text').map(child => child.getBBox()));
const labelLayer = layout => layout.ids.viewport.children.find(group => group.children.some(child => child.classList.contains('edge-label')));

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

test('dense edge labels avoid nodes and each other with deterministic bounded view', () => {
  const long = 'delegated provider discovery reconciliation responsibility';
  const model = {entities: [
    {name: 'ProviderDiscovery', relationships: [rel('ReadLedger', long, '1:n'), rel('Session', 'provider lookup', '0..1:n'), rel('AuditTrail', 'audit source', '1:n')]},
    {name: 'ReadLedger', relationships: [rel('ProviderDiscovery', 'reverse ledger reconciliation', 'n:1'), rel('Session', long, '1:n'), rel('TokenExchange', 'parallel token read', '1:n')]},
    {name: 'Session', relationships: [rel('ProviderDiscovery', long, 'n:1'), rel('ReadLedger', 'reverse session ledger', 'n:1'), rel('Session', 'previous session', '1:0..1'), rel('Session', 'next session', '0..1:1'), rel('Session', 'superseded session', '0..1:1')]},
    {name: 'Authorizer', relationships: [rel('Session', long, '1:n'), rel('ReadLedger', 'authorization ledger link', '1:n')]},
    {name: 'TokenExchange', relationships: [rel('ProviderDiscovery', 'token provider linkage', 'n:1'), rel('Session', long, '1:n')]},
    {name: 'AuditTrail', subtypeOf: 'ProviderDiscovery', relationships: [rel('Session', 'session audit history', 'n:1')]}
  ]};
  const layout = start(model);
  const labelLayer = layout.ids.viewport.children.find(group => group.children.some(child => child.classList.contains('edge-label')));
  const labels = labelLayer.children.filter(child => child.classList.contains('edge-label'));
  const nodeBoxes = nodeGeometry(layout);
  const boxes = labels.map(label => label.getBBox());
  assert.ok(labels.some(label => label.textContent === `${long} · 1:n`));
  assert.ok(labels.some(label => label.textContent === 'is-a'));
  assert.ok(labels.some(label => label.textContent === 'reverse ledger reconciliation · n:1'));
  assert.equal(layout.nodes().find(node => node.children.some(child => child.textContent === 'Session')).children.find(child => child.tag === 'rect').attributes.height, '130');
  for (const box of boxes) assert.ok(nodeBoxes.every(node => !intersects(box, node)), `label overlaps node geometry: ${JSON.stringify(box)}`);
  for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++) {
    assert.equal(intersects(boxes[i], boxes[j]), false, `labels ${i} and ${j} overlap`);
  }
  const view = layout.ids.graph.attributes.viewBox.split(' ').map(Number);
  assert.ok(view.every(Number.isFinite));
  for (const box of boxes) {
    assert.ok(box.x >= view[0] && box.y >= view[1] && box.x + box.width <= view[0] + view[2] && box.y + box.height <= view[1] + view[3]);
  }
  const repeat = start(model);
  const repeatLayer = repeat.ids.viewport.children.find(group => group.children.some(child => child.classList.contains('edge-label')));
  assert.deepEqual(labels.map(label => [label.attributes.x, label.attributes.y]), repeatLayer.children.filter(child => child.classList.contains('edge-label')).map(label => [label.attributes.x, label.attributes.y]));
  assert.equal(layout.ids.graph.attributes.viewBox, repeat.ids.graph.attributes.viewBox);
});

test('relationship labels avoid long rendered node text and rectangles', () => {
  const longName = 'A'.repeat(60);
  const layout = start({entities: [
    {name: longName, relationships: [rel('Target', 'owner', '1:n')]},
    {name: 'Target'}
  ]});
  const labels = labelLayer(layout).children.filter(child => child.classList.contains('edge-label'));
  const owner = labels.find(label => label.textContent === 'owner · 1:n');
  assert.ok(owner);
  for (const box of [owner.getBBox()]) {
    assert.ok(nodeGeometry(layout).every(node => !intersects(box, node)), `label overlaps node geometry: ${JSON.stringify(box)}`);
  }
});

test('relationship label fallback adds a linked leader', () => {
  let attempts = 0;
  const layout = start({entities: [
    {name: 'Source', relationships: [rel('Target', 'forced fallback', '1:n')]},
    {name: 'Target'}
  ]}, element => {
    if (element.textContent === 'forced fallback · 1:n' && attempts++ < 61) return {x: 40, y: 55, width: 250, height: 70};
  });
  const labels = labelLayer(layout);
  const label = labels.children.find(child => child.textContent === 'forced fallback · 1:n');
  const index = labels.children.indexOf(label);
  const leader = labels.children[index - 1];
  assert.equal(attempts, 62);
  assert.equal(leader.classList.contains('edge-label-leader'), true);
  assert.ok(leader.attributes.d.endsWith(`L ${label.attributes.x} ${label.attributes.y}`));
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
