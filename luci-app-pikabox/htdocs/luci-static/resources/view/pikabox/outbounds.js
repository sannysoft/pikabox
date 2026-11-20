'use strict';
'require view';
'require rpc';
'require ui';
'require view.pikabox.theme as theme';

var callList = rpc.declare({ object: 'pikabox', method: 'outbounds', expect: { '': {} } });
var callSet = rpc.declare({ object: 'pikabox', method: 'outbound_set', params: [ 'outbound' ], expect: { '': {} } });
var callDelete = rpc.declare({ object: 'pikabox', method: 'outbound_delete', params: [ 'id' ], expect: { '': {} } });
var callTest = rpc.declare({ object: 'pikabox', method: 'outbound_test', params: [ 'id' ], expect: { '': {} } });

function input(name, label, value, attrs) {
	return E('label', {}, [ label, E('input', Object.assign({ 'class': 'cbi-input-text', 'name': name, 'value': value == null ? '' : String(value) }, attrs || {})) ]);
}

function serverFields(ob, defaultPort) {
	return [
		input('server', _('Server'), ob.server, { 'placeholder': 'vp3.example.com' }),
		input('server_port', _('Port'), ob.server_port || defaultPort, { 'type': 'number', 'min': '1', 'max': '65535' }),
		input('username', _('Username'), ob.username),
		input('password', _('Password'), ob.password, { 'type': 'password', 'autocomplete': 'new-password' })
	];
}

// Protocols with a form; they need the sing-box-extended build. Each builds
// sing-box outbound JSON from the form, on top of the JSON being edited so
// options the form doesn't show are kept.
var PROTOCOLS = {
	mieru: {
		name: _('Mieru'),
		fields: function(ob) {
			return serverFields(ob, 8446).concat([
				E('label', {}, [ _('Transport'), theme.select('transport', [ 'TCP', 'UDP' ], ob.transport || 'TCP') ]),
				E('label', {}, [ _('Multiplexing'), theme.select('multiplexing', [
					{ id: '', name: _('Default') }, 'MULTIPLEXING_OFF', 'MULTIPLEXING_LOW', 'MULTIPLEXING_MIDDLE', 'MULTIPLEXING_HIGH'
				], ob.multiplexing || '') ])
			]);
		},
		build: function(root, ob) {
			ob.transport = theme.value(root, 'transport');
			ob.multiplexing = theme.value(root, 'multiplexing') || undefined;
		}
	},
	trusttunnel: {
		name: _('TrustTunnel'),
		fields: function(ob) {
			var tls = ob.tls || {};
			return serverFields(ob, 443).concat([
				theme.checkbox('quic', !!ob.quic, _('QUIC (HTTP/3 over UDP) — faster where UDP gets through; off = HTTP/2 over TCP')),
				input('server_name', _('TLS server name (SNI)'), tls.server_name, { 'placeholder': _('the server\'s domain') }),
				E('label', {}, [ _('Pinned certificate (PEM, optional — for a self-signed server)'),
					E('textarea', { 'class': 'cbi-input-textarea pikabox-mono', 'name': 'certificate', 'style': 'min-height:64px', 'placeholder': '-----BEGIN CERTIFICATE-----…' },
						[ [].concat(tls.certificate || []).join('\n') ]) ]),
				theme.checkbox('insecure', !!tls.insecure, _('Skip certificate verification (not recommended)'))
			]);
		},
		build: function(root, ob) {
			var quic = theme.value(root, 'quic'), pem = theme.value(root, 'certificate');
			ob.quic = quic || undefined;
			ob.tls = Object.assign({}, ob.tls, {
				enabled: true,
				server_name: theme.value(root, 'server_name') || undefined,
				alpn: [ quic ? 'h3' : 'h2' ],
				certificate: pem ? pem.split('\n').map(function(l) { return l.trim(); }).filter(Boolean) : undefined,
				insecure: theme.value(root, 'insecure') || undefined
			});
		}
	}
};

// parseSource returns the outbound JSON when the source is JSON of a protocol
// that has a form.
function parseSource(source) {
	try {
		var ob = JSON.parse(source || '');
		return ob && PROTOCOLS[ob.type] ? ob : null;
	} catch (e) {
		return null;
	}
}

function showModal(view, row) {
	row = row || { enabled: true, source: '' };
	var edited = parseSource(row.source) || {};
	var box = E('div');
	var render = function(protocol) {
		if (!PROTOCOLS[protocol]) {
			box.replaceChildren(
				E('label', {}, [
					_('Source — share URI or sing-box outbound JSON'),
					E('textarea', { 'class': 'cbi-input-textarea pikabox-mono', 'name': 'source', 'placeholder': 'vless://uuid@host:443?security=reality&pbk=…#name\n\nor\n\n{"type":"vless","server":"…","server_port":443,…}' }, [ row.source || '' ])
				]),
				E('p', { 'class': 'pikabox-inline-help' }, _('Supported URIs: vless, vmess, trojan, ss, hysteria2/hy2, tuic (vless also with XHTTP). JSON is passed to sing-box as-is (its tag is replaced by this outbound\'s ID), so any outbound type and option works.')));
			return;
		}
		box.replaceChildren.apply(box, PROTOCOLS[protocol].fields(edited.type === protocol ? edited : {}).concat([
			E('p', { 'class': 'pikabox-inline-help' }, _('Needs sing-box-extended on the router.'))
		]));
	};
	var current = edited.type || 'source';
	var picker = theme.select('protocol', [ { id: 'source', name: _('Share link or JSON') } ].concat(Object.keys(PROTOCOLS).map(function(id) {
		return { id: id, name: PROTOCOLS[id].name };
	})), current, { 'change': function(ev) { render(ev.target.value); } });
	render(current);
	theme.modal(view, row.id ? _('Edit Outbound') : _('Add Outbound'), [
		theme.checkbox('enabled', row.enabled !== false, _('Enabled')),
		E('label', {}, [ _('Name'), E('input', { 'class': 'cbi-input-text', 'name': 'name', 'value': row.name || '', 'placeholder': 'nl-vp1' }) ]),
		E('label', {}, [ _('Protocol'), picker ]),
		box
	], function(root) {
		var protocol = theme.value(root, 'protocol'), source = theme.value(root, 'source');
		if (PROTOCOLS[protocol]) {
			var ob = Object.assign({}, edited.type === protocol ? edited : {}, {
				type: protocol,
				server: theme.value(root, 'server'),
				server_port: parseInt(theme.value(root, 'server_port'), 10) || undefined,
				username: theme.value(root, 'username'),
				password: theme.value(root, 'password')
			});
			PROTOCOLS[protocol].build(root, ob);
			source = JSON.stringify(ob);
		}
		return callSet({
			id: row.id || '',
			enabled: theme.value(root, 'enabled'),
			name: theme.value(root, 'name'),
			source: source
		});
	}, _('Outbound saved'));
}

function usedBy(usages) {
	if (!usages || !usages.length)
		return E('span', { 'class': 'pikabox-hint' }, _('Not used'));
	return E('div', { 'class': 'pikabox-tags' }, usages.map(function(u) {
		var label;
		switch (u.kind) {
		case 'default': label = _('Default route'); break;
		case 'group':   label = _('Group: %s').format(u.name || u.id); break;
		case 'route':   label = _('Route %d: %s').format(u.position, u.name || u.id); break;
		case 'dns':     label = _('DNS: %s').format(u.name || u.id); break;
		case 'device':  label = _('Device: %s').format(u.name || u.id); break;
		default:        label = u.id;
		}
		return E('span', { 'class': 'pikabox-tag' }, [ label ]);
	}));
}

function dialCell(dial) {
	var parts = [];
	if (dial && dial.attempts) {
		var pct = dial.failures * 100 / dial.attempts;
		parts.push(E('span', {
			'class': dial.failures ? (pct >= 5 ? 'pikabox-status-error' : '') : 'pikabox-hint',
			'title': _('%d of %d connections failed in the last %s').format(dial.failures, dial.attempts, dial.window)
		}, [ '%s%%'.format(pct < 10 ? pct.toFixed(1) : Math.round(pct)) ]));
	}
	if (dial && dial.cooling_until)
		parts.push(E('div', { 'class': 'pikabox-status-error', 'style': 'font-size:12px' }, _('set aside until %s').format(new Date(dial.cooling_until).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }))));
	return parts.length ? E('div', {}, parts) : E('span', { 'class': 'pikabox-hint' }, '—');
}

function renderTable(view, outbounds) {
	return E('table', { 'class': 'pikabox-table' }, [
		E('thead', {}, E('tr', {}, [
			E('th', {}, _('Name')),
			E('th', {}, _('ID / tag')),
			E('th', {}, _('Type')),
			E('th', {}, _('Server')),
			E('th', {}, _('Used by')),
			E('th', {}, _('Latency')),
			E('th', { 'title': _('256 KB download through the outbound, re-checked hourly') }, _('Bulk')),
			E('th', { 'title': _('Download speed, measured when something chooses this outbound by speed') }, _('Speed')),
			E('th', { 'title': _('Connections through this outbound that could not reach its server, over the recent window') }, _('Failed')),
			E('th', {}, _('Checked')),
			E('th', {}, _('Enabled')),
			E('th', {}, '')
		])),
		E('tbody', {}, outbounds.length ? outbounds.map(function(ob) {
			var probe = ob.probe || {};
			return E('tr', {}, [
				E('td', {}, theme.text(ob.name)),
				E('td', { 'class': 'pikabox-mono' }, theme.text(ob.id)),
				E('td', {}, ob.error ? E('span', { 'class': 'pikabox-status-error', 'title': ob.error }, [ _('invalid') ]) : theme.text(ob.type)),
				E('td', { 'class': 'pikabox-wrap' }, theme.text(ob.server ? ob.server + ':' + ob.port : '')),
				E('td', {}, usedBy(ob.used_by)),
				E('td', {}, theme.latency(probe.latency_ms, probe.health, probe.error)),
				E('td', {}, theme.bulk(probe.bulk, probe.bulk_kbps, probe.bulk_error)),
				E('td', {}, theme.speed(probe.speed_mbps, probe.speed_error, probe.speed_checked_at)),
				E('td', {}, dialCell(ob.dial)),
				E('td', {}, theme.timeAgo(probe.checked_at, _('never'))),
				E('td', {}, ob.enabled ? _('Yes') : _('No')),
				E('td', {}, E('div', { 'class': 'pikabox-actions' }, [
					E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() {
						return callTest(ob.id).then(function(result) {
							if (theme.result(result))
								return theme.refresh(view);
						});
					}) }, _('Test')),
					E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() { showModal(view, ob); }) }, _('Edit')),
					E('button', { 'class': 'btn cbi-button cbi-button-remove', 'click': ui.createHandlerFn(view, function() {
						theme.confirmDelete(view, _('Delete Outbound'),
							_('Delete "%s"? It is also removed from outbound groups, routes that target it are dropped, and the final target is cleared if it pointed here.').format(ob.name),
							function() { return callDelete(ob.id); });
					}) }, _('Delete'))
				]))
			]);
		}) : E('tr', {}, E('td', { 'colspan': 12 }, _('No outbounds yet'))))
	]);
}

return view.extend({
	handleSaveApply: null,
	handleSave: null,
	handleReset: null,

	load: function() {
		return callList();
	},

	render: function(data) {
		var view = this;
		theme.inject();
		return E('div', { 'class': 'pikabox-page' }, [
			E('div', { 'class': 'pikabox-section' }, [
				E('div', { 'class': 'pikabox-section-header' }, [
					E('h3', {}, _('Outbounds')),
					E('div', { 'class': 'pikabox-toolbar' }, [
						E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() {
							return callTest('').then(function(result) {
								if (theme.result(result, _('All outbounds tested')))
									return theme.refresh(view);
							});
						}) }, _('Test all')),
						E('button', { 'class': 'btn cbi-button cbi-button-add', 'click': ui.createHandlerFn(view, function() { showModal(view); }) }, _('Add'))
					])
				]),
				E('p', { 'class': 'pikabox-hint' }, _('Latency is a real request through each outbound, re-measured every probe interval. Bulk downloads 256 KB through it (hourly) to catch tunnels that answer quickly but freeze on real traffic; those are skipped by fastest and fallback picks.')),
				(data && data.ok === false) ? E('div', { 'class': 'pikabox-warning' }, theme.text((data.errors || []).join('; '))) : '',
				renderTable(view, (data && data.outbounds) || [])
			])
		]);
	}
});
