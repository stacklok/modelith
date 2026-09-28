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
  const theme = document.getElementById('theme');
  theme.addEventListener('change', () => { document.documentElement.dataset.theme = theme.value; });
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
  const line = (parent, value) => html('p', parent, value, 'item');
  const heading = (parent, value) => html('h3', parent, value);
  const values = (parent, title, items, describe) => {
    heading(parent, title);
    if (!list(items).length) line(parent, 'None');
    else {
      const ul = html('ul', parent);
      list(items).forEach(item => html('li', ul, describe(item)));
    }
  };
  const fields = (parent, entries) => {
    const dl = html('dl', parent);
    entries.forEach(([name, value]) => {
      html('dt', dl, name);
      html('dd', dl, String(value));
    });
  };
  const records = (parent, title, items, render) => {
    heading(parent, title);
    if (!list(items).length) { line(parent, 'None'); return; }
    const ul = html('ul', parent, undefined, 'records');
    list(items).forEach(item => render(html('li', ul), item));
  };
  const relText = (from, rel) => `${from} → ${rel.entity} · ${rel.role || '(no role)'} · ${rel.cardinality} · ${rel.ownership || 'referenced'}${rel.symmetric ? ' · symmetric' : ''}${rel.note ? ' · ' + rel.note : ''}`;
  const applyView = () => svg.setAttribute('viewBox', `${view.x} ${view.y} ${view.w} ${view.h}`);
  const motion = window.matchMedia?.('(prefers-reduced-motion: reduce)');
  let cameraFrame = 0, fade, cameraTarget;
  const cancelMotion = (finish = false) => {
    if (cameraFrame) { cancelAnimationFrame(cameraFrame); cameraFrame = 0; }
    if (finish && cameraTarget) { view = cameraTarget; applyView(); }
    cameraTarget = null;
    fade?.cancel(); fade = null;
  };
  motion?.addEventListener?.('change', () => { if (motion.matches) cancelMotion(true); });
  const zoom = factor => {
    cancelMotion();
    view.x += view.w * (1 - factor) / 2;
    view.y += view.h * (1 - factor) / 2;
    view.w *= factor;
    view.h *= factor;
    applyView();
  };
  const nodeEntities = new Map();
  let selected = null, hovered = null, pointerEdge = null, focusedEdge = null;
  const renderedEdges = new Map();
  const syncHover = () => { hovered = focusedEdge ?? pointerEdge; visualState(); };
  const visualState = () => {
    const active = hovered === null ? connections.filter(c => c.source === selected || c.target === selected) : connections.filter(c => c.id === hovered);
    const neighbors = new Set(hovered === null ? [selected] : []);
    active.forEach(c => { neighbors.add(c.source); neighbors.add(c.target); });
    const query = document.getElementById('search').value.toLocaleLowerCase();
    const matches = name => name.toLocaleLowerCase().includes(query);
    const focus = hovered !== null || selected !== null;
    nodeEntities.forEach((entity, node) => {
      node.classList.toggle('selected', entity.name === selected);
      node.classList.toggle('highlight', focus && neighbors.has(entity.name));
      node.classList.toggle('dim', !matches(entity.name) || focus && !neighbors.has(entity.name));
    });
    connections.forEach(c => {
      const current = renderedEdges.get(c.id);
      if (!current) return;
      const hit = active.includes(c);
      current.forEach(element => {
        element.classList.toggle('highlight', focus && hit);
        element.classList.toggle('dim', !matches(c.source) && !matches(c.target) || focus && !hit);
      });
    });
  };
  const select = name => {
    selected = name;
    const entity = byName.get(name);
    details.replaceChildren();
    html('h2', details, entity.name + (entity.external ? ' (external)' : ''));
    if (entity.external) {
      line(details, 'Defined by an imported model; details are not included in this file.');
    } else {
      line(details, entity.definition || 'No description');
      if (entity.derived) line(details, `Derived entity${entity.derivation ? ': ' + entity.derivation : ''}`);
      if (entity.subtypeOf) line(details, `Is a ${entity.subtypeOf}`);
      records(details, 'Attributes', entity.attributes, (row, a) => fields(row, [
        ['Name', a.name], ['Type', a.type], ['Required', a.required === null || a.required === undefined ? 'not specified' : a.required ? 'yes' : 'no'],
        ['Description', a.description || 'None'], ['Derived', a.derived ? 'yes' : 'no'],
        ...(a.derived ? [['Derivation', a.derivation || 'None']] : [])]));
      values(details, 'Outgoing relationships', entity.relationships, r => relText(entity.name, r));
    }
    values(details, 'Incoming relationships', entities.flatMap(other => list(other.relationships).filter(r => r.entity === entity.name).map(r => relText(other.name, r))), r => r);
    if (!entity.external) {
      records(details, 'Actions', entity.actions, (row, a) => fields(row, [
        ['Name', a.name], ['Actor', a.actor || 'Not specified'], ['Preserves', list(a.preserves).join(', ') || 'None'],
        ['Description', a.description || 'None']]));
      records(details, 'Invariants', entity.invariants, (row, i) => fields(row, [['ID', i.id], ['Statement', i.statement]]));
    }
    visualState();
  };
  document.getElementById('title').textContent = model.title || 'Domain model';
  document.getElementById('description').textContent = model.description;
  const defs = shape('defs', svg, {});
  const marker = shape('marker', defs, {id: 'arrow', viewBox: '0 0 10 10', refX: 9, refY: 5,
    markerWidth: 10, markerHeight: 10, markerUnits: 'userSpaceOnUse', orient: 'auto'});
  shape('path', marker, {d: 'M 0 0 L 10 5 L 0 10 z', class: 'arrow'});
  const edges = shape('g', layer, {});
  const nodes = shape('g', layer, {});
  const labels = shape('g', layer, {});
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
      connections.push({id: connections.length, source: entity.name, target, label, kind, index});
    };
    list(entity.relationships).forEach(r => add(r.entity, `${r.role || '(no role)'} · ${r.cardinality}`, r.ownership === 'owned' ? 'owned' : 'referenced'));
    if (entity.subtypeOf) add(entity.subtypeOf, 'is-a', 'isa');
  });
  const boundary = (center, dx, dy) => {
    const scale = 1 / Math.max(Math.abs(dx) / 125, Math.abs(dy) / (center.h / 2));
    return {x: center.x + dx * scale, y: center.y + dy * scale};
  };
  const renderedNodes = new Map();
  const edgeTarget = (element, id) => {
    element.addEventListener('pointerenter', () => { pointerEdge = id; syncHover(); });
    element.addEventListener('pointerleave', () => { if (pointerEdge === id) { pointerEdge = null; syncHover(); } });
  };
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
      textShape(group, entity.external ? 'external' : entity.derived ? 'derived' : 'entity', p.x + 125, p.y + 54, 'node-badge')];
    list(entity.relationships).filter(r => r.entity === entity.name).forEach((r, index) =>
      shapes.push(textShape(group, `${r.role || '(no role)'} · ${r.cardinality}`, p.x + 125, p.y + 76 + index * 20, 'node-secondary')));
    const badge = shapes[2], badgeBox = badge.getBBox();
    const badgeBack = shape('rect', group, {x: badgeBox.x - 5, y: badgeBox.y - 2,
      width: badgeBox.width + 10, height: badgeBox.height + 4, rx: 6, class: 'badge-bg'});
    group.insertBefore(badgeBack, badge);
    shapes.push(badgeBack);
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
    cancelMotion();
    const p = positions.get(name);
    p.x += dx; p.y += dy;
    renderedNodes.get(name).shapes.forEach(s => {
      s.setAttribute('x', Number(s.getAttribute('x')) + dx);
      s.setAttribute('y', Number(s.getAttribute('y')) + dy);
    });
    scheduleDraw();
  };
  const arrange = mode => {
    cancelMotion();
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
    if (motion && !motion.matches && layer.animate) fade = layer.animate([{opacity: .72}, {opacity: 1}], {duration: 180, easing: 'ease-out'});
  };
  let reservations = [];
  const draw = () => {
    const restoreFocus = focusedEdge;
    edges.replaceChildren();
    labels.replaceChildren();
    renderedEdges.clear();
    const pendingLabels = [];
    const edgePoints = [];
    connections.forEach(({id, source, target, label, kind, index}) => {
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
      const d = `M ${start.x} ${start.y} Q ${c.x} ${c.y} ${end.x} ${end.y}`;
      const path = shape('path', edges, {d, class: `edge ${kind}`, 'marker-end': 'url(#arrow)', 'pointer-events': 'none'});
      const hit = shape('path', edges, {d, class: 'edge-hit', 'data-edge-id': id, 'aria-hidden': 'true'});
      edgeTarget(hit, id);
      renderedEdges.set(id, [path, hit]);
      pendingLabels.push({id, source, target, label, start, c, end});
    });
    const reserved = [...renderedNodes.values()].flatMap(node => node.shapes.map(s => expand(s.getBBox())));
    const leaders = [], anchors = [], overflow = [];
    const background = (label, x, y, width, height, id, description) => {
      const title = shape('title', labels, {id: `edge-title-${id}`});
      title.textContent = description;
      const rect = shape('rect', labels, {x, y, width, height, rx: 4, class: 'label-bg', role: 'img', tabindex: 0,
        'data-edge-id': id, 'aria-labelledby': `edge-title-${id}`});
      labels.insertBefore(rect, label);
      edgeTarget(rect, id);
      rect.addEventListener('focus', () => { focusedEdge = id; syncHover(); });
      rect.addEventListener('blur', () => { if (focusedEdge === id) { focusedEdge = null; syncHover(); } });
      renderedEdges.get(id).push(rect, label);
      return rect;
    };
    const placeLabel = ({id, source, target, label: value, start, c, end}) => {
      const label = textShape(labels, value, 0, 0, 'edge-label');
      const measured = measureLabel(label);
      const width = measured.width + 10, height = measured.height + 6;
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
          const radius = gap ? 1 / Math.max(Math.abs(direction.x) / (width / 2), Math.abs(direction.y) / (height / 2)) : 0;
          const center = {x: anchor.x + direction.x*(radius + gap), y: anchor.y + direction.y*(radius + gap)};
          const box = expand({x: center.x - width / 2, y: center.y - height / 2, width, height});
          if (reserved.some(other => intersects(box, other)) || anchors.some(p => segmentHits(p, p, box)) ||
              leaders.some(leader => segmentHits(leader.a, leader.b, box))) continue;
          const border = {x: anchor.x + direction.x*gap, y: anchor.y + direction.y*gap};
          if (gap && reserved.some(other => segmentHits(anchor, border, other))) continue;
          chosen = {center, box, anchor, border, gap};
          break search;
        }
      }
      if (!chosen) {
        overflow.push({id, source, target, label, value: `${source} → ${target} · ${value}`,
          description: `${source} to ${target}: ${value}`});
        return;
      }
      label.setAttribute('x', chosen.center.x - measured.x - measured.width / 2);
      label.setAttribute('y', chosen.center.y - measured.y - measured.height / 2);
      const bg = background(label, chosen.center.x - width / 2, chosen.center.y - height / 2, width, height, id, `${source} to ${target}: ${value}`);
      reserved.push(chosen.box);
      anchors.push(chosen.anchor);
      if (chosen.gap) {
        const {anchor, border} = chosen;
        const leader = shape('path', labels, {d: `M ${anchor.x} ${anchor.y} L ${border.x} ${border.y}`, class: 'edge-label-leader'});
        labels.insertBefore(leader, bg);
        renderedEdges.get(id).push(leader);
        leaders.push({a: anchor, b: border});
      }
    };
    pendingLabels.forEach(placeLabel);
    // Reserve overflow below the entire graph, only after all local labels have been placed.
    const left = reserved.reduce((min, box) => Math.min(min, box.x), Infinity);
    let bottom = reserved.reduce((max, box) => Math.max(max, box.y + box.height),
      edgePoints.reduce((max, p) => Math.max(max, p.y), -Infinity)) + 28;
    overflow.forEach(({id, label, value, description}) => {
      label.textContent = value;
      const measured = measureLabel(label);
      label.setAttribute('x', left + 5 - measured.x);
      label.setAttribute('y', bottom + 3 - measured.y);
      const width = measured.width + 10, height = measured.height + 6;
      background(label, left, bottom, width, height, id, description);
      const box = expand({x: left, y: bottom, width, height});
      reserved.push(box);
      bottom = box.y + box.height + 20;
    });
    reservations = reserved.concat(edgePoints.map(p => ({x: p.x, y: p.y, width: 0, height: 0})));
    visualState();
    if (restoreFocus !== null) renderedEdges.get(restoreFocus)?.[2].focus({preventScroll: true});
  };
  let frame = 0;
  const scheduleDraw = () => {
    if (!frame) frame = requestAnimationFrame(() => { frame = 0; draw(); });
  };
  const fit = () => {
    if (frame) { cancelAnimationFrame(frame); frame = 0; draw(); }
    cancelMotion();
    const minX = reservations.reduce((min, box) => Math.min(min, box.x), Infinity);
    const minY = reservations.reduce((min, box) => Math.min(min, box.y), Infinity);
    const maxX = reservations.reduce((max, box) => Math.max(max, box.x + box.width), -Infinity);
    const maxY = reservations.reduce((max, box) => Math.max(max, box.y + box.height), -Infinity);
    const target = reservations.length
      ? {x: minX - 10, y: minY - 10, w: Math.max(500, maxX - minX + 20), h: Math.max(280, maxY - minY + 20)}
      : {x: 0, y: 0, w: 500, h: 280};
    if (!view || !motion || motion.matches) { view = target; applyView(); return; }
    const start = {...view};
    const begun = performance.now();
    cameraTarget = target;
    const tick = () => {
      const t = Math.min(1, (performance.now() - begun) / 220);
      const ease = 1 - (1 - t) ** 3;
      view = Object.fromEntries(Object.keys(target).map(key => [key, start[key] + (target[key] - start[key]) * ease]));
      applyView();
      if (t < 1) cameraFrame = requestAnimationFrame(tick);
      else { cameraFrame = 0; cameraTarget = null; }
    };
    cameraFrame = requestAnimationFrame(tick);
  };
  const layout = document.getElementById('layout');
  document.getElementById('arrange').addEventListener('click', () => arrange(layout.value));
  arrange('grid');
  const search = document.getElementById('search');
  search.addEventListener('input', visualState);
  document.getElementById('zoom-in').addEventListener('click', () => zoom(0.8));
  document.getElementById('zoom-out').addEventListener('click', () => zoom(1.25));
  document.getElementById('fit').addEventListener('click', fit);
  svg.addEventListener('keydown', event => {
    if (event.altKey && arrowSteps[event.key] && event.target.closest('.node')) return;
    if (arrowSteps[event.key]) {
      event.preventDefault();
      cancelMotion();
      view.x += arrowSteps[event.key][0]; view.y += arrowSteps[event.key][1]; applyView();
    } else if (event.key === '+' || event.key === '=') { event.preventDefault(); zoom(0.8); }
    else if (event.key === '-') { event.preventDefault(); zoom(1.25); }
    else if (event.key === '0') { event.preventDefault(); fit(); }
    else if (event.key === 'Escape') {
      selected = null; pointerEdge = null; focusedEdge = null;
      syncHover();
      svg.focus({preventScroll: true});
    }
  });
  const point = event => {
    const p = svg.createSVGPoint();
    p.x = event.clientX; p.y = event.clientY;
    return p.matrixTransform(svg.getScreenCTM().inverse());
  };
  let drag = null;
  svg.addEventListener('pointerleave', () => { pointerEdge = null; syncHover(); });
  svg.addEventListener('pointerdown', event => {
    if (drag || event.button !== 0 || event.isPrimary === false) return;
    suppressClick = false;
    cancelMotion();
    const relationship = event.target.closest('.edge-hit') || event.target.closest('.label-bg');
    if (relationship) {
      renderedEdges.get(Number(relationship.getAttribute('data-edge-id')))?.[2].focus({preventScroll: true});
      event.preventDefault();
      return;
    }
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
  const globalDefinitions = [
    {name: 'Invariants', items: list(model.invariants), render: (panel, items) => {
      if (!items.length) { line(panel, 'No invariants defined.'); return; }
      items.forEach(invariant => {
        const record = html('article', panel, undefined, 'global-record');
        heading(record, invariant.id);
        line(record, invariant.statement);
      });
    }},
    {name: 'Enums', items: list(model.enums), render: (panel, items) => {
      if (!items.length) { line(panel, 'No enums defined.'); return; }
      items.forEach(enumType => {
        const record = html('article', panel, undefined, 'global-record');
        heading(record, enumType.name);
        line(record, enumType.description || 'No description.');
        if (!list(enumType.values).length) { line(record, 'No values defined.'); return; }
        const definitions = html('dl', record, undefined, 'enum-values');
        enumType.values.forEach(value => {
          html('dt', definitions, value.name);
          html('dd', definitions, value.definition || 'No definition.');
        });
      });
    }},
    {name: 'Glossary', items: list(model.glossary), render: (panel, items) => {
      if (!items.length) { line(panel, 'No glossary terms defined.'); return; }
      const definitions = html('dl', panel, undefined, 'global-definitions');
      items.forEach(term => {
        html('dt', definitions, term.name);
        html('dd', definitions, term.definition);
      });
    }},
    {name: 'Scenarios', items: list(model.scenarios), render: (panel, items) => {
      if (!items.length) { line(panel, 'No scenarios defined.'); return; }
      items.forEach(scenario => {
        const record = html('article', panel, undefined, 'global-record');
        heading(record, scenario.name);
        line(record, scenario.description || 'No description.');
        values(record, 'Actors', scenario.actors, actor => actor);
        html('h4', record, 'Steps');
        if (!list(scenario.steps).length) line(record, 'No steps defined.');
        else {
          const steps = html('ol', record);
          list(scenario.steps).forEach(step => html('li', steps, step));
        }
        values(record, 'Invariants touched', scenario.invariants_touched, invariant => invariant);
      });
    }},
    {name: 'Imports', items: list(model.imports), render: (panel, items) => {
      if (!items.length) { line(panel, 'No imports defined.'); return; }
      const definitions = html('dl', panel, undefined, 'global-definitions');
      items.forEach(item => {
        html('dt', definitions, item.scope);
        html('dd', definitions, item.path);
      });
    }}
  ];
  const tablist = html('div', globals, undefined, 'global-tabs');
  tablist.setAttribute('role', 'tablist');
  tablist.setAttribute('aria-label', 'Model-wide definitions');
  const panels = html('div', globals, undefined, 'global-panels');
  const tabs = [];
  const tabPanels = [];
  const setActiveGlobal = index => {
    tabs.forEach((tab, tabIndex) => {
      const active = tabIndex === index;
      tab.setAttribute('aria-selected', String(active));
      tab.setAttribute('tabindex', active ? '0' : '-1');
      const panel = tabPanels[tabIndex];
      panel.hidden = !active;
      panel.setAttribute('tabindex', active ? '0' : '-1');
    });
  };
  const focusGlobalTab = index => {
    setActiveGlobal(index);
    tabs[index].focus({preventScroll: true});
    tabs[index].scrollIntoView?.({block: 'nearest', inline: 'nearest'});
  };
  globalDefinitions.forEach((definition, index) => {
    const tabID = `globals-tab-${index}`;
    const panelID = `globals-panel-${index}`;
    const tab = html('button', tablist, `${definition.name} (${definition.items.length})`, 'global-tab');
    tab.setAttribute('type', 'button');
    tab.setAttribute('role', 'tab');
    tab.setAttribute('id', tabID);
    tab.setAttribute('aria-controls', panelID);
    const panel = html('section', panels, undefined, 'globals-panel');
    panel.setAttribute('role', 'tabpanel');
    panel.setAttribute('id', panelID);
    panel.setAttribute('aria-labelledby', tabID);
    definition.render(panel, definition.items);
    tab.addEventListener('click', () => setActiveGlobal(index));
    tab.addEventListener('keydown', event => {
      let next = index;
      if (event.key === 'ArrowLeft') next = (index + tabs.length - 1) % tabs.length;
      else if (event.key === 'ArrowRight') next = (index + 1) % tabs.length;
      else if (event.key === 'Home') next = 0;
      else if (event.key === 'End') next = tabs.length - 1;
      else return;
      event.preventDefault();
      focusGlobalTab(next);
    });
    tabs.push(tab);
    tabPanels.push(panel);
  });
  setActiveGlobal(Math.max(0, globalDefinitions.findIndex(definition => definition.items.length)));
  applyView();
})();
