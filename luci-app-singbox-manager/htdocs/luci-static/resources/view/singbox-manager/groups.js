'use strict';
'require view';
'require rpc';
'require ui';
'require view.singbox-manager.theme as theme';

var callList = rpc.declare({ object: 'singbox.manager', method: 'outbound_groups', expect: { '': {} } });
var callSet = rpc.declare({ object: 'singbox.manager', method: 'outbound_group_set', params: [ 'group' ], expect: { '': {} } });
var callDelete = rpc.declare({ object: 'singbox.manager', method: 'outbound_group_delete', params: [ 'id' ], expect: { '': {} } });

var MODES = [
	{ id: 'urltest', name: _('Fastest (urltest)') },
	{ id: 'fallback', name: _('First alive, in order (fallback)') }
];

function showModal(view, data, row) {
	row = row || { enabled: true, mode: 'fallback', members: [] };
	var options = (data.targets || []).filter(function(t) { return t.id !== row.id; });
	theme.modal(view, row.id ? _('Edit Outbound Group') : _('Add Outbound Group'), [
		theme.checkbox('enabled', row.enabled !== false, _('Enabled')),
		E('label', {}, [ _('Name'), E('input', { 'class': 'cbi-input-text', 'name': 'name', 'value': row.name || '', 'placeholder': 'us-out' }) ]),
		E('label', {}, [ _('Mode'), theme.select('mode', MODES, row.mode || 'fallback') ]),
		E('div', {}, [
			E('div', { 'class': 'singbox-manager-label', 'style': 'margin-bottom:6px' }, _('Members (order matters for fallback)')),
			theme.orderedList(row.members || [], options)
		]),
		E('p', { 'class': 'singbox-manager-inline-help' }, _('A subscription member stands for its N fastest filtered outbounds (fastest of them is used). Groups can nest. Fallback probes members in order every 30 s and switches to the first one that answers.'))
	], function(root) {
		return callSet({
			id: row.id || '',
			enabled: theme.value(root, 'enabled'),
			name: theme.value(root, 'name'),
			mode: theme.value(root, 'mode'),
			members: theme.orderedValues(root)
		});
	}, _('Outbound group saved'));
}

function renderMembers(group) {
	return E('div', { 'class': 'singbox-manager-members' }, (group.member_info || []).length ? group.member_info.map(function(m, i) {
		var cls = 'singbox-manager-member' + (m.id === group.now ? ' now' : '') + (m.available ? '' : ' off');
		var title = m.available ? m.kind : _('%s — not available (disabled, broken, or nothing selected)').format(m.kind);
		return E('span', { 'class': cls, 'title': title }, [
			(group.mode === 'fallback' ? (i + 1) + '. ' : '') + (m.name || m.id) + (m.latency_ms ? ' · ' + m.latency_ms + ' ms' : '')
		]);
	}) : [ E('span', { 'class': 'singbox-manager-hint' }, _('no members')) ]);
}

function renderTable(view, data) {
	var groups = data.groups || [];
	return E('table', { 'class': 'singbox-manager-table' }, [
		E('thead', {}, E('tr', {}, [
			E('th', {}, _('Name')),
			E('th', {}, _('Mode')),
			E('th', {}, _('Members')),
			E('th', {}, _('Now')),
			E('th', {}, _('Latency')),
			E('th', {}, '')
		])),
		E('tbody', {}, groups.length ? groups.map(function(g) {
			return E('tr', {}, [
				E('td', {}, [
					E('div', {}, [ E('strong', {}, theme.text(g.name)), g.enabled ? '' : E('span', { 'class': 'singbox-manager-badge off', 'style': 'margin-left:6px' }, _('disabled')) ]),
					E('div', { 'class': 'singbox-manager-hint singbox-manager-mono' }, theme.text(g.id))
				]),
				E('td', {}, theme.text(g.mode === 'fallback' ? _('fallback') : _('fastest'))),
				E('td', {}, renderMembers(g)),
				E('td', {}, g.available ? theme.text(g.now || _('(not running)')) : E('span', { 'class': 'singbox-manager-status-error' }, _('unavailable'))),
				E('td', {}, theme.latency(g.latency_ms)),
				E('td', {}, E('div', { 'class': 'singbox-manager-actions' }, [
					E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() { showModal(view, data, g); }) }, _('Edit')),
					E('button', { 'class': 'btn cbi-button cbi-button-remove', 'click': ui.createHandlerFn(view, function() {
						theme.confirmDelete(view, _('Delete Outbound Group'),
							_('Delete "%s"? Routes that send to it are dropped and the final target is cleared if it pointed here.').format(g.name),
							function() { return callDelete(g.id); });
					}) }, _('Delete'))
				]))
			]);
		}) : E('tr', {}, E('td', { 'colspan': 6 }, _('No outbound groups yet'))))
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
		data = data || {};
		theme.inject();
		return E('div', { 'class': 'singbox-manager-page' }, [
			E('div', { 'class': 'singbox-manager-section' }, [
				E('div', { 'class': 'singbox-manager-section-header' }, [
					E('h3', {}, _('Outbound Groups')),
					E('button', { 'class': 'btn cbi-button cbi-button-add', 'click': ui.createHandlerFn(view, function() { showModal(view, data); }) }, _('Add'))
				]),
				(data.warnings && data.warnings.length) ? E('div', { 'class': 'singbox-manager-warning' }, data.warnings.map(function(w) { return E('div', {}, theme.text(w)); })) : '',
				(data.ok === false) ? E('div', { 'class': 'singbox-manager-warning' }, theme.text((data.errors || []).join('; '))) : '',
				renderTable(view, data)
			])
		]);
	}
});
