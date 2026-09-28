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
  const heights = new Map(entities.map(entity => [entity.name, 70 + list(entity.relationships).filter(r => r.entity === entity.name).length * 20]));
  const nodeHeight = entity => heights.get(entity.name);
  const positions = new Map();
  let view;
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
    return element;
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
  const padding = 4;
  const expand = box => ({x: box.x - padding, y: box.y - padding, width: box.width + padding * 2, height: box.height + padding * 2});
  const intersects = (a, b) => a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;
  // Clip the complete segment against both rectangle slabs, including boundary contact.
  const segmentHits = (a, b, box) => {
    let enter = 0, exit = 1;
    for (const [axis, size] of [['x', 'width'], ['y', 'height']]) {
      const delta = b[axis] - a[axis];
      if (!delta) {
        if (a[axis] < box[axis] || a[axis] > box[axis] + box[size]) return false;
      } else {
        const near = (box[axis] - a[axis]) / delta;
        const far = (box[axis] + box[size] - a[axis]) / delta;
        enter = Math.max(enter, Math.min(near, far));
        exit = Math.min(exit, Math.max(near, far));
        if (enter > exit) return false;
      }
    }
    return true;
  };
  const labelMeasurements = new Map();
  const measureLabel = label => {
    if (!labelMeasurements.has(label.textContent)) {
      const {x, y, width, height} = label.getBBox();
      labelMeasurements.set(label.textContent, {x, y, width, height});
    }
    return labelMeasurements.get(label.textContent);
  };
  const samples = [0.5];
  for (let step = 1; step <= 9; step++) samples.push((10 - step) / 20, (10 + step) / 20);
  const pairs = new Map();
  const connections = [];
  entities.forEach(entity => {
    const add = (target, label, kind) => {
      if (!byName.has(target) || target === entity.name) return;
      const pair = JSON.stringify([entity.name, target].sort());
      const index = pairs.get(pair) || 0;
      pairs.set(pair, index + 1);
      connections.push({source: entity.name, target, label, kind, index});
    };
    list(entity.relationships).forEach(r => add(r.entity, `${r.role || '(no role)'} · ${r.cardinality}`, r.ownership === 'owned' ? 'owned' : 'referenced'));
    if (entity.subtypeOf) add(entity.subtypeOf, 'is-a', 'isa');
  });
  const boundary = (center, dx, dy) => {
    const scale = 1 / Math.max(Math.abs(dx) / 125, Math.abs(dy) / (center.h / 2));
    return {x: center.x + dx * scale, y: center.y + dy * scale};
  };
  const renderedNodes = new Map();
  let suppressClick = false;
  const arrowSteps = {ArrowLeft: [-40, 0], ArrowRight: [40, 0], ArrowUp: [0, -40], ArrowDown: [0, 40]};
  entities.forEach((entity, i) => {
    const p = {x: 40, y: 55};
    positions.set(entity.name, p);
    const group = shape('g', nodes, {class: `node${entity.external ? ' external' : ''}${entity.derived ? ' derived' : ''}`, role: 'button', tabindex: 0, 'aria-labelledby': `node-title-${i}`});
    nodeEntities.set(group, entity);
    const title = shape('title', group, {id: `node-title-${i}`});
    title.textContent = `${entity.name}${entity.external ? ', external' : ''}${entity.derived ? ', derived' : ''}`;
    const shapes = [shape('rect', group, {x: p.x, y: p.y, width: 250, height: nodeHeight(entity), rx: 8}),
      textShape(group, entity.name, p.x + 125, p.y + 32, 'node-label'),
      textShape(group, entity.external ? 'external' : entity.derived ? 'derived' : 'entity', p.x + 125, p.y + 54, 'edge-label')];
    list(entity.relationships).filter(r => r.entity === entity.name).forEach((r, index) =>
      shapes.push(textShape(group, `${r.role || '(no role)'} · ${r.cardinality}`, p.x + 125, p.y + 76 + index * 20, 'edge-label')));
    renderedNodes.set(entity.name, {shapes,
      left: Math.min(...shapes.map(s => s.getBBox().x)) - p.x,
      right: Math.max(...shapes.map(s => s.getBBox().x + s.getBBox().width)) - p.x});
    group.addEventListener('click', event => {
      if (suppressClick) { event.preventDefault(); suppressClick = false; return; }
      select(entity.name);
    });
    group.addEventListener('keydown', event => {
      if (event.altKey && arrowSteps[event.key]) {
        event.preventDefault(); event.stopPropagation();
        moveNode(entity.name, ...arrowSteps[event.key]);
      } else if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); select(entity.name); }
    });
  });
  const moveNode = (name, dx, dy) => {
    const p = positions.get(name);
    p.x += dx; p.y += dy;
    renderedNodes.get(name).shapes.forEach(s => {
      s.setAttribute('x', Number(s.getAttribute('x')) + dx);
      s.setAttribute('y', Number(s.getAttribute('y')) + dy);
    });
    scheduleDraw();
  };
  const arrange = mode => {
    const layers = [];
    if (mode === 'grid') {
      const columns = Math.max(1, Math.min(3, Math.ceil(Math.sqrt(entities.length))));
      for (let i = 0; i < entities.length; i += columns) layers.push(entities.slice(i, i + columns).map(e => e.name));
    } else {
      const incoming = new Map(entities.map(e => [e.name, 0]));
      const outgoing = new Map(entities.map(e => [e.name, []]));
      connections.forEach(({source, target}) => { incoming.set(target, incoming.get(target) + 1); outgoing.get(source).push(target); });
      const visited = new Set();
      const traverse = root => {
        const queue = [[root, 0]];
        visited.add(root);
        for (let i = 0; i < queue.length; i++) {
          const [name, depth] = queue[i];
          (layers[depth] ||= []).push(name);
          for (const target of outgoing.get(name)) if (!visited.has(target)) {
            visited.add(target); queue.push([target, depth + 1]);
          }
        }
      };
      entities.forEach(e => { if (!incoming.get(e.name) && !visited.has(e.name)) traverse(e.name); });
      entities.forEach(e => { if (!visited.has(e.name)) traverse(e.name); });
    }
    let axis = mode === 'flow-right' ? 40 : 55;
    layers.forEach(names => {
      let cross = mode === 'flow-right' ? 55 : 40;
      for (const name of names) {
        const node = renderedNodes.get(name);
        const p = positions.get(name);
        const x = mode === 'flow-right' ? axis - node.left : cross - node.left;
        const y = mode === 'flow-right' ? cross : axis;
        const dx = x - p.x, dy = y - p.y;
        p.x = x; p.y = y;
        node.shapes.forEach(s => {
          s.setAttribute('x', Number(s.getAttribute('x')) + dx);
          s.setAttribute('y', Number(s.getAttribute('y')) + dy);
        });
        cross += mode === 'flow-right' ? nodeHeight(byName.get(name)) + 110
          : mode === 'grid' ? Math.max(460, node.right - node.left + 110) : node.right - node.left + 110;
      }
      axis += mode === 'flow-right'
        ? Math.max(...names.map(name => renderedNodes.get(name).right - renderedNodes.get(name).left), 0) + 150
        : Math.max(...names.map(name => nodeHeight(byName.get(name))), 0) + 150;
    });
    if (frame) { cancelAnimationFrame(frame); frame = 0; }
    draw();
    fit();
  };
  let reservations = [];
  const draw = () => {
    edges.replaceChildren();
    labels.replaceChildren();
    const pendingLabels = [];
    const edgePoints = [];
    connections.forEach(({source, target, label, kind, index}) => {
      const from = positions.get(source), to = positions.get(target);
      const a = {x: from.x + 125, y: from.y + nodeHeight(byName.get(source)) / 2, h: nodeHeight(byName.get(source))};
      const b = {x: to.x + 125, y: to.y + nodeHeight(byName.get(target)) / 2, h: nodeHeight(byName.get(target))};
      const dx = b.x - a.x, dy = b.y - a.y;
      const length = Math.hypot(dx, dy);
      const px = length ? -dy / length : 0, py = length ? dx / length : 1;
      const offset = (index % 2 ? 1 : -1) * Math.ceil(index / 2) * 52 * (source < target ? 1 : -1);
      const c = {x: (a.x + b.x) / 2 + px * offset, y: (a.y + b.y) / 2 + py * offset};
      const start = boundary(a, c.x - a.x || 1, c.y - a.y);
      const end = boundary(b, c.x - b.x || -1, c.y - b.y);
      edgePoints.push(start, c, end);
      shape('path', edges, {d: `M ${start.x} ${start.y} Q ${c.x} ${c.y} ${end.x} ${end.y}`,
        class: `edge ${kind}`, 'marker-end': 'url(#arrow)'});
      pendingLabels.push({source, target, label, start, c, end});
    });
    const reserved = [...renderedNodes.values()].flatMap(node => node.shapes.map(s => expand(s.getBBox())));
    const leaders = [], anchors = [], overflow = [];
    const placeLabel = ({source, target, label: value, start, c, end}) => {
      const label = textShape(labels, value, 0, 0, 'edge-label');
      const measured = measureLabel(label);
      const visible = samples.map(t => {
        const anchor = {x: (1-t)**2*start.x + 2*(1-t)*t*c.x + t*t*end.x,
          y: (1-t)**2*start.y + 2*(1-t)*t*c.y + t*t*end.y};
        const dx = 2*((1-t)*(c.x-start.x) + t*(end.x-c.x));
        const dy = 2*((1-t)*(c.y-start.y) + t*(end.y-c.y));
        const length = Math.hypot(dx, dy);
        return {anchor, tangent: {x: dx / length, y: dy / length}, length};
      }).filter(({anchor, length}) => length && !reserved.some(box => segmentHits(anchor, anchor, box)));
      let chosen;
      // Try every visible sample at a short offset before considering a longer stem.
      search: for (const gap of [0, 8, 20, 36]) for (const {anchor, tangent} of visible) {
        const normal = {x: -tangent.y, y: tangent.x};
        for (const [across, along] of gap ? [[1, 0], [-1, 0], [1, 1], [1, -1], [-1, 1], [-1, -1]] : [[0, 0]]) {
          const length = Math.hypot(across, along) || 1;
          const direction = {x: (normal.x*across + tangent.x*along) / length,
            y: (normal.y*across + tangent.y*along) / length};
          const radius = gap ? 1 / Math.max(Math.abs(direction.x) / (measured.width / 2), Math.abs(direction.y) / (measured.height / 2)) : 0;
          const center = {x: anchor.x + direction.x*(radius + gap), y: anchor.y + direction.y*(radius + gap)};
          const box = expand({x: center.x - measured.width / 2, y: center.y - measured.height / 2, width: measured.width, height: measured.height});
          if (reserved.some(other => intersects(box, other)) || anchors.some(p => segmentHits(p, p, box)) ||
              leaders.some(leader => segmentHits(leader.a, leader.b, box))) continue;
          const border = {x: anchor.x + direction.x*gap, y: anchor.y + direction.y*gap};
          if (gap && reserved.some(other => segmentHits(anchor, border, other))) continue;
          chosen = {center, box, anchor, border, gap};
          break search;
        }
      }
      if (!chosen) {
        overflow.push({label, value: `${source} → ${target} · ${value}`});
        return;
      }
      label.setAttribute('x', chosen.center.x - measured.x - measured.width / 2);
      label.setAttribute('y', chosen.center.y - measured.y - measured.height / 2);
      reserved.push(chosen.box);
      anchors.push(chosen.anchor);
      if (chosen.gap) {
        const {anchor, border} = chosen;
        const leader = shape('path', labels, {d: `M ${anchor.x} ${anchor.y} L ${border.x} ${border.y}`, class: 'edge-label-leader'});
        labels.insertBefore(leader, label);
        leaders.push({a: anchor, b: border});
      }
    };
    pendingLabels.forEach(placeLabel);
    // Reserve overflow below the entire graph, only after all local labels have been placed.
    const left = reserved.reduce((min, box) => Math.min(min, box.x), Infinity);
    let bottom = reserved.reduce((max, box) => Math.max(max, box.y + box.height),
      edgePoints.reduce((max, p) => Math.max(max, p.y), -Infinity)) + 28;
    overflow.forEach(({label, value}) => {
      label.textContent = value;
      const measured = measureLabel(label);
      label.setAttribute('x', left - measured.x);
      label.setAttribute('y', bottom - measured.y);
      const box = expand({x: left, y: bottom, width: measured.width, height: measured.height});
      reserved.push(box);
      bottom = box.y + box.height + 20;
    });
    reservations = reserved.concat(edgePoints.map(p => ({x: p.x, y: p.y, width: 0, height: 0})));
  };
  let frame = 0;
  const scheduleDraw = () => {
    if (!frame) frame = requestAnimationFrame(() => { frame = 0; draw(); });
  };
  const fit = () => {
    if (frame) { cancelAnimationFrame(frame); frame = 0; draw(); }
    const minX = reservations.reduce((min, box) => Math.min(min, box.x), Infinity);
    const minY = reservations.reduce((min, box) => Math.min(min, box.y), Infinity);
    const maxX = reservations.reduce((max, box) => Math.max(max, box.x + box.width), -Infinity);
    const maxY = reservations.reduce((max, box) => Math.max(max, box.y + box.height), -Infinity);
    view = reservations.length
      ? {x: minX - 10, y: minY - 10, w: Math.max(500, maxX - minX + 20), h: Math.max(280, maxY - minY + 20)}
      : {x: 0, y: 0, w: 500, h: 280};
    applyView();
  };
  const layout = document.getElementById('layout');
  document.getElementById('arrange').addEventListener('click', () => arrange(layout.value));
  arrange('grid');
  const search = document.getElementById('search');
  search.addEventListener('input', () => {
    const query = search.value.toLocaleLowerCase();
    nodes.querySelectorAll('.node').forEach(node => node.classList.toggle('dim', !nodeEntities.get(node).name.toLocaleLowerCase().includes(query)));
  });
  document.getElementById('zoom-in').addEventListener('click', () => zoom(0.8));
  document.getElementById('zoom-out').addEventListener('click', () => zoom(1.25));
  document.getElementById('fit').addEventListener('click', fit);
  svg.addEventListener('keydown', event => {
    if (event.altKey && arrowSteps[event.key] && event.target.closest('.node')) return;
    if (arrowSteps[event.key]) {
      event.preventDefault();
      view.x += arrowSteps[event.key][0]; view.y += arrowSteps[event.key][1]; applyView();
    } else if (event.key === '+' || event.key === '=') { event.preventDefault(); zoom(0.8); }
    else if (event.key === '-') { event.preventDefault(); zoom(1.25); }
    else if (event.key === '0') { event.preventDefault(); fit(); }
  });
  const point = event => {
    const p = svg.createSVGPoint();
    p.x = event.clientX; p.y = event.clientY;
    return p.matrixTransform(svg.getScreenCTM().inverse());
  };
  let drag = null;
  svg.addEventListener('pointerdown', event => {
    if (drag || event.button !== 0 || event.isPrimary === false) return;
    suppressClick = false;
    const node = event.target.closest('.node');
    drag = {id: event.pointerId, name: node ? nodeEntities.get(node).name : null,
      capture: node || svg, x: event.clientX, y: event.clientY, moved: false};
    drag.capture.setPointerCapture(event.pointerId);
  });
  svg.addEventListener('pointermove', event => {
    if (!drag || event.pointerId !== drag.id) return;
    if (drag.name && !drag.moved && Math.hypot(event.clientX - drag.x, event.clientY - drag.y) < 5) return;
    const now = point(event);
    const before = point({clientX: drag.x, clientY: drag.y});
    if (drag.name) {
      drag.moved = true;
      moveNode(drag.name, now.x - before.x, now.y - before.y);
    } else {
      view.x -= now.x - before.x;
      view.y -= now.y - before.y;
      applyView();
    }
    drag.x = event.clientX; drag.y = event.clientY;
  });
  const endDrag = event => {
    if (!drag || event.pointerId !== drag.id) return;
    suppressClick = event.type === 'pointerup' && drag.name && drag.moved;
    const capture = drag.capture;
    drag = null;
    if (capture.hasPointerCapture(event.pointerId)) capture.releasePointerCapture(event.pointerId);
  };
  svg.addEventListener('pointerup', endDrag);
  svg.addEventListener('pointercancel', endDrag);
  svg.addEventListener('lostpointercapture', endDrag);
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
