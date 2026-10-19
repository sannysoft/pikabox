'use strict';
'require view';
'require rpc';
'require poll';
'require ui';
'require view.pikabox.theme as theme';

var callStatus = rpc.declare({
	object: 'pikabox',
	method: 'status',
	expect: { '': {} }
});

var callHealthCheck = rpc.declare({
	object: 'pikabox',
	method: 'probe',
	expect: { '': {} }
});

var callStart = rpc.declare({
	object: 'pikabox',
	method: 'start',
	expect: { '': {} }
});

var callSetManagerEnabled = rpc.declare({
	object: 'pikabox',
	method: 'manager_set_enabled',
	params: [ 'enabled' ],
	expect: { '': {} }
});

var callSetMode = rpc.declare({
	object: 'pikabox',
	method: 'manager_set_mode',
	params: [ 'mode' ],
	expect: { '': {} }
});

var callStop = rpc.declare({
	object: 'pikabox',
	method: 'stop',
	expect: { '': {} }
});

var callCleanup = rpc.declare({
	object: 'pikabox',
	method: 'cleanup',
	expect: { '': {} }
});

var callRestart = rpc.declare({
	object: 'pikabox',
	method: 'restart',
	expect: { '': {} }
});

var callReload = rpc.declare({
	object: 'pikabox',
	method: 'reload',
	expect: { '': {} }
});

var callValidate = rpc.declare({
	object: 'pikabox',
	method: 'validate',
	expect: { '': {} }
});

var callLogs = rpc.declare({
	object: 'pikabox',
	method: 'logs',
	params: [ 'lines' ],
	expect: { '': {} }
});

var callHealth = rpc.declare({ object: 'pikabox', method: 'health', expect: { '': {} } });
var callConnections = rpc.declare({ object: 'pikabox', method: 'connections', expect: { '': {} } });
var callCloseConnection = rpc.declare({ object: 'pikabox', method: 'connection_close', params: [ 'id' ], expect: { '': {} } });

var MODE_OPTIONS = [ 'direct', 'rule', 'global' ];
var POLL_INTERVAL = 5;
var LOG_LINES = 300;
var HISTORY = 48;

function valueOrDash(value) {
	if (value === null || value === undefined || value === '')
		return '-';
	return value;
}

function formatBytes(value) {
	value = Number(value || 0);
	if (value < 1024)
		return '%d B'.format(value);
	if (value < 1024 * 1024)
		return '%.1f KiB'.format(value / 1024);
	if (value < 1024 * 1024 * 1024)
		return '%.1f MiB'.format(value / 1024 / 1024);
	return '%.2f GiB'.format(value / 1024 / 1024 / 1024);
}

function formatRate(bytesPerSec) {
	return formatBytes(bytesPerSec) + '/s';
}

function healthClass(value) {
	if (value === 'ok')
		return 'ok';
	if (value && value !== 'unknown')
		return 'error';
	return '';
}

function reportResult(result, successText, failureText) {
	if (result && result.ok)
		theme.notify(result.message || successText);
	else
		theme.error(result, failureText);
}

function startRuntime(data) {
	var enabled = data.manager_enabled ? Promise.resolve({ ok: true }) : callSetManagerEnabled(true);
	return enabled.then(function(result) {
		if (!result.ok)
			return result;
		return callStart();
	});
}

// runAction calls an RPC, reports its outcome and re-renders the live panel
// right away instead of leaving stale state up until the next poll tick.
function runAction(view, call, successText, failureText) {
	return call().then(function(result) {
		reportResult(result, successText, failureText);
	}).then(function() {
		return refreshLive(view, true);
	});
}

function pushHistory(view, data) {
	view.hist = view.hist || [];
	view.hist.push({
		t: Date.now(),
		rx: Number(data.rx_bytes || 0),
		tx: Number(data.tx_bytes || 0)
	});
	if (view.hist.length > HISTORY)
		view.hist.shift();
	return view.hist;
}

// rateSeries turns cumulative byte counters into bytes/second using the real
// time between samples (polls drift, and action-triggered refreshes add extra
// samples). A counter that went backwards means sing-box restarted: count 0.
function rateSeries(hist, key) {
	var out = [];
	for (var i = 1; i < hist.length; i++) {
		var dt = (hist[i].t - hist[i - 1].t) / 1000;
		var delta = hist[i][key] - hist[i - 1][key];
		out.push(dt > 0 && delta > 0 ? delta / dt : 0);
	}
	return out;
}

// sparkline builds an SVG line+area chart via innerHTML so it renders in the
// SVG namespace (LuCI's E() only creates HTML elements). Inputs are numeric
// rates, so string interpolation here is safe.
function sparkline(values, color) {
	var box = E('div', { 'class': 'pikabox-spark' });
	if (!values || values.length < 2) {
		box.appendChild(E('span', { 'class': 'pikabox-spark-empty' }, _('Collecting data…')));
		return box;
	}
	var w = 600, h = 72, pad = 5;
	var max = values.reduce(function(m, v) { return Math.max(m, v); }, 1);
	var step = w / (values.length - 1);
	var pts = values.map(function(v, i) {
		return [ i * step, h - pad - (v / max) * (h - 2 * pad) ];
	});
	var line = pts.map(function(p, i) { return (i ? 'L' : 'M') + p[0].toFixed(1) + ',' + p[1].toFixed(1); }).join(' ');
	var area = 'M0,' + h + ' ' + pts.map(function(p) { return 'L' + p[0].toFixed(1) + ',' + p[1].toFixed(1); }).join(' ') + ' L' + w + ',' + h + ' Z';
	box.innerHTML = '<svg viewBox="0 0 ' + w + ' ' + h + '" preserveAspectRatio="none" style="width:100%;height:100%;display:block">'
		+ '<path d="' + area + '" fill="' + color + '" fill-opacity="0.14"/>'
		+ '<path d="' + line + '" fill="none" stroke="' + color + '" stroke-width="2" vector-effect="non-scaling-stroke"/>'
		+ '</svg>';
	return box;
}

function lastValue(series) {
	return series.length ? series[series.length - 1] : 0;
}

function metric(label, value, accent) {
	return E('div', { 'class': 'pikabox-metric' }, [
		E('div', { 'class': 'pikabox-metric-label' }, label),
		E('div', { 'class': 'pikabox-metric-value' + (accent ? ' ' + accent : '') }, theme.text(value))
	]);
}

function renderHero(view, data) {
	var running = !!data.running;
	var paused = !running && !!data.manager_paused;
	var primary = running
		? E('button', {
			'class': 'btn cbi-button cbi-button-remove',
			'title': _('Stop the proxy and hold it down (survives reboot) until started again'),
			'click': ui.createHandlerFn(view, function() {
				return runAction(view, callStop, _('sing-box stopped'), _('Stop failed'));
			})
		}, _('Stop'))
		: E('button', {
			'class': 'btn cbi-button cbi-button-apply',
			'disabled': data.daemon ? null : 'disabled',
			'click': ui.createHandlerFn(view, function() {
				return runAction(view, function() { return startRuntime(data); }, _('sing-box started'), _('Start failed'));
			})
		}, _('Start'));

	var subtitle = running
		? _('PID %s').format(valueOrDash(data.sing_box_pid))
		: (!data.daemon ? _('Manager daemon is not running')
			: (paused ? _('Management mode — proxy held down, LAN untouched')
				: (data.manager_enabled ? _('Manager enabled') : _('Manager disabled'))));

	var modeSelect = E('select', {
		'class': 'cbi-input-select pikabox-mode',
		'change': ui.createHandlerFn(view, function(ev) {
			var mode = ev.target.value;
			return runAction(view, function() { return callSetMode(mode); },
				_('Mode set to %s').format(mode), _('Set mode failed'));
		})
	}, MODE_OPTIONS.map(function(opt) {
		return E('option', { 'value': opt, 'selected': opt === data.runtime_mode ? 'selected' : null }, opt);
	}));

	var hops = data.hops || [];
	var exitSpeed = hops.length ? hops[hops.length - 1].speed_mbps : 0;
	var health = valueOrDash(data.health) + (data.latency_ms ? ' · ' + data.latency_ms + ' ms' : '') + (exitSpeed ? ' · ' + theme.mbps(exitSpeed) : '');

	return E('div', { 'class': 'pikabox-hero' }, [
		E('div', { 'class': 'pikabox-hero-status' }, [
			E('span', { 'class': 'pikabox-dot' + (running ? ' on' : (data.daemon ? ' idle' : ' off')) }),
			E('div', {}, [
				E('div', { 'class': 'pikabox-hero-state' }, running ? _('Running') : (paused ? _('Paused') : _('Stopped'))),
				E('div', { 'class': 'pikabox-hero-sub' }, [ subtitle ])
			])
		]),
		E('div', { 'class': 'pikabox-hero-facts' }, [
			E('div', {}, [ E('span', {}, _('Final')), E('strong', {}, theme.text(((data.hops || [])[0] || {}).name || data.final)) ]),
			E('div', {}, [ E('span', {}, _('Mode')), modeSelect ]),
			E('div', {}, [ E('span', {}, _('Exit')), E('strong', { 'class': 'pkb-oneline', 'title': (data.chain_names || []).concat([ data.selected_name || data.selected_outbound || '' ]).join(' → ') }, theme.text(data.selected_name || data.selected_outbound)) ]),
			E('div', {}, [ E('span', {}, _('Health')), E('strong', { 'class': healthClass(data.health) }, [ health ]) ])
		]),
		renderActions(view, data, primary)
	]);
}

function renderChart(title, series, color, total) {
	return E('div', { 'class': 'pikabox-chart' }, [
		E('div', { 'class': 'pikabox-chart-head' }, [
			E('span', { 'class': 'pikabox-chart-title' }, title),
			E('span', { 'class': 'pikabox-chart-rate' }, [ formatRate(lastValue(series)) ])
		]),
		sparkline(series, color),
		E('div', { 'class': 'pikabox-chart-foot' }, [ _('Total %s').format(formatBytes(total)) ])
	]);
}

function renderThroughput(data, hist) {
	return E('div', { 'class': 'pikabox-charts' }, [
		renderChart(_('Download'), rateSeries(hist, 'rx'), '#2271b1', data.rx_bytes),
		renderChart(_('Upload'), rateSeries(hist, 'tx'), '#0f7a39', data.tx_bytes)
	]);
}

var ICONS = {
	restart: '<path d="M3 12a9 9 0 1 0 3-6.7"/><path d="M3 4v5h5"/>',
	reload: '<path d="M21 12a9 9 0 0 1-15.5 6.3"/><path d="M3 12a9 9 0 0 1 15.5-6.3"/><path d="M18 2v4h-4"/><path d="M6 22v-4h4"/>',
	validate: '<path d="M12 3l8 3v6c0 4.5-3.4 8.2-8 9-4.6-.8-8-4.5-8-9V6z"/><path d="M8.5 12l2.5 2.5 4.5-5"/>'
};

function iconButton(view, icon, label, enabled, action) {
	var btn = E('button', {
		'type': 'button',
		'class': 'btn cbi-button pkb-icon-btn',
		'title': label,
		'aria-label': label,
		'disabled': enabled ? null : 'disabled',
		'click': ui.createHandlerFn(view, action)
	});
	// Static markup only (no data), so innerHTML is safe here.
	btn.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true">' + ICONS[icon] + '</svg>';
	return btn;
}

// renderActions puts the secondary runtime actions next to Start/Stop as
// icon buttons, so the hero holds every control for the proxy itself.
function renderActions(view, data, primary) {
	return E('div', { 'class': 'pikabox-hero-action pkb-actions' }, [
		E('div', { 'class': 'pkb-icon-group', 'role': 'group', 'aria-label': _('Runtime actions') }, [
			iconButton(view, 'restart', _('Restart sing-box'), data.running, function() {
				return runAction(view, callRestart, _('sing-box restarted'), _('Restart failed'));
			}),
			iconButton(view, 'reload', _('Reload config (re-render and hot-reload)'), data.running, function() {
				return runAction(view, callReload, _('sing-box reloaded'), _('Reload failed'));
			}),
			iconButton(view, 'validate', _('Validate config'), true, function() {
				return callValidate().then(function(result) {
					reportResult(result, _('Configuration is valid'), _('Configuration has errors'));
				});
			})
		]),
		primary
	]);
}

// renderStaleAlert warns when sing-box is down but its firewall rules and policy
// routing are still installed: proxied devices are then steered to a port
// nothing listens on and lose internet until the rules are removed.
function renderStaleAlert(view, data) {
	if (!data.stale_datapath)
		return '';
	return E('div', { 'class': 'pikabox-alert' }, [
		E('div', {}, [
			E('strong', {}, [ _('Proxied devices have no internet') ]),
			E('div', {}, [ _('sing-box is not running, but its transparent-proxy firewall rules and routing are still active. Start sing-box, or remove the rules to restore direct internet.') ])
		]),
		E('button', {
			'class': 'btn cbi-button cbi-button-negative',
			'click': ui.createHandlerFn(view, function() {
				return runAction(view, callCleanup, _('Proxy rules removed'), _('Removing proxy rules failed'));
			})
		}, [ _('Remove proxy rules') ])
	]);
}

// ---- exit path -------------------------------------------------------------

var ago = function(iso) { return theme.ago(iso); };

function hopKind(kind) {
	return kind;
}

// renderPath shows the live exit: default route → each group's current
// choice → the server actually carrying traffic, next to recent switches.
function renderPath(data) {
	var hops = data.hops || [];
	var chain;
	if (!data.running) {
		chain = E('div', { 'class': 'pkb-empty' }, [ _('Not running — start the proxy to see where traffic exits.') ]);
	} else if (!hops.length) {
		chain = E('div', { 'class': 'pkb-hops' }, [ E('span', { 'class': 'pkb-hop-start' }, [ _('Everything else') ]), E('span', { 'class': 'pkb-hop-arrow' }, [ '→' ]), theme.pill(data.final || 'direct', 'builtin') ]);
	} else {
		var parts = [ E('span', { 'class': 'pkb-hop-start' }, [ _('Everything else') ]) ];
		hops.forEach(function(hop, i) {
			var leaf = i === hops.length - 1;
			var wrap = E('div', { 'class': 'pkb-hopwrap' }, [ E('span', { 'class': 'pkb-hop-arrow', 'aria-hidden': 'true' }, [ '→' ]) ]);
			parts.push(wrap);
			var meta = [];
			if (hop.detail)
				meta.push(theme.modeName(hop.detail));
			if (hop.latency_ms)
				meta.push(hop.latency_ms + ' ms');
			if (hop.speed_mbps)
				meta.push(theme.mbps(hop.speed_mbps));
			wrap.appendChild(E('div', { 'class': 'pkb-hop' + (leaf ? ' pkb-leaf' : '') }, [
				theme.pill(hop.name, hopKind(hop.kind) === 'direct' || hopKind(hop.kind) === 'block' ? 'builtin' : hopKind(hop.kind)),
				E('span', { 'class': 'pkb-hop-meta' }, [ meta.join(' · ') || (leaf ? _('exit') : '') ])
			]));
		});
		chain = E('div', { 'class': 'pkb-hops' }, parts);
	}
	var events = data.events || [];
	return E('div', { 'class': 'pikabox-section' }, [
		E('div', { 'class': 'pkb-path' }, [
			E('div', {}, [ E('h3', { 'style': 'margin:0 0 10px' }, _('Exit path')), chain ]),
			E('div', {}, [
				E('h3', { 'style': 'margin:0 0 10px' }, _('Recent changes')),
				events.length ? E('ul', { 'class': 'pkb-timeline' }, events.map(function(ev) {
					return E('li', { 'class': 'pkb-ev-' + ev.kind }, [ E('time', { 'datetime': ev.time, 'title': new Date(ev.time).toLocaleString() }, [ ago(ev.time) ]), ev.message ]);
				})) : E('p', { 'class': 'pikabox-hint' }, _('No switches or selection changes since the daemon started.'))
			])
		])
	]);
}

// ---- health board ----------------------------------------------------------

function sparkSmall(points) {
	var box = E('div', { 'class': 'pkb-spark' });
	if (!points || points.length < 2)
		return box;
	var w = 200, h = 28, pad = 3;
	var max = points.reduce(function(m, p) { return Math.max(m, p.ms); }, 1);
	var step = w / (points.length - 1);
	var path = '', dots = '';
	points.forEach(function(p, i) {
		var x = (i * step).toFixed(1);
		if (p.ms > 0) {
			var y = (h - pad - (p.ms / max) * (h - 2 * pad)).toFixed(1);
			path += (path && points[i - 1] && points[i - 1].ms > 0 ? 'L' : 'M') + x + ',' + y + ' ';
		} else {
			dots += '<rect x="' + (x - 1.5) + '" y="' + (h - 4) + '" width="3" height="3" fill="#b42318"/>';
		}
	});
	// Numbers only are interpolated, so innerHTML is safe here.
	box.innerHTML = '<svg viewBox="0 0 ' + w + ' ' + h + '" preserveAspectRatio="none" aria-hidden="true"><path d="' + path + '" fill="none" stroke="#2271b1" stroke-width="1.6" vector-effect="non-scaling-stroke"/>' + dots + '</svg>';
	return box;
}

var COLUMN_TITLES = [ _('Routes'), _('Targets'), _('Members'), _('Servers'), _('Servers') ];

function graphNode(node, live) {
	var down = node.health === 'down' || node.health === 'error' || !!node.stalled;
	var ms;
	if (node.kind === 'rule')
		ms = node.health === 'idle' ? E('span', { 'class': 'pkb-node-ms idle' }, [ _('off') ]) : '';
	else if (node.kind === 'builtin')
		ms = '';
	else if (node.stalled)
		ms = E('span', { 'class': 'pkb-node-ms down', 'title': _('Answers latency tests but freezes on real traffic') }, [ _('stalls') ]);
	else if (node.health === 'ok' && node.latency_ms)
		ms = E('span', { 'class': 'pkb-node-ms' }, [ node.latency_ms + ' ms' + (node.speed_mbps ? ' · ' + theme.mbps(node.speed_mbps) : '') ]);
	else if (node.health === 'idle')
		ms = E('span', { 'class': 'pkb-node-ms idle' }, [ _('off') ]);
	else if (node.health === 'unknown')
		ms = E('span', { 'class': 'pkb-node-ms idle' }, [ '—' ]);
	else
		ms = E('span', { 'class': 'pkb-node-ms down' }, [ node.health === 'error' ? _('error') : _('down') ]);

	var foot = node.detail || '';
	if (node.kind === 'group')
		foot = theme.modeName(node.detail) + (node.now ? ' · ' + _('using %s').format(node.now) : '');
	if (node.more)
		foot = (foot ? foot + ' · ' : '') + _('+%d alive standby').format(node.more);
	if (down && !node.last_ok)
		foot = node.error || foot;
	else if (down)
		foot = _('ok %s').format(ago(node.last_ok));
	if (node.stalled)
		foot = _('%s on a 256 KB download').format(node.stalled);
	if (node.unused)
		foot = _('not used by any route');

	var kind = node.kind;
	var cls = 'pkb-node' + (down ? ' pkb-down' : '') + (live ? ' pkb-live' : '') + (node.unused ? ' pkb-unused' : '') + (node.kind === 'rule' ? ' pkb-rule' : '');
	return E('div', { 'class': cls, 'data-node': node.id, 'title': node.error || '' }, [
		E('div', { 'class': 'pkb-node-head' }, [ theme.pill(node.name || node.id, node.kind === 'builtin' ? 'builtin' : kind, node.name), ms ]),
		(node.history && node.history.length > 1 && node.kind !== 'rule') ? sparkSmall(node.history) : '',
		foot ? E('div', { 'class': 'pkb-node-foot', 'title': foot }, [ foot ]) : ''
	]);
}

// drawEdges connects parent → member with curves from the measured card
// positions; the path traffic takes right now is drawn bold.
function drawEdges(graph, edges, byId, live) {
	var svg = graph.querySelector('svg');
	if (!svg || !graph.isConnected)
		return;
	var box = graph.getBoundingClientRect();
	var paths = [];
	edges.forEach(function(edge) {
		var a = graph.querySelector('[data-node="' + CSS.escape(edge.from) + '"]');
		var b = graph.querySelector('[data-node="' + CSS.escape(edge.to) + '"]');
		if (!a || !b)
			return;
		var ra = a.getBoundingClientRect(), rb = b.getBoundingClientRect();
		var x1 = ra.right - box.left, y1 = ra.top + ra.height / 2 - box.top;
		var x2 = rb.left - box.left, y2 = rb.top + rb.height / 2 - box.top;
		var dx = Math.max(24, (x2 - x1) / 2);
		var target = byId[edge.to] || {};
		var on = edge.active && live[edge.from];
		var cls = 'pkb-edge' + (on ? ' pkb-on' : '') + ((target.health === 'down' || target.health === 'error') ? ' pkb-dead' : '');
		paths.push({ cls: cls, active: on, d: 'M' + x1.toFixed(1) + ',' + y1.toFixed(1) + ' C' + (x1 + dx).toFixed(1) + ',' + y1.toFixed(1) + ' ' + (x2 - dx).toFixed(1) + ',' + y2.toFixed(1) + ' ' + x2.toFixed(1) + ',' + y2.toFixed(1) });
	});
	// Active edges last so they paint on top.
	paths.sort(function(p, q) { return (p.active ? 1 : 0) - (q.active ? 1 : 0); });
	svg.innerHTML = paths.map(function(p) { return '<path class="' + p.cls + '" d="' + p.d + '"/>'; }).join('');
}

function renderGraph(data) {
	var nodes = data.nodes || [];
	var edges = data.edges || [];
	var byId = {};
	nodes.forEach(function(n) { byId[n.id] = n; });

	// Live nodes: everything reachable from the default route through active edges.
	var live = {};
	var activeFrom = {};
	edges.forEach(function(e) { if (e.active) (activeFrom[e.from] = activeFrom[e.from] || []).push(e.to); });
	(function mark(id) {
		if (live[id])
			return;
		live[id] = true;
		(activeFrom[id] || []).forEach(mark);
	})('route:final');

	// Order each column by the average position of its parents (fewer crossings).
	var columns = [];
	nodes.forEach(function(n) { (columns[n.column] = columns[n.column] || []).push(n); });
	var position = {};
	var parentsOf = {};
	edges.forEach(function(e) { (parentsOf[e.to] = parentsOf[e.to] || []).push(e.from); });
	columns.forEach(function(col, c) {
		if (!col)
			return;
		if (c > 0) {
			col.forEach(function(n) {
				var ps = (parentsOf[n.id] || []).filter(function(p) { return position[p] !== undefined; });
				n._rank = ps.length ? ps.reduce(function(sum, p) { return sum + position[p]; }, 0) / ps.length : 1e6;
			});
			col.sort(function(a, b) { return a._rank - b._rank; });
		}
		col.forEach(function(n, i) { position[n.id] = i; });
	});

	var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
	svg.setAttribute('aria-hidden', 'true');
	var graph = E('div', { 'class': 'pkb-graph' }, [ svg ].concat(columns.filter(Boolean).map(function(col, i) {
		return E('div', { 'class': 'pkb-col' }, [ E('div', { 'class': 'pkb-col-title' }, [ COLUMN_TITLES[col[0].column] || '' ]) ].concat(col.map(function(n) { return graphNode(n, live[n.id] && n.kind !== 'rule'); })));
	})));

	var redraw = function() { drawEdges(graph, edges, byId, live); };
	if (window.ResizeObserver)
		new ResizeObserver(redraw).observe(graph);
	window.setTimeout(redraw, 0);
	return E('div', {}, [
		E('div', { 'class': 'pkb-graph-scroll' }, graph),
		E('div', { 'class': 'pkb-graph-legend' }, [
			E('span', {}, [ E('i'), _('carrying traffic now') ]),
			E('span', {}, [ E('i', { 'class': 'pkb-l-idle' }), _('member') ]),
			E('span', {}, [ E('i', { 'class': 'pkb-l-dead' }), _('member is down') ])
		])
	]);
}

function renderHealth(view, data) {
	data = data || {};
	var warnings = data.warnings || [];
	var base = L.url('admin/services/pikabox');
	return E('div', { 'class': 'pikabox-section pkb-health' }, [
		E('div', { 'class': 'pikabox-section-header' }, [
			E('h3', {}, _('Health')),
			E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() {
				return callHealthCheck().then(function(result) {
					reportResult(result, _('All outbounds and subscriptions re-tested'), _('Re-test failed'));
					return refreshHealth(view);
				});
			}) }, _('Re-test all'))
		]),
		warnings.length ? E('div', { 'class': 'pkb-warnings' }, warnings.map(function(w) {
			return E('div', { 'class': 'pkb-warning' + (w.level === 'error' ? ' pkb-error' : ''), 'role': w.level === 'error' ? 'alert' : null }, [
				E('strong', {}, theme.text(w.title)),
				E('span', {}, theme.text(w.message)),
				w.page && w.page !== 'dashboard' ? E('a', { 'href': base + '/' + w.page }, [ _('Open') + ' →' ]) : ''
			]);
		})) : '',
		(data.nodes || []).length > 1 ? renderGraph(data)
			: E('div', { 'class': 'pkb-empty' }, [ _('Nothing to monitor yet — add an outbound or a subscription.') ])
	]);
}

function refreshHealth(view) {
	return callHealth().then(function(data) {
		var current = view.root && view.root.querySelector('.pkb-health');
		if (current)
			current.parentNode.replaceChild(renderHealth(view, data), current);
	});
}

// ---- connections -----------------------------------------------------------

function formatDuration(iso) {
	var s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
	if (s < 60)
		return s + 's';
	if (s < 3600)
		return Math.floor(s / 60) + 'm ' + (s % 60) + 's';
	return Math.floor(s / 3600) + 'h ' + Math.floor((s % 3600) / 60) + 'm';
}

function connChain(c) {
	return E('div', { 'class': 'pkb-chain' }, (c.chain || []).map(function(name, i) {
		var kind = (c.chain_kind || [])[i];
		return theme.pill(name, kind === 'direct' || kind === 'block' ? 'builtin' : kind);
	}));
}

function renderConnections(view) {
	view.conn = view.conn || { prev: {}, filter: '', paused: false };
	var state = view.conn;
	var body = E('tbody');
	var stats = E('div', { 'class': 'pkb-conn-stats', 'aria-live': 'polite' });
	var filter = E('input', { 'type': 'search', 'placeholder': _('Filter host, device, route, exit'), 'aria-label': _('Filter connections'), 'value': state.filter });
	var pause = E('button', { 'type': 'button', 'class': 'btn cbi-button' }, [ _('Pause') ]);
	filter.addEventListener('input', function() { state.filter = filter.value.trim().toLowerCase(); draw(); });
	pause.addEventListener('click', function() {
		state.paused = !state.paused;
		pause.textContent = state.paused ? _('Resume') : _('Pause');
	});

	var draw = function() {
		var list = state.list || [];
		var q = state.filter;
		var rows = list.filter(function(c) {
			if (!q)
				return true;
			return [ c.host, c.device, c.source, c.route ].concat(c.chain || []).join(' ').toLowerCase().indexOf(q) >= 0;
		});
		body.replaceChildren.apply(body, rows.length ? rows.map(function(c) {
			var close = E('button', { 'type': 'button', 'class': 'pkb-x', 'aria-label': _('Close connection to %s').format(c.host), 'title': _('Close connection') }, [ '×' ]);
			close.addEventListener('click', function() {
				callCloseConnection(c.id).then(function() { return refreshConnections(view, true); });
			});
			return E('tr', { 'class': c.rate > 64 * 1024 ? 'pkb-hot' : '' }, [
				E('td', {}, [
					E('div', { 'class': 'pkb-host', 'title': c.host + ':' + c.port }, theme.text(c.host)),
					E('span', { 'class': 'pkb-sub' }, [ (c.network || '') + ' :' + (c.port || '') ])
				]),
				E('td', {}, [ E('span', {}, theme.text(c.device || c.source)), c.device ? E('span', { 'class': 'pkb-sub' }, theme.text(c.source)) : '' ]),
				E('td', { 'title': c.rule || '' }, [ c.route_key === 'final' ? E('span', { 'class': 'pikabox-hint' }, [ c.route ]) : theme.pill(c.route, 'rule') ]),
				E('td', {}, connChain(c)),
				E('td', { 'class': 'pkb-num' }, [ c.rate_down ? formatRate(c.rate_down) : '–', E('span', { 'class': 'pkb-sub' }, [ c.rate_up ? '↑ ' + formatRate(c.rate_up) : '' ]) ]),
				E('td', { 'class': 'pkb-num' }, [ formatBytes(c.download), E('span', { 'class': 'pkb-sub' }, [ '↑ ' + formatBytes(c.upload) ]) ]),
				E('td', { 'class': 'pkb-num' }, [ formatDuration(c.start) ]),
				E('td', {}, close)
			]);
		}) : [ E('tr', {}, E('td', { 'colspan': 8, 'class': 'pkb-empty', 'style': 'border:0' }, [
			state.running === false ? _('Not running.') : (q ? _('No connection matches the filter.') : _('No open connections.'))
		])) ]);
		var down = list.reduce(function(sum, c) { return sum + (c.rate_down || 0); }, 0);
		var up = list.reduce(function(sum, c) { return sum + (c.rate_up || 0); }, 0);
		stats.replaceChildren(
			E('span', {}, [ q ? _('%d of %d open').format(rows.length, list.length) : _('%d open').format(list.length) ]),
			E('span', {}, [ '↓ ' + formatRate(down) ]),
			E('span', {}, [ '↑ ' + formatRate(up) ])
		);
	};
	state.draw = draw;

	return E('div', { 'class': 'pikabox-section pkb-conns' }, [
		E('div', { 'class': 'pkb-conn-tools' }, [
			E('h3', { 'style': 'margin:0' }, _('Connections')),
			stats,
			E('div', { 'class': 'pikabox-actions' }, [ E('label', { 'class': 'pkb-search' }, [ filter ]), pause ])
		]),
		E('div', { 'class': 'pkb-conn-wrap' }, E('table', { 'class': 'pkb-conn' }, [
			E('thead', {}, E('tr', {}, [
				E('th', {}, _('Destination')), E('th', {}, _('Device')), E('th', {}, _('Route')), E('th', {}, _('Exit')),
				E('th', { 'class': 'pkb-num' }, _('Speed')), E('th', { 'class': 'pkb-num' }, _('Total')), E('th', { 'class': 'pkb-num' }, _('Age')), E('th')
			])),
			body
		]))
	]);
}

// refreshConnections polls the connection list and derives per-connection
// speed from the byte counters' change since the previous poll.
function refreshConnections(view, force) {
	var state = view.conn;
	if (!state || (state.paused && !force))
		return Promise.resolve();
	return callConnections().then(function(data) {
		var now = Date.now();
		var prev = state.prev || {};
		var next = {};
		var list = (data && data.connections) || [];
		list.forEach(function(c) {
			var p = prev[c.id];
			if (p && now > p.t) {
				var dt = (now - p.t) / 1000;
				c.rate_down = Math.max(0, (c.download - p.down) / dt);
				c.rate_up = Math.max(0, (c.upload - p.up) / dt);
			}
			c.rate = (c.rate_down || 0) + (c.rate_up || 0);
			next[c.id] = { t: now, down: c.download, up: c.upload };
		});
		list.sort(function(a, b) { return (b.rate || 0) - (a.rate || 0) || (a.start < b.start ? 1 : -1); });
		state.prev = next;
		state.list = list;
		state.running = data && data.running;
		if (state.draw)
			state.draw();
	});
}

// renderBanner is the small brand strip at the top of the dashboard.
function renderBanner(data) {
	return E('div', { 'class': 'pkb-banner' }, [
		E('img', { 'src': L.resource('pikabox/logo.svg'), 'width': '44', 'height': '44', 'alt': '' }),
		E('div', { 'class': 'pkb-banner-text' }, [
			E('div', { 'class': 'pkb-banner-name' }, [ 'Pika-box', data.version ? E('span', { 'class': 'pkb-banner-ver' }, [ 'v' + String(data.version).replace(/^v/, '') ]) : '' ]),
			E('div', { 'class': 'pkb-banner-tag' }, [ _('sing-box manager for OpenWrt') ])
		])
	]);
}

function renderLive(view, data) {
	data = data || {};
	var hist = pushHistory(view, data);
	return E('div', { 'class': 'pikabox-live' }, [
		renderStaleAlert(view, data),
		renderHero(view, data),
		(data.warnings && data.warnings.length)
			? E('div', { 'class': 'pikabox-warning' }, data.warnings.map(function(w) { return E('div', {}, theme.text(w)); }))
			: '',
		renderPath(data),
		renderThroughput(data, hist),
		E('div', { 'class': 'pikabox-metrics' }, [
			metric(_('Daemon'), data.daemon ? _('Online') : _('Offline'), data.daemon ? 'ok' : 'error'),
			metric(_('Connections'), String(data.connections || 0)),
			metric(_('Memory'), formatBytes((data.memory_kb || 0) * 1024)),
			metric(_('Final mode'), data.strategy ? theme.modeName(data.strategy) : '-')
		])
	]);
}

// refreshLive fetches status and swaps the live panel. A poll tick is skipped
// while the user is interacting with a control inside the panel (e.g. the mode
// dropdown is open), otherwise the rebuild would close it mid-selection.
function refreshLive(view, force) {
	return callStatus().then(function(status) {
		var current = view.root && view.root.querySelector('.pikabox-live');
		if (!current)
			return;
		var active = document.activeElement;
		if (!force && active && active.tagName === 'SELECT' && current.contains(active))
			return;
		current.parentNode.replaceChild(renderLive(view, status || {}), current);
	});
}

function downloadText(text) {
	var blob = new Blob([ text || '' ], { type: 'text/plain' });
	var url = URL.createObjectURL(blob);
	var link = E('a', { 'href': url, 'download': 'pikabox.log' });
	document.body.appendChild(link);
	link.click();
	link.remove();
	window.setTimeout(function() { URL.revokeObjectURL(url); }, 1000);
}

// updateLogs replaces the log text in place. When the reader is parked at the
// bottom it follows new lines; when they have scrolled up to read something,
// their position is kept instead of jumping on every poll.
function updateLogs(view, data) {
	var pre = view.logEl;
	if (!pre)
		return;
	var text = theme.stripAnsi((data && data.text) || '');
	view.logText = text;
	var atBottom = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 24;
	var top = pre.scrollTop;
	pre.textContent = text || _('No log output yet');
	pre.scrollTop = atBottom ? pre.scrollHeight : top;
}

function refreshLogs(view) {
	return callLogs(LOG_LINES).then(function(result) {
		updateLogs(view, result || {});
	});
}

function renderLogs(view, data) {
	view.logEl = E('pre', { 'class': 'pikabox-log' });
	var box = E('div', { 'class': 'pikabox-logs' }, [
		E('div', { 'class': 'pikabox-section-header' }, [
			E('h3', {}, _('Logs')),
			E('div', { 'class': 'pikabox-toolbar' }, [
				E('button', {
					'class': 'btn cbi-button',
					'click': ui.createHandlerFn(view, function() { return refreshLogs(view); })
				}, _('Refresh')),
				E('button', {
					'class': 'btn cbi-button',
					'click': function() { downloadText(view.logText); }
				}, _('Download'))
			])
		]),
		view.logEl
	]);
	updateLogs(view, data);
	// Start at the newest line once the element is laid out.
	window.requestAnimationFrame(function() {
		view.logEl.scrollTop = view.logEl.scrollHeight;
	});
	return box;
}

return view.extend({
	handleSaveApply: null,
	handleSave: null,
	handleReset: null,

	load: function() {
		return Promise.all([ callStatus(), callLogs(LOG_LINES), callHealth() ]).then(function(results) {
			return { status: results[0], logs: results[1], health: results[2] };
		});
	},

	render: function(data) {
		var view = this;
		data = data || {};
		theme.inject();

		view.root = E('div', { 'class': 'pikabox-dashboard' }, [
			renderBanner(data.status || {}),
			renderLive(view, data.status || {}),
			renderHealth(view, data.health),
			renderConnections(view),
			renderLogs(view, data.logs || {})
		]);
		refreshConnections(view, true);

		poll.add(function() { return refreshLive(view, false); }, POLL_INTERVAL);
		poll.add(function() { return refreshHealth(view); }, 15);
		poll.add(function() { return refreshConnections(view, false); }, 3);
		poll.add(function() { return refreshLogs(view); }, POLL_INTERVAL);

		return view.root;
	}
});
