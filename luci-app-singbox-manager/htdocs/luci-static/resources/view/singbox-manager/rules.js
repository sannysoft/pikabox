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

function splitRule(rule) {
	var i = rule.indexOf(':');
	return i < 0 ? [ '', rule ] : [ rule.slice(0, i), rule.slice(i + 1) ];
}

function showModal(view, data, row) {
	row = row || { enabled: true, rules: [] };
	var types = data.rule_types || Object.keys(PLACEHOLDERS);
	var body = E('tbody');
	var count = E('span', { 'class': 'sbm-count' });
	var filter = E('input', { 'class': 'cbi-input-text', 'type': 'search', 'placeholder': _('Filter rules…'), 'style': 'max-width:220px' });

	var update = function() {
		var q = filter.value.trim().toLowerCase();
		var rows = body.querySelectorAll('tr');
		var shown = 0;
		rows.forEach(function(tr) {
			var text = (tr.querySelector('select').value + ':' + tr.querySelector('input').value).toLowerCase();
			var visible = !q || text.indexOf(q) >= 0;
			tr.style.display = visible ? '' : 'none';
			if (visible)
				shown++;
		});
		count.textContent = q ? _('%d of %d rules').format(shown, rows.length) : _('%d rules').format(rows.length);
	};

	var ruleRow = function(type, value) {
		type = types.indexOf(type) >= 0 ? type : types[0];
		var input = E('input', { 'class': 'cbi-input-text', 'value': value || '', 'placeholder': PLACEHOLDERS[type] || '', 'aria-label': _('Value') });
		var select = theme.select('_type', types, type, { 'aria-label': _('Type'), 'change': function() { input.placeholder = PLACEHOLDERS[select.value] || ''; } });
		var tr = E('tr', {}, [
			E('td', {}, select),
			E('td', {}, input),
			E('td', {}, E('button', { 'type': 'button', 'class': 'sbm-x', 'aria-label': _('Remove rule'), 'click': function() { tr.remove(); update(); } }, [ '×' ]))
		]);
		input.addEventListener('input', function() { tr.classList.remove('sbm-invalid'); });
		input.addEventListener('keydown', function(ev) {
			if (ev.key === 'Enter') {
				ev.preventDefault();
				var next = ruleRow(select.value, '');
				tr.parentNode.insertBefore(next, tr.nextSibling);
				next.querySelector('input').focus();
				update();
			}
		});
		return tr;
	};

	(row.rules || []).forEach(function(rule) {
		var parts = splitRule(rule);
		body.appendChild(ruleRow(parts[0], parts[1]));
	});
	if (!(row.rules || []).length)
		body.appendChild(ruleRow('domain_suffix', ''));

	// Bulk paste: "type:value" lines keep their type; bare values take the
	// chosen type — handy for pasting a domain or CIDR list.
	var pasteType = theme.select('_paste_type', types, 'domain_suffix');
	var pasteBox = E('textarea', { 'class': 'cbi-input-textarea singbox-manager-mono', 'style': 'min-height:110px', 'placeholder': 'telegram.org\nt.me\nip_cidr:91.108.4.0/22' });
	var paste = E('div', { 'style': 'display:none;gap:8px' }, [
		E('div', { 'class': 'sbm-add' }, [ E('span', { 'class': 'sbm-count' }, [ _('Bare lines become') ]), pasteType ]),
		pasteBox,
		E('div', { 'class': 'sbm-add' }, [
			E('button', { 'type': 'button', 'class': 'btn cbi-button cbi-button-add', 'click': function() {
				pasteBox.value.split('\n').map(function(l) { return l.trim(); }).filter(function(l) { return l && l.charAt(0) !== '#'; }).forEach(function(line) {
					var parts = splitRule(line);
					if (types.indexOf(parts[0]) < 0 || /^https?$/.test(parts[0]))
						parts = [ /^https?:\/\//.test(line) ? 'remote' : pasteType.value, line ];
					body.appendChild(ruleRow(parts[0], parts[1]));
				});
				pasteBox.value = '';
				paste.style.display = 'none';
				update();
			} }, _('Add lines'))
		])
	]);

	theme.modal(view, row.id ? _('Edit rule group') : _('New rule group'), [
		E('div', { 'class': 'sbm-rules-tools' }, [
			E('label', { 'style': 'flex:1 1 240px' }, [ _('Name'), E('input', { 'class': 'cbi-input-text', 'name': 'name', 'value': row.name || '', 'placeholder': 'telegram' }) ]),
			theme.checkbox('enabled', row.enabled !== false, _('Enabled'))
		]),
		E('div', { 'class': 'sbm-rules-tools' }, [ filter, count ]),
		E('div', { 'class': 'sbm-rules-scroll' }, E('table', { 'class': 'sbm-rules' }, [
			E('thead', {}, E('tr', {}, [ E('th', {}, _('Type')), E('th', {}, _('Value')), E('th') ])),
			body
		])),
		E('div', { 'class': 'sbm-add' }, [
			E('button', { 'type': 'button', 'class': 'btn cbi-button cbi-button-add', 'click': function() {
				var last = body.lastElementChild;
				var tr = ruleRow(last ? last.querySelector('select').value : 'domain_suffix', '');
				body.appendChild(tr);
				tr.querySelector('input').focus();
				update();
			} }, _('Add rule')),
			E('button', { 'type': 'button', 'class': 'btn cbi-button', 'click': function() {
				paste.style.display = paste.style.display === 'none' ? 'grid' : 'none';
				if (paste.style.display !== 'none')
					pasteBox.focus();
			} }, _('Paste list…'))
		]),
		paste,
		E('p', { 'class': 'singbox-manager-inline-help' }, _('A connection matches the group when any rule matches, so rule order does not matter. Enter in a value adds a row below. remote = a sing-box rule-set URL (.srs binary, otherwise JSON), downloaded through the tunnel and cached on the router.'))
	], function(root) {
		var rules = [];
		var bad = false;
		body.querySelectorAll('tr').forEach(function(tr) {
			var value = tr.querySelector('input').value.trim();
			if (!value)
				return;
			rules.push(tr.querySelector('select').value + ':' + value);
		});
		if (!rules.length) {
			body.querySelectorAll('tr').forEach(function(tr) { tr.classList.add('sbm-invalid'); });
			bad = true;
		}
		if (bad)
			return Promise.resolve({ ok: false, errors: [ _('Add at least one rule') ] });
		return callSet({
			id: row.id || '',
			enabled: theme.value(root, 'enabled'),
			name: theme.value(root, 'name'),
			rules: rules
		});
	}, _('Rule group saved'), true);
	filter.addEventListener('input', update);
	update();
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
			return E('span', { 'class': 'sbm-pill sbm-k-rule' }, [ type + (counts[type] > 1 ? ' ×' + counts[type] : '') ]);
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
