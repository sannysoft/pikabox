'use strict';
'require view';
'require rpc';
'require ui';
'require view.pikabox.theme as theme';

var callRouting = rpc.declare({ object: 'pikabox', method: 'routing', expect: { '': {} } });
var callSetRouting = rpc.declare({ object: 'pikabox', method: 'routing_set', params: [ 'final', 'routes' ], expect: { '': {} } });
var callSetMode = rpc.declare({ object: 'pikabox', method: 'manager_set_mode', params: [ 'mode' ], expect: { '': {} } });

function kindOf(targets, id) {
	var t = (targets || []).filter(function(x) { return x.id === id; })[0];
	if (!t)
		return '';
	return t.kind === 'builtin' ? t.id : t.kind;
}

// targetSelect is a <select> of every target, its left edge coloured by the
// chosen target's kind (same colours as the pills elsewhere).
function targetSelect(name, targets, current, onChange) {
	var select = theme.select(name, (targets || []).map(function(t) {
		return { id: t.id, name: t.kind === 'builtin' ? t.name : '%s — %s'.format(t.name || t.id, t.kind) };
	}), current, { 'class': 'cbi-input-select pkb-target' });
	var paint = function() { select.dataset.kind = kindOf(targets, select.value); };
	select.addEventListener('change', function() { paint(); onChange(); });
	paint();
	return select;
}

function routeRow(data, route, onChange) {
	var groups = (data.rule_groups || []).map(function(g) { return { id: g.id, name: g.name || g.id }; });
	var ruleGroup = theme.select('rule_group', groups, route.rule_group, { 'class': 'cbi-input-select pkb-rulegroup', 'change': onChange, 'aria-label': _('Rule group') });
	var row = E('li', { 'class': 'pkb-item pkb-route' + (route.enabled === false ? ' pkb-off' : ''), 'data-sortable-item': '1' }, [
		theme.grip(),
		E('label', { 'class': 'pkb-switch', 'title': _('Enabled') }, [
			E('input', { 'type': 'checkbox', 'name': 'enabled', 'checked': route.enabled !== false ? 'checked' : null, 'change': function(ev) {
				row.classList.toggle('pkb-off', !ev.target.checked);
				onChange();
			} }),
			E('span')
		]),
		ruleGroup,
		E('span', { 'class': 'pkb-arrow', 'aria-hidden': 'true' }, [ '→' ]),
		targetSelect('outbound', data.targets, route.outbound, onChange),
		E('button', { 'type': 'button', 'class': 'pkb-x', 'aria-label': _('Remove route'), 'click': function() {
			row.remove();
			onChange();
		} }, [ '×' ])
	]);
	return row;
}

// syncRuleGroups lets each rule group be routed once: in every row's select,
// groups already taken by another row are disabled. Returns the first group
// no row uses yet (undefined when all are routed).
function syncRuleGroups(list, groups) {
	var selects = Array.prototype.slice.call(list.querySelectorAll('select[name="rule_group"]'));
	var used = selects.map(function(s) { return s.value; });
	selects.forEach(function(select, i) {
		Array.prototype.forEach.call(select.options, function(opt) {
			var taken = opt.value !== select.value && used.indexOf(opt.value) >= 0;
			opt.disabled = taken;
			opt.textContent = (groups.filter(function(g) { return g.id === opt.value; })[0] || {}).name + (taken ? ' — ' + _('already routed') : '');
		});
	});
	return groups.map(function(g) { return g.id; }).filter(function(id) { return used.indexOf(id) < 0; })[0];
}

function readRoutes(list) {
	return Array.prototype.map.call(list.querySelectorAll('[data-sortable-item]'), function(row) {
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

		var dirty = E('span', { 'class': 'pkb-dirty', 'role': 'status' }, [ _('Unsaved changes') ]);
		var empty = E('div', { 'class': 'pkb-empty' }, [ _('No routes — everything goes to the default below. Add a route to send a rule group somewhere else.') ]);
		var list = E('ol', { 'class': 'pkb-list' });
		var groups = (data.rule_groups || []).map(function(g) { return { id: g.id, name: g.name || g.id }; });
		var addButton;
		var sync = function() {
			var free = syncRuleGroups(list, groups);
			if (addButton) {
				addButton.disabled = !free;
				addButton.title = !groups.length ? _('Create a rule group first') : (free ? '' : _('Every rule group is already routed'));
			}
			return free;
		};
		var changed = function() {
			dirty.classList.add('on');
			empty.style.display = list.querySelector('[data-sortable-item]') ? 'none' : '';
			sync();
		};
		(data.routes || []).forEach(function(route) { list.appendChild(routeRow(data, route, changed)); });
		empty.style.display = (data.routes || []).length ? 'none' : '';
		theme.sortable(list, changed);

		var finalSelect = targetSelect('final', data.targets, data.final || 'direct', changed);

		var page = E('div', { 'class': 'pikabox-page' }, [
			E('div', { 'class': 'pikabox-section' }, [
				E('div', { 'class': 'pikabox-section-header' }, [
					E('h3', {}, _('Routing')),
					E('label', { 'class': 'pikabox-inline-control', 'title': _('rule: apply the routes below · global: everything to the default · direct: no proxy') }, [
						_('Mode'),
						theme.select('mode', [ 'rule', 'global', 'direct' ], data.mode || 'rule', {
							'change': ui.createHandlerFn(view, function(ev) {
								return callSetMode(ev.target.value).then(function(result) {
									theme.result(result, _('Mode set to %s').format(ev.target.value));
								});
							})
						})
					])
				]),
				E('p', { 'class': 'pikabox-hint' }, _('Checked top to bottom: the first rule group that matches picks the outbound. Drag rows by the handle to reorder. A route to a target that is unavailable right now is skipped.')),
				list,
				empty,
				E('div', { 'class': 'pkb-add' }, [
					addButton = E('button', {
						'class': 'btn cbi-button cbi-button-add',
						'click': function() {
							var free = sync();
							if (!free)
								return;
							list.appendChild(routeRow(data, { enabled: true, rule_group: free, outbound: data.final || 'direct' }, changed));
							changed();
						}
					}, _('Add route'))
				]),
				E('div', { 'class': 'pkb-final' }, [
					E('span'),
					E('span'),
					E('strong', {}, [ _('Everything else') ]),
					E('span', { 'class': 'pkb-arrow', 'aria-hidden': 'true' }, [ '→' ]),
					finalSelect,
					E('span')
				]),
				E('div', { 'class': 'pikabox-actions', 'style': 'justify-content:flex-end' }, [
					dirty,
					E('button', {
						'class': 'btn cbi-button cbi-button-apply',
						'click': ui.createHandlerFn(view, function() {
							return callSetRouting(finalSelect.value, readRoutes(list)).then(function(result) {
								if (theme.result(result, result.reloaded ? _('Routing saved and applied') : _('Routing saved')))
									return theme.refresh(view);
							});
						})
					}, _('Save routing'))
				])
			])
		]);
		sync();
		return page;
	}
});
