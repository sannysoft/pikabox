'use strict';
'require view';
'require rpc';
'require ui';
'require view.singbox-manager.theme as theme';

var callList = rpc.declare({ object: 'singbox.manager', method: 'outbounds', expect: { '': {} } });
var callSet = rpc.declare({ object: 'singbox.manager', method: 'outbound_set', params: [ 'outbound' ], expect: { '': {} } });
var callDelete = rpc.declare({ object: 'singbox.manager', method: 'outbound_delete', params: [ 'id' ], expect: { '': {} } });
var callTest = rpc.declare({ object: 'singbox.manager', method: 'outbound_test', params: [ 'id' ], expect: { '': {} } });

function showModal(view, row) {
	row = row || { enabled: true, source: '' };
	theme.modal(view, row.id ? _('Edit Outbound') : _('Add Outbound'), [
		theme.checkbox('enabled', row.enabled !== false, _('Enabled')),
		E('label', {}, [ _('Name'), E('input', { 'class': 'cbi-input-text', 'name': 'name', 'value': row.name || '', 'placeholder': 'nl-vp1' }) ]),
		E('label', {}, [
			_('Source — share URI or sing-box outbound JSON'),
			E('textarea', { 'class': 'cbi-input-textarea singbox-manager-mono', 'name': 'source', 'placeholder': 'vless://uuid@host:443?security=reality&pbk=…#name\n\nor\n\n{"type":"vless","server":"…","server_port":443,…}' }, [ row.source || '' ])
		]),
		E('p', { 'class': 'singbox-manager-inline-help' }, _('Supported URIs: vless, vmess, trojan, ss, hysteria2/hy2, tuic. JSON is passed to sing-box as-is (its tag is replaced by this outbound\'s ID), so any outbound type and option works.'))
	], function(root) {
		return callSet({
			id: row.id || '',
			enabled: theme.value(root, 'enabled'),
			name: theme.value(root, 'name'),
			source: theme.value(root, 'source')
		});
	}, _('Outbound saved'));
}

function renderTable(view, outbounds) {
	return E('table', { 'class': 'singbox-manager-table' }, [
		E('thead', {}, E('tr', {}, [
			E('th', {}, _('Name')),
			E('th', {}, _('ID / tag')),
			E('th', {}, _('Type')),
			E('th', {}, _('Server')),
			E('th', {}, _('Latency')),
			E('th', {}, _('Checked')),
			E('th', {}, _('Enabled')),
			E('th', {}, '')
		])),
		E('tbody', {}, outbounds.length ? outbounds.map(function(ob) {
			var probe = ob.probe || {};
			return E('tr', {}, [
				E('td', {}, theme.text(ob.name)),
				E('td', { 'class': 'singbox-manager-mono' }, theme.text(ob.id)),
				E('td', {}, ob.error ? E('span', { 'class': 'singbox-manager-status-error', 'title': ob.error }, [ _('invalid') ]) : theme.text(ob.type)),
				E('td', { 'class': 'singbox-manager-wrap' }, theme.text(ob.server ? ob.server + ':' + ob.port : '')),
				E('td', {}, theme.latency(probe.latency_ms, probe.health, probe.error)),
				E('td', {}, theme.text(probe.checked_at ? new Date(probe.checked_at).toLocaleTimeString() : '')),
				E('td', {}, ob.enabled ? _('Yes') : _('No')),
				E('td', {}, E('div', { 'class': 'singbox-manager-actions' }, [
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
		}) : E('tr', {}, E('td', { 'colspan': 8 }, _('No outbounds yet'))))
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
		return E('div', { 'class': 'singbox-manager-page' }, [
			E('div', { 'class': 'singbox-manager-section' }, [
				E('div', { 'class': 'singbox-manager-section-header' }, [
					E('h3', {}, _('Outbounds')),
					E('div', { 'class': 'singbox-manager-toolbar' }, [
						E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() {
							return callTest('').then(function(result) {
								if (theme.result(result, _('All outbounds tested')))
									return theme.refresh(view);
							});
						}) }, _('Test all')),
						E('button', { 'class': 'btn cbi-button cbi-button-add', 'click': ui.createHandlerFn(view, function() { showModal(view); }) }, _('Add'))
					])
				]),
				E('p', { 'class': 'singbox-manager-hint' }, _('Latency is a real request through each outbound (handshake + transport + HTTP GET of the test URL), re-measured every probe interval.')),
				(data && data.ok === false) ? E('div', { 'class': 'singbox-manager-warning' }, theme.text((data.errors || []).join('; '))) : '',
				renderTable(view, (data && data.outbounds) || [])
			])
		]);
	}
});
