'use strict';
'require view';
'require rpc';
'require ui';
'require view.pikabox.theme as theme';

var callList = rpc.declare({ object: 'pikabox', method: 'outbounds', expect: { '': {} } });
var callSet = rpc.declare({ object: 'pikabox', method: 'outbound_set', params: [ 'outbound' ], expect: { '': {} } });
var callDelete = rpc.declare({ object: 'pikabox', method: 'outbound_delete', params: [ 'id' ], expect: { '': {} } });
var callTest = rpc.declare({ object: 'pikabox', method: 'outbound_test', params: [ 'id' ], expect: { '': {} } });

function showModal(view, row) {
	row = row || { enabled: true, source: '' };
	theme.modal(view, row.id ? _('Edit Outbound') : _('Add Outbound'), [
		theme.checkbox('enabled', row.enabled !== false, _('Enabled')),
		E('label', {}, [ _('Name'), E('input', { 'class': 'cbi-input-text', 'name': 'name', 'value': row.name || '', 'placeholder': 'nl-vp1' }) ]),
		E('label', {}, [
			_('Source — share URI or sing-box outbound JSON'),
			E('textarea', { 'class': 'cbi-input-textarea pikabox-mono', 'name': 'source', 'placeholder': 'vless://uuid@host:443?security=reality&pbk=…#name\n\nor\n\n{"type":"vless","server":"…","server_port":443,…}' }, [ row.source || '' ])
		]),
		E('p', { 'class': 'pikabox-inline-help' }, _('Supported URIs: vless, vmess, trojan, ss, hysteria2/hy2, tuic. JSON is passed to sing-box as-is (its tag is replaced by this outbound\'s ID), so any outbound type and option works.'))
	], function(root) {
		return callSet({
			id: row.id || '',
			enabled: theme.value(root, 'enabled'),
			name: theme.value(root, 'name'),
			source: theme.value(root, 'source')
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
		}) : E('tr', {}, E('td', { 'colspan': 10 }, _('No outbounds yet'))))
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
