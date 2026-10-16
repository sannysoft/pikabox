'use strict';
'require view';
'require rpc';
'require ui';
'require view.singbox-manager.theme as theme';

var callRouting = rpc.declare({ object: 'singbox.manager', method: 'routing', expect: { '': {} } });
var callSetRouting = rpc.declare({ object: 'singbox.manager', method: 'routing_set', params: [ 'final', 'routes' ], expect: { '': {} } });
var callSetMode = rpc.declare({ object: 'singbox.manager', method: 'manager_set_mode', params: [ 'mode' ], expect: { '': {} } });

function targetOptions(targets) {
	return (targets || []).map(function(t) {
		return { id: t.id, name: t.kind === 'builtin' ? t.name : '%s (%s)'.format(t.name || t.id, t.kind) };
	});
}

function routeRow(data, route) {
	var row = E('div', { 'class': 'singbox-manager-route-row', 'data-route': '1' }, [
		E('span', { 'class': 'singbox-manager-idx' }),
		E('label', { 'class': 'singbox-manager-toggle', 'title': _('Enabled') }, [
			E('input', { 'type': 'checkbox', 'name': 'enabled', 'checked': route.enabled !== false ? 'checked' : null }),
			E('span')
		]),
		theme.select('rule_group', (data.rule_groups || []).map(function(g) { return { id: g.id, name: g.name || g.id }; }), route.rule_group),
		E('span', { 'style': 'text-align:center' }, [ '→' ]),
		theme.select('outbound', targetOptions(data.targets), route.outbound),
		E('div', { 'class': 'singbox-manager-actions' }, [
			E('button', { 'type': 'button', 'class': 'btn cbi-button', 'title': _('Move up'), 'click': function() {
				var prev = row.previousElementSibling;
				if (prev && prev.dataset.route)
					row.parentNode.insertBefore(row, prev);
				renumber(row.parentNode);
			} }, [ '↑' ]),
			E('button', { 'type': 'button', 'class': 'btn cbi-button', 'title': _('Move down'), 'click': function() {
				var next = row.nextElementSibling;
				if (next && next.dataset.route)
					row.parentNode.insertBefore(next, row);
				renumber(row.parentNode);
			} }, [ '↓' ]),
			E('button', { 'type': 'button', 'class': 'btn cbi-button cbi-button-remove', 'click': function() {
				var parent = row.parentNode;
				row.remove();
				renumber(parent);
			} }, [ '×' ])
		])
	]);
	return row;
}

function renumber(list) {
	list.querySelectorAll('[data-route] .singbox-manager-idx').forEach(function(el, i) { el.textContent = (i + 1) + '.'; });
}

function readRoutes(list) {
	return Array.prototype.map.call(list.querySelectorAll('[data-route]'), function(row) {
		return {
			enabled: theme.value(row, 'enabled'),
			rule_group: theme.value(row, 'rule_group'),
			outbound: theme.value(row, 'outbound')
		};
	});
}

return view.extend({
	handleSaveApply: null,
	handleSave: null,
	handleReset: null,

	load: function() {
		return callRouting();
	},

	render: function(data) {
		var view = this;
		data = data || {};
		theme.inject();

		var list = E('div', { 'class': 'singbox-manager-order' });
		(data.routes || []).forEach(function(route) { list.appendChild(routeRow(data, route)); });
		renumber(list);

		var finalSelect = theme.select('final', targetOptions(data.targets), data.final || 'direct');
		var noGroups = !(data.rule_groups || []).length;

		return E('div', { 'class': 'singbox-manager-page' }, [
			E('div', { 'class': 'singbox-manager-section' }, [
				E('h3', {}, _('Mode')),
				E('div', { 'class': 'singbox-manager-controls' }, [
					E('label', { 'class': 'singbox-manager-inline-control', 'title': _('rule = apply the routes below; global = everything to the default; direct = no proxy') }, [
						_('Mode'),
						theme.select('mode', [ 'rule', 'global', 'direct' ], data.mode || 'rule', {
							'change': ui.createHandlerFn(view, function(ev) {
								return callSetMode(ev.target.value).then(function(result) {
									theme.result(result, _('Mode updated'));
								});
							})
						})
					])
				])
			]),
			E('div', { 'class': 'singbox-manager-section' }, [
				E('div', { 'class': 'singbox-manager-section-header' }, [
					E('h3', {}, _('Routes')),
					E('button', {
						'class': 'btn cbi-button cbi-button-add',
						'disabled': noGroups ? 'disabled' : null,
						'title': noGroups ? _('Create a rule group first') : '',
						'click': function() {
							list.appendChild(routeRow(data, { enabled: true, outbound: data.final || 'direct' }));
							renumber(list);
						}
					}, _('Add route'))
				]),
				E('p', { 'class': 'singbox-manager-hint' }, _('Evaluated top to bottom; the first matching rule group decides the outbound. Routes to a target that is currently unavailable are skipped.')),
				list,
				E('div', { 'class': 'singbox-manager-route-row', 'style': 'border-top:1px solid var(--border-color-medium);padding-top:10px' }, [
					E('span'),
					E('span'),
					E('strong', {}, [ _('Default (everything else)') ]),
					E('span', { 'style': 'text-align:center' }, [ '→' ]),
					finalSelect,
					E('span')
				]),
				E('div', { 'class': 'right' }, E('button', {
					'class': 'btn cbi-button cbi-button-apply',
					'click': ui.createHandlerFn(view, function() {
						return callSetRouting(finalSelect.value, readRoutes(list)).then(function(result) {
							if (theme.result(result, result.reloaded ? _('Routing saved and applied') : _('Routing saved')))
								return theme.refresh(view);
						});
					})
				}, _('Save routing')))
			])
		]);
	}
});
