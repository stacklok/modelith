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
    if (child.parentNode) child.parentNode.children.splice(child.parentNode.children.indexOf(child), 1);
    child.parentNode = this;
    this.children.splice(this.children.indexOf(reference), 0, child);
    return child;
  }
  replaceChildren() { this.children = []; }
  setAttribute(key, value) {
    this.attributes[key] = String(value);
    if (key === 'class') this.classes = new Set(String(value).split(' '));
  }
  getAttribute(key) { return this.attributes[key]; }
  addEventListener(name, handler) { this.handlers[name] = handler; }
  fire(name, props = {}) { this.handlers[name]({...props, type: name, target: props.target || this, preventDefault() { this.prevented = true; }, stopPropagation() { this.stopped = true; }}); }
  querySelectorAll(selector) {
    return this.children.flatMap(child => [child, ...child.querySelectorAll(selector)])
      .filter(child => child.classList.contains(selector.slice(1)));
  }
  closest(selector) { return this.classList.contains(selector.slice(1)) ? this : this.parentNode?.closest(selector) || null; }
  getScreenCTM() {
    const [x, y, w, h] = this.attributes.viewBox.split(' ').map(Number);
    const scale = Math.min(1000 / w, 600 / h);
    const left = (1000 - w * scale) / 2, top = (600 - h * scale) / 2;
    return {inverse: () => ({x, y, scale, left, top})};
  }
  createSVGPoint() { return {x: 0, y: 0, matrixTransform(m) { return {x: m.x + (this.x - m.left) / m.scale, y: m.y + (this.y - m.top) / m.scale}; }}; }
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
  focus() { this.focused = true; }
  setPointerCapture(id) { this.capture = id; }
  hasPointerCapture(id) { return this.capture === id; }
  releasePointerCapture(id) { if (this.capture === id) this.capture = undefined; }
}
function start(model, measure, source = script, globals = {}) {
  const ids = Object.fromEntries(['graph', 'viewport', 'details', 'globals', 'title', 'description',
    'search', 'zoom-in', 'zoom-out', 'layout', 'arrange', 'fit'].map(id => [id, new Element(id, measure)]));
  ids.layout.value = 'grid';
  const frames = new Map();
  let nextFrame = 0;
  ids['model-data'] = {textContent: JSON.stringify(model)};
  vm.runInNewContext(source, {requestAnimationFrame: callback => { frames.set(++nextFrame, callback); return nextFrame; },
    cancelAnimationFrame: id => frames.delete(id), document: {
    getElementById: id => ids[id],
    createElement: tag => new Element(tag, measure),
    createElementNS: (ns, tag) => new Element(tag, measure)
  }, ...globals});
  const all = root => [root, ...root.children.flatMap(all)];
  const nodes = () => ids.viewport.querySelectorAll('.node');
  const details = () => all(ids.details).map(e => e.textContent);
  return {ids, all, nodes, details, flush: () => { for (const [id, callback] of frames) { frames.delete(id); callback(); } }};
}
function productionSegmentHits() {
  let segmentHits;
  const source = script.replace('  const labelMeasurements = new Map();', '  globalThis.__captureSegmentHits(segmentHits);\n  const labelMeasurements = new Map();');
  assert.notEqual(source, script, 'instrumentation marker must match production helper boundary');
  start({entities: []}, undefined, source, {__captureSegmentHits: helper => { segmentHits = helper; }});
  return segmentHits;
}
const rel = (entity, role, cardinality, ownership = 'referenced') => ({entity, role, cardinality, ownership});

test('segmentHits clips horizontal, vertical, point, boundary, reverse and diagonal segments', () => {
  const segmentHits = productionSegmentHits();
  const box = {x: 10, y: 10, width: 10, height: 10};
  const cases = [
    ['horizontal intersects', {x: 0, y: 15}, {x: 30, y: 15}, true],
    ['horizontal misses', {x: 0, y: 5}, {x: 30, y: 5}, false],
    ['vertical intersects', {x: 15, y: 0}, {x: 15, y: 30}, true],
    ['vertical misses', {x: 5, y: 0}, {x: 5, y: 30}, false],
    ['point inside', {x: 15, y: 15}, {x: 15, y: 15}, true],
    ['point outside', {x: 5, y: 5}, {x: 5, y: 5}, false],
    ['point on boundary', {x: 10, y: 15}, {x: 10, y: 15}, true],
    ['endpoint touching', {x: 0, y: 15}, {x: 10, y: 15}, true],
    ['reverse direction', {x: 30, y: 15}, {x: 0, y: 15}, true],
    ['diagonal misses', {x: 0, y: 0}, {x: 5, y: 5}, false],
    ['diagonal hits', {x: 0, y: 0}, {x: 30, y: 30}, true]
  ];
  for (const [name, a, b, expected] of cases) assert.equal(segmentHits(a, b, box), expected, name);
});
const intersects = (a, b) => a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;
const nodeGeometry = layout => layout.nodes().flatMap(node => node.children
  .filter(child => child.tag === 'rect' || child.tag === 'text').map(child => child.getBBox()));
const labelLayer = layout => layout.ids.viewport.children.find(group => group.children.some(child => child.classList.contains('edge-label')));
const rect = node => node.children.find(child => child.tag === 'rect');
const nodeX = node => Number(rect(node).attributes.x);
const nodeY = node => Number(rect(node).attributes.y);
const edgePaths = layout => layout.all(layout.ids.viewport).filter(e => e.classList.contains('edge'));
const pathNumbers = element => element.attributes.d.match(/-?\d+(?:\.\d+)?(?:e[+-]?\d+)?/gi).map(Number);
const contains = (box, x, y) => x >= box.x && x <= box.x + box.width && y >= box.y && y <= box.y + box.height;
// Independent segment/rectangle oracle: intersect the four sides, including boundary contact.
const hitsBox = ([ax, ay, bx, by], box) => {
  if (contains(box, ax, ay) || contains(box, bx, by)) return true;
  for (const x of [box.x, box.x + box.width]) {
    const t = (x - ax) / (bx - ax);
    if (t >= 0 && t <= 1 && ay + t * (by - ay) >= box.y && ay + t * (by - ay) <= box.y + box.height) return true;
  }
  for (const y of [box.y, box.y + box.height]) {
    const t = (y - ay) / (by - ay);
    if (t >= 0 && t <= 1 && ax + t * (bx - ax) >= box.x && ax + t * (bx - ax) <= box.x + box.width) return true;
  }
  return false;
};
const assertLabelGeometry = layout => {
  const children = labelLayer(layout).children;
  const labels = children.filter(e => e.classList.contains('edge-label'));
  const obstacles = nodeGeometry(layout);
  const paths = edgePaths(layout);
  assert.equal(labels.length, paths.length);
  labels.forEach((label, i) => {
    const box = label.getBBox();
    assert.ok(obstacles.every(other => !intersects(box, other)), `label ${i} overlaps node/text`);
    labels.forEach((other, j) => { if (i !== j) assert.ok(!intersects(box, other.getBBox()), `labels ${i}/${j} overlap`); });
    const leader = children[children.indexOf(label) - 1];
    const hasLeader = leader?.classList.contains('edge-label-leader');
    if (!hasLeader && label.textContent.includes(' → ')) return;
    const segment = hasLeader ? pathNumbers(leader) : [box.x + box.width / 2, box.y + box.height / 2];
    const [ax, ay, bx, by] = segment;
    const [sx, sy, cx, cy, ex, ey] = pathNumbers(paths[i]);
    const samples = Array.from({length: 19}, (_, n) => (n + 1) / 20);
    assert.ok(samples.some(t => Math.hypot(ax - ((1-t)**2*sx + 2*(1-t)*t*cx + t*t*ex),
      ay - ((1-t)**2*sy + 2*(1-t)*t*cy + t*t*ey)) < 1e-7), `label ${i} anchor not on sampled curve`);
    assert.ok(obstacles.every(other => !contains(other, ax, ay)), `anchor ${i} hidden by node/text`);
    if (!hasLeader) return;
    assert.ok(obstacles.every(other => !hitsBox(segment, other)), `leader ${i} crosses node/text`);
    labels.forEach((other, j) => { if (i !== j) assert.ok(!hitsBox(segment, other.getBBox()), `leader ${i} crosses label ${j}`); });
    assert.ok(bx >= box.x - 1e-7 && bx <= box.x + box.width + 1e-7 && by >= box.y - 1e-7 && by <= box.y + box.height + 1e-7);
    assert.ok(Math.min(Math.abs(bx - box.x), Math.abs(bx - box.x - box.width), Math.abs(by - box.y), Math.abs(by - box.y - box.height)) < 1e-7,
      `leader ${i} must stop at label border`);
    assert.ok(Math.hypot(bx - ax, by - ay) <= 40, 'leader must stay short');
  });
};

const arrange = (layout, mode) => { layout.ids.layout.value = mode; layout.ids.arrange.fire('click'); };
const pointer = (layout, name, x, y, id = 1) => {
  const target = name ? layout.nodes().find(n => n.children.some(c => c.textContent === name)) : layout.ids.graph;
  layout.ids.graph.fire('pointerdown', {target, button: 0, isPrimary: true, clientX: x, clientY: y, pointerId: id});
  return target;
};

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
  const paths = graph.filter(e => e.tag === 'path' && e.classList.contains('edge'));
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
  assert.ok(labelLayer.children.some(e => e.classList.contains('edge-label-leader')), 'exercise actual leaders');
  assertLabelGeometry(layout);
  pointer(layout, 'ReadLedger', 200, 200);
  layout.ids.graph.fire('pointermove', {pointerId: 1, clientX: 240, clientY: 180});
  layout.ids.graph.fire('pointerup', {pointerId: 1});
  layout.flush();
  assertLabelGeometry(layout);
  for (const mode of ['flow-down', 'flow-right', 'grid']) {
    arrange(layout, mode);
    assertLabelGeometry(layout);
  }
});

test('leaders avoid prior labels and later labels cannot cover earlier leaders', () => {
  // Reduced adversarial fixtures: removing either segment reservation guard breaks its case.
  const cases = [
    [[2, 7, 6, 10], [2, 4, 20, 8], [2, 5, 22, 6], [2, 5, 24, 8], [2, 6, 30, 6], [4, 7, 0, 4],
      [6, 3, 5, 5], [6, 5, 8, 4], [6, 2, 17, 1], [6, 3, 27, 11], [6, 1, 28, 8], [6, 2, 33, 1]],
    [[0, 8, 13, 6], [1, 7, 32, 3], [4, 5, 11, 0], [4, 5, 19, 8], [4, 8, 27, 5], [4, 7, 31, 6],
      [5, 7, 6, 10], [5, 7, 15, 4]]
  ];
  for (const connections of cases) {
    const entities = Array.from({length: 9}, (_, i) => ({name: `Node${i}`, relationships: []}));
    for (const [source, target, id, repeats] of connections)
      entities[source].relationships.push(rel(`Node${target}`, `${id}-` + 'long role '.repeat(repeats), '1:n'));
    const layout = start({entities});
    assertLabelGeometry(layout);
    const labels = labelLayer(layout).children.filter(e => e.tag === 'text');
    entities.flatMap(entity => entity.relationships.map(r => ({source: entity.name, ...r}))).forEach((r, i) => {
      const value = `${r.role} · ${r.cardinality}`;
      assert.ok(labels[i].textContent === value || labels[i].textContent === `${r.source} → ${r.entity} · ${value}`);
    });
    layout.nodes()[4].fire('keydown', {key: 'ArrowDown', altKey: true});
    layout.flush();
    assertLabelGeometry(layout);
    arrange(layout, 'grid');
    assertLabelGeometry(layout);
  }
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

test('fully obscured edges use deterministic explicit overflow labels, never fake leaders', () => {
  const model = {entities: [
    {name: 'Source', relationships: [rel('Target', 'forced fallback', '1:n'), rel('Target', '<offers>', '0..1:n')]},
    {name: 'Target'}
  ]};
  // A rendered node title obscures every candidate curve point, not a changing measurement mock.
  const measure = element => element.textContent === 'Source' && element.tag === 'text'
    ? {x: Number(element.attributes.x) - 2000, y: Number(element.attributes.y) - 2000, width: 4000, height: 4000} : undefined;
  const a = start(model, measure), b = start(model, measure);
  for (const layout of [a, b]) {
    const labels = labelLayer(layout).children;
    assert.deepEqual(labels.map(e => e.textContent), ['Source → Target · forced fallback · 1:n', 'Source → Target · <offers> · 0..1:n']);
    assert.ok(labels.every(e => e.tag === 'text'));
    assertLabelGeometry(layout);
    const [x, y, w, h] = layout.ids.graph.attributes.viewBox.split(' ').map(Number);
    assert.ok([x, y, w, h].every(Number.isFinite));
    for (const label of labels) {
      const box = label.getBBox();
      assert.ok(box.x >= x && box.y >= y && box.x + box.width <= x + w && box.y + box.height <= y + h);
    }
  }
  assert.deepEqual(labelLayer(a).children.map(e => e.attributes), labelLayer(b).children.map(e => e.attributes));
});

test('same-column spanning edges choose visible curve anchors around middle node text', () => {
  const model = {entities: [
    {name: 'Source', relationships: [rel('Target', 'offers', '1:n'), rel('Target', 'holds', '0..1:n'), rel('Target', 'executes', '1:1')]},
    {name: 'SideA'}, {name: 'SideB'},
    {name: 'PermissionMode'}, {name: 'ReadLedger'}, {name: 'ProviderDiscovery'},
    {name: 'Target', subtypeOf: 'Source', relationships: [rel('Source', 'reverse', 'n:1')]}
  ]};
  const layout = start(model);
  const [sx, sy, cx, cy, ex, ey] = pathNumbers(edgePaths(layout)[0]);
  const middleText = layout.nodes()[3].children.filter(e => e.tag === 'text').map(e => e.getBBox());
  assert.ok(middleText.some(box => contains(box, (sx + 2*cx + ex)/4, (sy + 2*cy + ey)/4)));
  assertLabelGeometry(layout);
  for (const value of ['offers · 1:n', 'holds · 0..1:n', 'executes · 1:1', 'is-a', 'reverse · n:1'])
    assert.ok(labelLayer(layout).children.some(e => e.textContent === value), `visible edge should retain local ${value}`);
  pointer(layout, 'PermissionMode', 200, 200);
  layout.ids.graph.fire('pointermove', {pointerId: 1, clientX: 220, clientY: 230});
  layout.ids.graph.fire('pointerup', {pointerId: 1});
  layout.flush();
  assertLabelGeometry(layout);
  for (const mode of ['flow-down', 'flow-right', 'grid']) {
    arrange(layout, mode);
    assertLabelGeometry(layout);
  }
});

test('label measurement is cached per content, not per placement or drag candidate', () => {
  const reads = new Map();
  const layout = start({entities: [
    {name: 'A', relationships: [rel('B', 'role', '1:n'), rel('B', 'role', '1:n')]}, {name: 'B'}
  ]}, element => {
    if (element.textContent.includes(' · ')) reads.set(element.textContent, (reads.get(element.textContent) || 0) + 1);
  });
  for (let i = 0; i < 5; i++) {
    layout.nodes()[0].fire('keydown', {key: 'ArrowRight', altKey: true});
    layout.flush();
  }
  arrange(layout, 'grid');
  assert.equal(reads.get('role · 1:n'), 1);
  assert.ok([...reads.values()].every(count => count === 1));
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

test('search, zoom, fit, keyboard pan and pointer pan', () => {
  const {ids, nodes} = start({entities: [{name: 'Alpha'}, {name: 'Beta'}]});
  const initial = ids.graph.attributes.viewBox;
  ids.search.value = 'alp'; ids.search.fire('input');
  assert.deepEqual(nodes().map(n => n.classList.contains('dim')), [false, true]);
  ids['zoom-in'].fire('click');
  assert.notEqual(ids.graph.attributes.viewBox, initial);
  ids['zoom-out'].fire('click');
  ids.fit.fire('click');
  assert.equal(ids.graph.attributes.viewBox, initial);
  ids.graph.fire('keydown', {key: 'ArrowRight'});
  assert.notEqual(ids.graph.attributes.viewBox, initial);
  ids.graph.fire('keydown', {key: '0'});
  assert.equal(ids.graph.attributes.viewBox, initial);
  ids.graph.fire('pointerdown', {target: ids.graph, button: 0, clientX: 10, clientY: 10, pointerId: 1});
  ids.graph.fire('pointermove', {clientX: 20, clientY: 10, pointerId: 1});
  assert.notEqual(ids.graph.attributes.viewBox, initial);
  ids.graph.fire('pointerup', {pointerId: 1});
});

test('arrangements cover cycles, disconnected nodes, self roles and empty graphs deterministically', () => {
  const model = {entities: [
    {name: 'Root', relationships: [rel('LongNode', 'child', '1:n')]},
    {name: 'LongNode', relationships: [rel('Leaf', 'next', '1:n')]},
    {name: 'Leaf', relationships: [rel('LongNode', 'back', '1:n')]},
    {name: 'Solo', relationships: [rel('Solo', 'self', '1:1')]},
    {name: 'Unconnected'}
  ]};
  for (const mode of ['grid', 'flow-down', 'flow-right']) {
    const a = start(model), b = start(model);
    arrange(a, mode); arrange(b, mode);
    assert.deepEqual(a.nodes().map(n => [nodeX(n), nodeY(n)]), b.nodes().map(n => [nodeX(n), nodeY(n)]));
    assert.equal(a.ids.graph.attributes.viewBox, b.ids.graph.attributes.viewBox);
    const boxes = a.nodes().map(rect).map(r => r.getBBox());
    for (let i = 0; i < boxes.length; i++) for (let j = i + 1; j < boxes.length; j++)
      assert.equal(intersects(boxes[i], boxes[j]), false, `${mode} nodes overlap`);
    assert.equal(edgePaths(a).length, 3);
  }
  for (const mode of ['flow-down', 'flow-right']) {
    const empty = start({entities: []}); arrange(empty, mode);
    assert.equal(empty.ids.graph.attributes.viewBox, '0 0 500 280');
  }
  const down = start(model); arrange(down, 'flow-down');
  assert.ok(nodeY(down.nodes()[0]) < nodeY(down.nodes()[1]));
  assert.ok(nodeY(down.nodes()[1]) < nodeY(down.nodes()[2]));
  const right = start(model); arrange(right, 'flow-right');
  assert.ok(nodeX(right.nodes()[0]) < nodeX(right.nodes()[1]));
  assert.ok(nodeX(right.nodes()[1]) < nodeX(right.nodes()[2]));
});

test('drag uses letterboxed and zoomed SVG coordinates; fit preserves positions and arrange resets them', () => {
  const layout = start({entities: [{name: 'A', relationships: [rel('B', 'owns', '1:n')]}, {name: 'B'}]});
  const {ids, nodes, flush} = layout;
  nodes()[0].focus();
  nodes()[0].fire('click');
  ids.search.value = 'a'; ids.search.fire('input');
  const initial = nodeX(nodes()[0]), initialY = nodeY(nodes()[0]);
  const view = ids.graph.attributes.viewBox;
  const firstEdge = edgePaths(layout)[0].attributes.d;
  const scale = Math.min(1000 / Number(view.split(' ')[2]), 600 / Number(view.split(' ')[3]));
  const target = pointer(layout, 'A', 300, 200);
  assert.equal(target.capture, 1);
  ids.graph.fire('pointermove', {pointerId: 1, clientX: 320, clientY: 220});
  assert.ok(Math.abs(nodeX(nodes()[0]) - initial - 20 / scale) < 1e-8);
  assert.ok(Math.abs(nodeY(nodes()[0]) - initialY - 20 / scale) < 1e-8);
  assert.equal(edgePaths(layout)[0].attributes.d, firstEdge);
  flush();
  assert.notEqual(edgePaths(layout)[0].attributes.d, firstEdge);
  assert.ok(labelLayer(layout).children.some(e => e.textContent === 'owns · 1:n'));
  ids.graph.fire('pointerup', {pointerId: 1});
  assert.equal(target.capture, undefined);
  target.fire('click');
  assert.equal(nodes()[0].classList.contains('selected'), true);
  assert.equal(nodes()[1].classList.contains('dim'), true);
  ids['zoom-in'].fire('click');
  const zoomed = ids.graph.attributes.viewBox.split(' ').map(Number);
  const zoomScale = Math.min(1000 / zoomed[2], 600 / zoomed[3]);
  pointer(layout, 'A', 300, 200);
  ids.graph.fire('pointermove', {pointerId: 1, clientX: 320, clientY: 200});
  assert.ok(Math.abs(nodeX(nodes()[0]) - initial - 20 / scale - 20 / zoomScale) < 1e-8);
  ids.graph.fire('pointerup', {pointerId: 1});
  ids.fit.fire('click');
  const moved = nodeX(nodes()[0]);
  assert.equal(nodeX(nodes()[0]), moved);
  arrange(layout, 'grid');
  assert.equal(nodeX(nodes()[0]), initial);
  assert.equal(nodes()[0], target);
  assert.equal(nodes()[0].focused, true);
  assert.equal(nodes()[0].classList.contains('selected'), true);
  assert.equal(nodes()[1].classList.contains('dim'), true);
});

test('drag suppresses its browser click while a below-threshold gesture remains a click', () => {
  const layout = start({entities: [{name: 'A'}, {name: 'B'}]});
  const {ids, nodes, details} = layout;
  const [a, b] = nodes();

  b.fire('click');
  assert.equal(b.classList.contains('selected'), true);
  assert.equal(details()[1], 'B');

  pointer(layout, 'A', 100, 100);
  ids.graph.fire('pointermove', {pointerId: 1, clientX: 120, clientY: 100});
  ids.graph.fire('pointerup', {pointerId: 1});
  a.fire('click');
  assert.equal(a.classList.contains('selected'), false);
  assert.equal(b.classList.contains('selected'), true);
  assert.equal(details()[1], 'B');

  pointer(layout, 'A', 100, 100);
  ids.graph.fire('pointermove', {pointerId: 1, clientX: 103, clientY: 100});
  ids.graph.fire('pointerup', {pointerId: 1});
  a.fire('click');
  assert.equal(a.classList.contains('selected'), true);
  assert.equal(b.classList.contains('selected'), false);
  assert.equal(details()[1], 'A');
});

test('pointer threshold, IDs, cancellation, capture and keyboard move versus pan', () => {
  const layout = start({entities: [{name: 'A', relationships: [rel('B', 'edge', '1:n')]}, {name: 'B'}]});
  const {ids, nodes, flush, details} = layout;
  const x = nodeX(nodes()[0]);
  ids.graph.fire('pointerdown', {target: nodes()[0], button: 2, pointerId: 9, clientX: 100, clientY: 100});
  ids.graph.fire('pointerdown', {target: nodes()[0], button: 0, isPrimary: false, pointerId: 9, clientX: 100, clientY: 100});
  assert.equal(nodes()[0].capture, undefined);
  const node = pointer(layout, 'A', 100, 100);
  ids.graph.fire('pointermove', {pointerId: 2, clientX: 200, clientY: 100});
  ids.graph.fire('pointermove', {pointerId: 1, clientX: 103, clientY: 100});
  assert.equal(nodeX(node), x);
  ids.graph.fire('pointerup', {pointerId: 2});
  assert.equal(node.capture, 1);
  ids.graph.fire('pointerup', {pointerId: 1});
  node.fire('click');
  assert.equal(details()[1], 'A');
  pointer(layout, 'A', 100, 100);
  ids.graph.fire('pointermove', {pointerId: 1, clientX: 120, clientY: 100});
  ids.graph.fire('pointercancel', {pointerId: 1});
  assert.equal(node.capture, undefined);
  const afterCancel = nodeX(node);
  ids.graph.fire('pointermove', {pointerId: 1, clientX: 150, clientY: 100});
  assert.equal(nodeX(node), afterCancel);
  pointer(layout, 'A', 100, 100);
  ids.graph.fire('lostpointercapture', {pointerId: 1});
  ids.graph.fire('pointermove', {pointerId: 1, clientX: 150, clientY: 100});
  assert.equal(nodeX(node), afterCancel);
  const priorView = ids.graph.attributes.viewBox;
  node.fire('keydown', {key: 'ArrowRight', altKey: true});
  flush();
  assert.equal(nodeX(node), afterCancel + 40);
  assert.equal(ids.graph.attributes.viewBox, priorView);
  ids.graph.fire('keydown', {key: 'ArrowRight', target: node});
  assert.notEqual(ids.graph.attributes.viewBox, priorView);
  const beforePan = nodeX(node);
  const panView = ids.graph.attributes.viewBox;
  pointer(layout, null, 100, 100);
  ids.graph.fire('pointermove', {pointerId: 1, clientX: 130, clientY: 100});
  ids.graph.fire('pointerup', {pointerId: 1});
  assert.equal(nodeX(node), beforePan);
  assert.notEqual(ids.graph.attributes.viewBox, panView);
});

test('flow arrangements reserve long node text and tall self nodes; labels remain clear after movement', () => {
  const model = {entities: [
    {name: 'Very long '.repeat(12), relationships: [rel('Middle', 'source', '1:n')]},
    {name: 'Middle', relationships: [rel('Middle', 'self one', '1:1'), rel('Middle', 'self two', '1:1'), rel('End', 'a very long relation '.repeat(9), '1:n')]},
    {name: 'End', relationships: [rel('Middle', 'reverse', 'n:1')]}
  ]};
  for (const mode of ['flow-down', 'flow-right']) {
    const layout = start(model);
    arrange(layout, mode);
    layout.nodes()[1].fire('keydown', {key: 'ArrowRight', altKey: true});
    layout.flush();
    const boxes = nodeGeometry(layout);
    const labels = labelLayer(layout).children.filter(e => e.classList.contains('edge-label'));
    for (const label of labels) assert.ok(boxes.every(box => !intersects(box, label.getBBox())), `${mode}: label overlaps node`);
    const perNode = layout.nodes().map(node => node.children.filter(e => e.tag === 'rect' || e.tag === 'text').map(e => e.getBBox()));
    for (let i = 0; i < perNode.length; i++) for (let j = i + 1; j < perNode.length; j++)
      assert.ok(perNode[i].every(a => perNode[j].every(b => !intersects(a, b))), `${mode}: node text overlaps another node`);
    layout.ids.fit.fire('click');
    const [x, y, w, h] = layout.ids.graph.attributes.viewBox.split(' ').map(Number);
    for (const label of labels) {
      const box = label.getBBox();
      assert.ok(box.x >= x && box.y >= y && box.x + box.width <= x + w && box.y + box.height <= y + h);
    }
  }
});

test('fit frames translated geometry without returning to origin', () => {
  const layout = start({entities: [{name: 'Far'}]});
  pointer(layout, 'Far', 100, 100);
  layout.ids.graph.fire('pointermove', {pointerId: 1, clientX: 2100, clientY: 100});
  layout.ids.graph.fire('pointerup', {pointerId: 1});
  const moved = nodeX(layout.nodes()[0]);
  layout.ids.fit.fire('click');
  assert.ok(Number(layout.ids.graph.attributes.viewBox.split(' ')[0]) > 0);
  assert.equal(nodeX(layout.nodes()[0]), moved);
});

test('coincident nodes retain finite edge paths and labels after movement', () => {
  const layout = start({entities: [{name: 'A', relationships: [rel('B', 'edge', '1:n')]}, {name: 'B'}]});
  const [a, b] = layout.nodes();
  a.fire('keydown', {key: 'ArrowRight', altKey: true});
  const dx = nodeX(a) - nodeX(b), dy = nodeY(a) - nodeY(b);
  const scale = Math.min(1000 / Number(layout.ids.graph.attributes.viewBox.split(' ')[2]), 600 / Number(layout.ids.graph.attributes.viewBox.split(' ')[3]));
  pointer(layout, 'A', 100, 100);
  layout.ids.graph.fire('pointermove', {pointerId: 1, clientX: 100 - dx * scale, clientY: 100 - dy * scale});
  layout.ids.graph.fire('pointerup', {pointerId: 1});
  layout.flush();
  assert.ok(Math.abs(nodeX(a) - nodeX(b)) < 1e-8);
  assert.ok(Math.abs(nodeY(a) - nodeY(b)) < 1e-8);
  assert.ok(edgePaths(layout).every(e => !/NaN|Infinity/.test(e.attributes.d)));
  assert.ok(labelLayer(layout).children.every(e => !/NaN|Infinity/.test(Object.values(e.attributes).join(' '))));
  layout.ids.fit.fire('click');
  assert.ok(layout.ids.graph.attributes.viewBox.split(' ').map(Number).every(Number.isFinite));
  assert.deepEqual(labelLayer(layout).children.map(e => e.textContent), ['A → B · edge · 1:n']);
  assertLabelGeometry(layout);
  const prior = labelLayer(layout).children.map(e => ({...e.attributes}));
  a.fire('keydown', {key: 'ArrowRight', altKey: true});
  a.fire('keydown', {key: 'ArrowLeft', altKey: true});
  layout.flush();
  assert.deepEqual(labelLayer(layout).children.map(e => e.attributes), prior);
});
