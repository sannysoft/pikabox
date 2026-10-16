'use strict';
'require view';
'require rpc';
'require ui';
'require view.pikabox.theme as theme';

var callList = rpc.declare({ object: 'pikabox', method: 'subscriptions', expect: { '': {} } });
var callSet = rpc.declare({ object: 'pikabox', method: 'subscription_set', params: [ 'subscription' ], expect: { '': {} } });
var callDelete = rpc.declare({ object: 'pikabox', method: 'subscription_delete', params: [ 'id' ], expect: { '': {} } });
var callRefresh = rpc.declare({ object: 'pikabox', method: 'subscription_refresh', params: [ 'id' ], expect: { '': {} } });
var callTest = rpc.declare({ object: 'pikabox', method: 'subscription_test', params: [ 'id' ], expect: { '': {} } });
var callOutbounds = rpc.declare({ object: 'pikabox', method: 'subscription_outbounds', params: [ 'id' ], expect: { '': {} } });
var callPreview = rpc.declare({ object: 'pikabox', method: 'filter_preview', params: [ 'id', 'filter' ], expect: { '': {} } });

var FILTER_HELP = _('expr expression; empty keeps everything. Fields: name, type, server, port, outbound (the full sing-box JSON). Examples: %s').format(
	'type == "vless" && name matches "(?i)nl|netherlands"  ·  port == 443 && !(name contains "test")  ·  outbound.tls?.reality?.enabled == true');

function showModal(view, row) {
	row = row || { enabled: true, format: 'auto', count: 3, update_interval: '24h', filter: '' };
	var previewBox = E('div', { 'class': 'pikabox-inline-help' });
	var preview = function() {
		var root = document.querySelector('.pikabox-modal-form');
		return callPreview(row.id, theme.value(root, 'filter')).then(function(result) {
			if (!result.ok) {
				previewBox.replaceChildren(E('span', { 'class': 'pikabox-status-error' }, theme.text((result.errors || []).join('; '))));
				return;
			}
			var names = (result.names || []).slice(0, 30).join(', ') + ((result.names || []).length > 30 ? ', …' : '');
			previewBox.replaceChildren(E('span', {}, [ _('Matches %d of %d: ').format(result.matched, result.total), names ]));
		});
	};
	theme.modal(view, row.id ? _('Edit Subscription') : _('Add Subscription'), [
		theme.checkbox('enabled', row.enabled !== false, _('Enabled')),
		E('label', {}, [ _('Name'), E('input', { 'class': 'cbi-input-text', 'name': 'name', 'value': row.name || '', 'placeholder': 'provider-1' }) ]),
		E('label', {}, [ _('URL'), E('input', { 'class': 'cbi-input-text', 'name': 'url', 'value': row.url || '', 'placeholder': 'https://provider.example/sub/…' }) ]),
		E('label', {}, [ _('Format'), theme.select('format', [ 'auto', 'base64', 'plain' ], row.format || 'auto') ]),
		E('label', {}, [ _('User-Agent (optional)'), E('input', { 'class': 'cbi-input-text', 'name': 'user_agent', 'value': row.user_agent || '', 'placeholder': 'INCY/2.0.0/Android (default)' }) ]),
		theme.checkbox('hwid', !!row.hwid, _('Send HWID (device headers) — for panels with a device limit; registers this router as one device')),
		E('label', {}, [ _('Filter'), E('textarea', { 'class': 'cbi-input-textarea pikabox-mono', 'name': 'filter', 'style': 'min-height:64px' }, [ row.filter || '' ]) ]),
		E('p', { 'class': 'pikabox-inline-help' }, [ FILTER_HELP ]),
		row.id ? E('div', { 'class': 'pikabox-actions' }, [
			E('button', { 'type': 'button', 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, preview) }, _('Preview filter')),
			previewBox
		]) : '',
		E('label', {}, [ _('Use the N fastest alive (0 = all alive)'), E('input', { 'class': 'cbi-input-text', 'name': 'count', 'type': 'number', 'min': '0', 'value': String(row.count || 0) }) ]),
		E('label', {}, [ _('Refetch interval'), E('input', { 'class': 'cbi-input-text', 'name': 'update_interval', 'value': row.update_interval || '24h' }) ])
	], function(root) {
		return callSet({
			id: row.id || '',
			enabled: theme.value(root, 'enabled'),
			name: theme.value(root, 'name'),
			url: theme.value(root, 'url'),
			format: theme.value(root, 'format'),
			user_agent: theme.value(root, 'user_agent'),
			hwid: theme.value(root, 'hwid'),
			filter: theme.value(root, 'filter'),
			count: parseInt(theme.value(root, 'count'), 10) || 0,
			update_interval: theme.value(root, 'update_interval')
		});
	}, _('Subscription saved'));
}

function showOutbounds(view, sub) {
	return callOutbounds(sub.id).then(function(result) {
		var rows = (result.outbounds || []).slice().sort(function(a, b) {
			if (a.matched !== b.matched)
				return a.matched ? -1 : 1;
			var la = a.health === 'ok' ? a.latency_ms : 1e9, lb = b.health === 'ok' ? b.latency_ms : 1e9;
			return la - lb;
		});
		ui.showModal(_('Outbounds of %s').format(sub.name), [
			E('div', { 'style': 'max-height:65vh;overflow:auto' }, E('table', { 'class': 'pikabox-table' }, [
				E('thead', {}, E('tr', {}, [
					E('th', {}, _('Name')), E('th', {}, _('Type')), E('th', {}, _('Server')),
					E('th', {}, _('Filter')), E('th', {}, _('Latency')), E('th', {}, _('Bulk')), E('th', {}, _('In use'))
				])),
				E('tbody', {}, rows.length ? rows.map(function(c) {
					return E('tr', { 'class': c.selected ? 'pikabox-current' : '' }, [
						E('td', { 'class': 'pikabox-wrap' }, theme.text(c.name)),
						E('td', {}, theme.text(c.type)),
						E('td', { 'class': 'pikabox-wrap' }, theme.text(c.server + ':' + c.port)),
						E('td', {}, c.matched ? _('pass') : _('filtered out')),
						E('td', {}, c.matched ? theme.latency(c.latency_ms, c.health, c.error) : '-'),
						E('td', {}, c.matched ? theme.bulk(c.bulk, c.bulk_kbps, c.bulk_error) : '-'),
						E('td', {}, c.selected ? _('Yes') : '')
					]);
				}) : E('tr', {}, E('td', { 'colspan': 7 }, _('Nothing fetched yet'))))
			])),
			E('div', { 'class': 'right' }, E('button', { 'class': 'btn cbi-button', 'click': ui.hideModal }, _('Close')))
		]);
	});
}

function renderTable(view, subs) {
	return E('table', { 'class': 'pikabox-table' }, [
		E('thead', {}, E('tr', {}, [
			E('th', {}, _('Name')),
			E('th', {}, _('Fetched / filtered / alive')),
			E('th', {}, _('Best')),
			E('th', {}, _('In use (fastest first)')),
			E('th', {}, _('Updated')),
			E('th', {}, '')
		])),
		E('tbody', {}, subs.length ? subs.map(function(sub) {
			return E('tr', {}, [
				E('td', {}, [
					E('div', {}, [ E('strong', {}, theme.text(sub.name)), sub.enabled ? '' : E('span', { 'class': 'pikabox-badge off', 'style': 'margin-left:6px' }, _('disabled')) ]),
					E('div', { 'class': 'pikabox-hint pikabox-mono', 'title': sub.filter || '' }, theme.text(sub.filter ? sub.filter.slice(0, 60) : _('no filter'))),
					E('div', { 'class': 'pikabox-hint' }, theme.text(sub.count ? _('top %d').format(sub.count) : _('all alive')))
				]),
				E('td', {}, [
					E('div', {}, [ '%d / %d / %d'.format(sub.total || 0, sub.matched || 0, sub.alive || 0) ]),
					sub.skipped ? E('div', { 'class': 'pikabox-hint', 'title': sub.skipped }, theme.text(_('skipped: %s').format(sub.skipped.length > 70 ? sub.skipped.slice(0, 70) + '…' : sub.skipped))) : ''
				]),
				E('td', {}, theme.latency(sub.best_ms, sub.alive ? 'ok' : (sub.matched ? 'down' : ''))),
				E('td', {}, E('div', { 'class': 'pikabox-members' }, (sub.selected || []).length ? sub.selected.map(function(c) {
					return E('span', { 'class': 'pikabox-member', 'title': c.server + ':' + c.port }, [ c.name + ' · ' + (c.latency_ms ? c.latency_ms + ' ms' : '-') ]);
				}) : [ E('span', { 'class': 'pikabox-hint' }, _('none — not used yet')) ])),
				E('td', {}, [
					E('div', {}, theme.timeAgo(sub.last_update, _('never'))),
					sub.last_error ? E('div', { 'class': 'pikabox-status-error', 'title': sub.last_error }, theme.text(sub.last_error.slice(0, 80))) : ''
				]),
				E('td', {}, E('div', { 'class': 'pikabox-actions' }, [
					E('button', { 'class': 'btn cbi-button', 'title': _('Refetch, filter and re-test'), 'click': ui.createHandlerFn(view, function() {
						return callRefresh(sub.id).then(function(result) {
							if (theme.result(result, _('Fetched %d, %d match the filter').format(result.total || 0, result.matched || 0)))
								return theme.refresh(view);
						});
					}) }, _('Refresh')),
					E('button', { 'class': 'btn cbi-button', 'title': _('Re-test the filtered outbounds'), 'click': ui.createHandlerFn(view, function() {
						return callTest(sub.id).then(function(result) {
							if (theme.result(result))
								return theme.refresh(view);
						});
					}) }, _('Test')),
					E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() { return showOutbounds(view, sub); }) }, _('Outbounds')),
					E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() { showModal(view, sub); }) }, _('Edit')),
					E('button', { 'class': 'btn cbi-button cbi-button-remove', 'click': ui.createHandlerFn(view, function() {
						theme.confirmDelete(view, _('Delete Subscription'),
							_('Delete "%s"? It is removed from outbound groups and routes that target it are dropped.').format(sub.name),
							function() { return callDelete(sub.id); });
					}) }, _('Delete'))
				]))
			]);
		}) : E('tr', {}, E('td', { 'colspan': 6 }, _('No subscriptions yet'))))
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
					E('h3', {}, _('Subscriptions')),
					E('button', { 'class': 'btn cbi-button cbi-button-add', 'click': ui.createHandlerFn(view, function() { showModal(view); }) }, _('Add'))
				]),
				E('p', { 'class': 'pikabox-hint' }, _('Each subscription is fetched, filtered, delay-tested, and the N fastest alive outbounds are used wherever it is referenced (as a fastest-wins group with the subscription\'s ID). The selection is re-evaluated every probe interval.')),
				(data && data.ok === false) ? E('div', { 'class': 'pikabox-warning' }, theme.text((data.errors || []).join('; '))) : '',
				renderTable(view, (data && data.subscriptions) || [])
			])
		]);
	}
});
