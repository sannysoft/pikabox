'use strict';
'require view';
'require rpc';
'require ui';
'require view.singbox-manager.theme as theme';

var callList = rpc.declare({ object: 'singbox.manager', method: 'rule_groups', expect: { '': {} } });
var callSet = rpc.declare({ object: 'singbox.manager', method: 'rule_group_set', params: [ 'group' ], expect: { '': {} } });
var callDelete = rpc.declare({ object: 'singbox.manager', method: 'rule_group_delete', params: [ 'id' ], expect: { '': {} } });
var callRefreshSets = rpc.declare({ object: 'singbox.manager', method: 'rulesets_refresh', expect: { '': {} } });

var PLACEHOLDERS = {
	domain: 'example.com', domain_suffix: 'telegram.org', domain_keyword: 'google', domain_regex: '^(.+\\.)?netflix\\.com$',
	ip_cidr: '91.108.4.0/22', source_ip_cidr: '192.168.1.50/32', ip_is_private: 'true', port: '443', port_range: '1000:2000',
	source_port: '5000', source_port_range: '5000:6000', network: 'udp', protocol: 'bittorrent', process_name: 'curl',
	process_path: '/usr/bin/curl', package_name: 'org.telegram.messenger',
	remote: 'https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-telegram.srs'
};

function showModal(view, data, row) {
	row = row || { enabled: true, rules: [] };
	var types = data.rule_types || Object.keys(PLACEHOLDERS);
	var valueInput = E('input', { 'class': 'cbi-input-text', 'placeholder': PLACEHOLDERS[types[0]] || '' });
	var typeSelect = theme.select('_type', types, types[0], {
		'change': function(ev) { valueInput.placeholder = PLACEHOLDERS[ev.target.value] || ''; }
	});
	var textarea = E('textarea', { 'class': 'cbi-input-textarea singbox-manager-mono', 'name': 'rules', 'style': 'min-height:200px' }, [ (row.rules || []).join('\n') ]);
	theme.modal(view, row.id ? _('Edit Rule Group') : _('Add Rule Group'), [
		theme.checkbox('enabled', row.enabled !== false, _('Enabled')),
		E('label', {}, [ _('Name'), E('input', { 'class': 'cbi-input-text', 'name': 'name', 'value': row.name || '', 'placeholder': 'telegram' }) ]),
		E('div', { 'class': 'singbox-manager-route-row', 'style': 'grid-template-columns:minmax(140px,200px) minmax(0,1fr) auto' }, [
			typeSelect,
			valueInput,
			E('button', { 'type': 'button', 'class': 'btn cbi-button cbi-button-add', 'click': function() {
				var value = valueInput.value.trim();
				if (!value)
					return;
				textarea.value = (textarea.value.trim() ? textarea.value.trim() + '\n' : '') + typeSelect.value + ':' + value;
				valueInput.value = '';
			} }, _('Add rule'))
		]),
		E('label', {}, [ _('Rules — one "type:value" per line'), textarea ]),
		E('p', { 'class': 'singbox-manager-inline-help' }, _('A connection matches the group when ANY rule matches. remote:<url> is a sing-box rule-set (.srs binary, otherwise JSON source); it is downloaded through the tunnel and cached on the router.'))
	], function(root) {
		return callSet({
			id: row.id || '',
			enabled: theme.value(root, 'enabled'),
			name: theme.value(root, 'name'),
			rules: theme.value(root, 'rules').split('\n').map(function(l) { return l.trim(); }).filter(function(l) { return l && l.charAt(0) !== '#'; })
		});
	}, _('Rule group saved'));
}

function summarize(group, cached) {
	var counts = {};
	(group.rules || []).forEach(function(rule) {
		var type = rule.split(':')[0];
		counts[type] = (counts[type] || 0) + 1;
	});
	var remotes = (group.rules || []).filter(function(r) { return r.indexOf('remote:') === 0; });
	return E('div', {}, [
		E('div', { 'class': 'singbox-manager-members' }, Object.keys(counts).map(function(type) {
			return E('span', { 'class': 'singbox-manager-member' }, [ type + (counts[type] > 1 ? ' ×' + counts[type] : '') ]);
		})),
		remotes.length ? E('div', { 'class': 'singbox-manager-hint' }, remotes.map(function(r) {
			var url = r.slice(7);
			return E('div', { 'class': 'singbox-manager-mono', 'title': url }, [ (cached[url] ? '✓ ' : '… ') + url.replace(/^https?:\/\//, '').slice(0, 70) ]);
		})) : ''
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
		var groups = data.groups || [];
		var cached = data.remote_cached || {};
		theme.inject();
		return E('div', { 'class': 'singbox-manager-page' }, [
			E('div', { 'class': 'singbox-manager-section' }, [
				E('div', { 'class': 'singbox-manager-section-header' }, [
					E('h3', {}, _('Rule Groups')),
					E('div', { 'class': 'singbox-manager-toolbar' }, [
						E('button', { 'class': 'btn cbi-button', 'title': _('Re-download every remote rule-set now'), 'click': ui.createHandlerFn(view, function() {
							return callRefreshSets().then(function(result) {
								if (theme.result(result, _('Remote rule-sets refreshed')))
									return theme.refresh(view);
							});
						}) }, _('Update remote sets')),
						E('button', { 'class': 'btn cbi-button cbi-button-add', 'click': ui.createHandlerFn(view, function() { showModal(view, data); }) }, _('Add'))
					])
				]),
				(data.ok === false) ? E('div', { 'class': 'singbox-manager-warning' }, theme.text((data.errors || []).join('; '))) : '',
				E('table', { 'class': 'singbox-manager-table' }, [
					E('thead', {}, E('tr', {}, [ E('th', {}, _('Name')), E('th', {}, _('Rules')), E('th', {}, _('Enabled')), E('th', {}, '') ])),
					E('tbody', {}, groups.length ? groups.map(function(g) {
						return E('tr', {}, [
							E('td', {}, [ E('strong', {}, theme.text(g.name)), E('div', { 'class': 'singbox-manager-hint singbox-manager-mono' }, theme.text(g.id)) ]),
							E('td', { 'class': 'singbox-manager-wrap', 'style': 'max-width:520px' }, summarize(g, cached)),
							E('td', {}, g.enabled ? _('Yes') : _('No')),
							E('td', {}, E('div', { 'class': 'singbox-manager-actions' }, [
								E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() { showModal(view, data, g); }) }, _('Edit')),
								E('button', { 'class': 'btn cbi-button cbi-button-remove', 'click': ui.createHandlerFn(view, function() {
									theme.confirmDelete(view, _('Delete Rule Group'),
										_('Delete "%s"? Routes using it are removed, and it is dropped from DNS rules.').format(g.name),
										function() { return callDelete(g.id); });
								}) }, _('Delete'))
							]))
						]);
					}) : E('tr', {}, E('td', { 'colspan': 4 }, _('No rule groups yet'))))
				])
			])
		]);
	}
});
