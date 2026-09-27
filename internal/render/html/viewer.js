(() => {
  'use strict';
  const model = JSON.parse(document.getElementById('model-data').textContent);
  const svg = document.getElementById('graph');
  const layer = document.getElementById('viewport');
  const details = document.getElementById('details');
  const globals = document.getElementById('globals');
  const NS = 'http://www.w3.org/2000/svg';
  const entities = model.entities || [];
  const list = items => items || [];
  const byName = new Map(entities.map(entity => [entity.name, entity]));
  const columns = Math.max(1, Math.min(3, Math.ceil(Math.sqrt(entities.length))));
  const nodeHeight = entity => 70 + list(entity.relationships).filter(r => r.entity === entity.name).length * 20;
  const rows = Math.ceil(entities.length / columns);
  const rowY = [];
  let nextY = 55;
  for (let row = 0; row < rows; row++) {
    rowY.push(nextY);
    nextY += Math.max(...entities.slice(row * columns, (row + 1) * columns).map(nodeHeight)) + 150;
  }
  const positions = new Map(entities.map((entity, i) => [entity.name, {
    x: 40 + (i % columns) * 460, y: rowY[Math.floor(i / columns)]
  }]));
  const initial = {x: 0, y: 0, w: Math.max(380, columns * 460 + 40),
    h: Math.max(280, nextY - 90)};
  let view = {...initial};
  const html = (tag, parent, text, className) => {
    const element = document.createElement(tag);
    if (text !== undefined) element.textContent = text;
    if (className) element.className = className;
    parent.appendChild(element);
    return element;
  };
  const shape = (tag, parent, attributes) => {
    const element = document.createElementNS(NS, tag);
    for (const [key, value] of Object.entries(attributes)) element.setAttribute(key, String(value));
    parent.appendChild(element);
    return element;
  };
  const textShape = (parent, value, x, y, className) => {
    const element = shape('text', parent, {x, y, class: className, 'text-anchor': 'middle'});
    element.textContent = value;
  };
  const line = (parent, value) => html('div', parent, value, 'item');
  const heading = (parent, value) => html('h3', parent, value);
  const values = (parent, title, items, describe) => {
    heading(parent, title);
    if (!list(items).length) line(parent, 'None');
    list(items).forEach(item => line(parent, describe(item)));
  };
  const relText = (from, rel) => `${from} → ${rel.entity} · ${rel.role || '(no role)'} · ${rel.cardinality} · ${rel.ownership || 'referenced'}${rel.symmetric ? ' · symmetric' : ''}${rel.note ? ' · ' + rel.note : ''}`;
  const applyView = () => svg.setAttribute('viewBox', `${view.x} ${view.y} ${view.w} ${view.h}`);
  const zoom = factor => {
    view.x += view.w * (1 - factor) / 2;
    view.y += view.h * (1 - factor) / 2;
    view.w *= factor;
    view.h *= factor;
    applyView();
  };
  const nodeEntities = new Map();
  const select = name => {
    const entity = byName.get(name);
    details.replaceChildren();
    html('h2', details, entity.name + (entity.external ? ' (external)' : ''));
    if (entity.external) {
      line(details, 'Defined by an imported model; details are not included in this file.');
    } else {
      line(details, entity.definition || 'No description');
      if (entity.derived) line(details, `Derived entity${entity.derivation ? ': ' + entity.derivation : ''}`);
      if (entity.subtypeOf) line(details, `Is a ${entity.subtypeOf}`);
      values(details, 'Attributes', entity.attributes, a => `${a.name}: ${a.type} · required: ${a.required === null ? 'not specified' : a.required}${a.derived ? ' · derived: ' + a.derivation : ''}${a.description ? ' · ' + a.description : ''}`);
      values(details, 'Outgoing relationships', entity.relationships, r => relText(entity.name, r));
    }
    values(details, 'Incoming relationships', entities.flatMap(other => list(other.relationships).filter(r => r.entity === entity.name).map(r => relText(other.name, r))), r => r);
    if (!entity.external) {
      values(details, 'Actions', entity.actions, a => `${a.name}${a.actor ? ' · actor: ' + a.actor : ''}${a.preserves?.length ? ' · preserves: ' + a.preserves.join(', ') : ''}${a.description ? ' · ' + a.description : ''}`);
      values(details, 'Invariants', entity.invariants, i => `${i.id}: ${i.statement}`);
    }
    layer.querySelectorAll('.node').forEach(node => node.classList.toggle('selected', nodeEntities.get(node) === entity));
  };
  document.getElementById('title').textContent = model.title || 'Domain model';
  document.getElementById('description').textContent = model.description;
  const defs = shape('defs', svg, {});
  const marker = shape('marker', defs, {id: 'arrow', viewBox: '0 0 10 10', refX: 9, refY: 5,
    markerWidth: 10, markerHeight: 10, markerUnits: 'userSpaceOnUse', orient: 'auto'});
  shape('path', marker, {d: 'M 0 0 L 10 5 L 0 10 z', class: 'arrow'});
  const edges = shape('g', layer, {'aria-hidden': 'true'});
  const nodes = shape('g', layer, {});
  const labels = shape('g', layer, {'aria-hidden': 'true'});
  const pairs = new Map();
  const boundary = (center, dx, dy) => {
    const scale = 1 / Math.max(Math.abs(dx) / 125, Math.abs(dy) / (center.h / 2));
    return {x: center.x + dx * scale, y: center.y + dy * scale};
  };
  entities.forEach(entity => {
    const from = positions.get(entity.name);
    const drawEdge = (target, label, kind) => {
      const to = positions.get(target);
      if (!to || target === entity.name) return;
      const pair = JSON.stringify([entity.name, target].sort());
      const index = pairs.get(pair) || 0;
      pairs.set(pair, index + 1);
      const a = {x: from.x + 125, y: from.y + nodeHeight(entity) / 2, h: nodeHeight(entity)};
      const b = {x: to.x + 125, y: to.y + nodeHeight(byName.get(target)) / 2, h: nodeHeight(byName.get(target))};
      const dx = b.x - a.x, dy = b.y - a.y;
      const length = Math.hypot(dx, dy);
      const offset = (index % 2 ? 1 : -1) * Math.ceil(index / 2) * 52 * (entity.name < target ? 1 : -1);
      const px = -dy / length, py = dx / length;
      const c = {x: (a.x + b.x) / 2 + px * offset, y: (a.y + b.y) / 2 + py * offset};
      const start = boundary(a, c.x - a.x, c.y - a.y);
      const end = boundary(b, c.x - b.x, c.y - b.y);
      shape('path', edges, {d: `M ${start.x} ${start.y} Q ${c.x} ${c.y} ${end.x} ${end.y}`,
        class: `edge ${kind}`, 'marker-end': 'url(#arrow)'});
      textShape(labels, label, (start.x + 2 * c.x + end.x) / 4,
        (start.y + 2 * c.y + end.y) / 4 - 8, 'edge-label');
    };
    list(entity.relationships).forEach(r => drawEdge(r.entity, `${r.role || '(no role)'} · ${r.cardinality}`, r.ownership === 'owned' ? 'owned' : 'referenced'));
    if (entity.subtypeOf) drawEdge(entity.subtypeOf, 'is-a', 'isa');
  });
  entities.forEach((entity, i) => {
    const p = positions.get(entity.name);
    const group = shape('g', nodes, {class: `node${entity.external ? ' external' : ''}${entity.derived ? ' derived' : ''}`, role: 'button', tabindex: 0, 'aria-labelledby': `node-title-${i}`});
    nodeEntities.set(group, entity);
    const title = shape('title', group, {id: `node-title-${i}`});
    title.textContent = `${entity.name}${entity.external ? ', external' : ''}${entity.derived ? ', derived' : ''}`;
    shape('rect', group, {x: p.x, y: p.y, width: 250, height: nodeHeight(entity), rx: 8});
    textShape(group, entity.name, p.x + 125, p.y + 32, 'node-label');
    textShape(group, entity.external ? 'external' : entity.derived ? 'derived' : 'entity', p.x + 125, p.y + 54, 'edge-label');
    list(entity.relationships).filter(r => r.entity === entity.name).forEach((r, index) =>
      textShape(group, `${r.role || '(no role)'} · ${r.cardinality}`, p.x + 125, p.y + 76 + index * 20, 'edge-label'));
    group.addEventListener('click', () => select(entity.name));
    group.addEventListener('keydown', event => {
      if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); select(entity.name); }
    });
  });
  const search = document.getElementById('search');
  search.addEventListener('input', () => {
    const query = search.value.toLocaleLowerCase();
    nodes.querySelectorAll('.node').forEach(node => node.classList.toggle('dim', !nodeEntities.get(node).name.toLocaleLowerCase().includes(query)));
  });
  document.getElementById('zoom-in').addEventListener('click', () => zoom(0.8));
  document.getElementById('zoom-out').addEventListener('click', () => zoom(1.25));
  document.getElementById('reset').addEventListener('click', () => { view = {...initial}; applyView(); });
  svg.addEventListener('keydown', event => {
    const steps = {ArrowLeft: [-40, 0], ArrowRight: [40, 0], ArrowUp: [0, -40], ArrowDown: [0, 40]};
    if (steps[event.key]) {
      event.preventDefault();
      view.x += steps[event.key][0]; view.y += steps[event.key][1]; applyView();
    } else if (event.key === '+' || event.key === '=') { event.preventDefault(); zoom(0.8); }
    else if (event.key === '-') { event.preventDefault(); zoom(1.25); }
    else if (event.key === '0') { event.preventDefault(); view = {...initial}; applyView(); }
  });
  let drag = null;
  svg.addEventListener('pointerdown', event => {
    if (event.target.closest('.node')) return;
    drag = {x: event.clientX, y: event.clientY};
    svg.setPointerCapture(event.pointerId);
  });
  svg.addEventListener('pointermove', event => {
    if (!drag) return;
    const box = svg.getBoundingClientRect();
    view.x -= (event.clientX - drag.x) * view.w / box.width;
    view.y -= (event.clientY - drag.y) * view.h / box.height;
    drag = {x: event.clientX, y: event.clientY};
    applyView();
  });
  svg.addEventListener('pointerup', () => { drag = null; });
  svg.addEventListener('lostpointercapture', () => { drag = null; });
  const section = title => { const s = html('section', globals); html('h2', s, title); return s; };
  const invariants = section('Model invariants');
  values(invariants, 'Rules', model.invariants, i => `${i.id}: ${i.statement}`);
  const enums = section('Enums');
  values(enums, 'Types', model.enums, e => `${e.name}${e.description ? ' · ' + e.description : ''}: ${e.values.map(v => v.name + (v.definition ? ' (' + v.definition + ')' : '')).join(', ')}`);
  const glossary = section('Glossary');
  values(glossary, 'Terms', model.glossary, t => `${t.name}: ${t.definition}`);
  const scenarios = section('Scenarios');
  values(scenarios, 'Narratives', model.scenarios, s => `${s.name}${s.description ? ' · ' + s.description : ''}\nActors: ${s.actors?.join(', ') || 'none'}\nSteps: ${s.steps?.join(' → ') || 'none'}\nInvariants touched: ${s.invariants_touched?.join(', ') || 'none'}`);
  const imports = section('Imports');
  values(imports, 'Sources', model.imports, i => `${i.scope}: ${i.path}`);
  applyView();
})();
