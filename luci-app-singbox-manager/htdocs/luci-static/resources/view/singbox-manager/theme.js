'use strict';
'require baseclass';
'require ui';

// Shared visual theme for every SingBox Manager view. The stylesheet is the
// single source of truth for the card-based look modelled on the dashboard; it
// is injected once into <head> (keyed by id) so every page renders identically
// and re-renders (poll / refreshView) never duplicate it.
var CSS = [
	/* ---- page + section cards ---- */
	'.singbox-manager-page{display:grid;gap:16px}',
	'.singbox-manager-page.narrow{max-width:960px}',
	'.singbox-manager-dashboard{display:grid;gap:16px}',
	'.singbox-manager-live{display:grid;gap:16px}',
	'.singbox-manager-section{display:grid;gap:12px;border:1px solid var(--border-color-medium);border-radius:12px;padding:16px 20px;background:var(--background-color-high)}',
	'.singbox-manager-section>h3{margin:0;font-size:15px;font-weight:600}',
	'.singbox-manager-section-header{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}',
	'.singbox-manager-section-header h3{margin:0;font-size:15px;font-weight:600}',
	'.singbox-manager-hint{margin:0;font-size:12px;color:var(--text-color-medium)}',
	'.singbox-manager-inline-help{margin:0;font-size:12px;color:var(--text-color-medium)}',

	/* ---- badges / status ---- */
	'.singbox-manager-badge{font-size:11px;font-weight:600;padding:3px 10px;border-radius:999px;background:var(--border-color-medium);color:var(--text-color-medium);white-space:nowrap}',
	'.singbox-manager-badge.on{background:rgba(15,122,57,.15);color:#0f7a39}',
	'.singbox-manager-badge.off{background:rgba(180,35,24,.12);color:#b42318}',
	'.singbox-manager-badge.warn{background:rgba(180,83,9,.14);color:#b45309}',
	'.singbox-manager-status-ok{color:#0f7a39;font-weight:600}',
	'.singbox-manager-status-error{color:#b42318;font-weight:600}',

	/* ---- controls ---- */
	'.singbox-manager-controls{display:flex;gap:18px;flex-wrap:wrap}',
	'.singbox-manager-inline-control{display:flex;align-items:center;gap:10px;font-size:13px}',
	'.singbox-manager-filters{display:flex;gap:8px;flex-wrap:wrap}',
	'.singbox-manager-actions{display:flex;gap:6px;flex-wrap:wrap;align-items:center}',
	'.singbox-manager-toolbar{display:flex;gap:8px;flex-wrap:wrap}',

	/* ---- definition grid (label / value) ---- */
	'.singbox-manager-grid{display:grid;grid-template-columns:minmax(130px,190px) minmax(0,1fr);gap:10px 14px;align-items:start}',
	'.singbox-manager-label{font-size:12px;color:var(--text-color-medium);font-weight:600}',

	/* ---- setting rows (toggle list) ---- */
	'.singbox-manager-rows{display:grid;gap:0}',
	'.singbox-manager-row{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:12px 0;border-top:1px solid var(--border-color-medium)}',
	'.singbox-manager-row:first-child{border-top:0}',
	'.singbox-manager-row-label{font-size:13px;font-weight:600}',
	'.singbox-manager-row-hint{font-size:11px;color:var(--text-color-medium);margin-top:2px}',
	'.singbox-manager-row-control{flex:0 0 auto;display:flex;align-items:center}',
	'.singbox-manager-row-control input.cbi-input-text{min-width:min(280px,46vw)}',
	'.singbox-manager-meta{display:flex;gap:16px;flex-wrap:wrap;font-size:11px;color:var(--text-color-medium);padding-top:4px}',

	/* ---- toggle switch ---- */
	'.singbox-manager-toggle{position:relative;display:inline-block;width:42px;height:24px;flex:0 0 auto}',
	'.singbox-manager-toggle input{position:absolute;opacity:0;width:0;height:0}',
	'.singbox-manager-toggle span{position:absolute;inset:0;cursor:pointer;background:var(--border-color-medium);border-radius:24px;transition:.2s}',
	'.singbox-manager-toggle span:before{content:"";position:absolute;height:18px;width:18px;left:3px;top:3px;background:#fff;border-radius:50%;transition:.2s}',
	'.singbox-manager-toggle input:checked+span{background:#2271b1}',
	'.singbox-manager-toggle input:checked+span:before{transform:translateX(18px)}',

	/* ---- tables ---- */
	'.singbox-manager-table{width:100%;border-collapse:collapse;font-size:13px}',
	'.singbox-manager-table th,.singbox-manager-table td{padding:9px 10px;text-align:left;vertical-align:middle}',
	'.singbox-manager-table thead th{font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);font-weight:600;border-bottom:2px solid var(--border-color-medium);white-space:nowrap}',
	'.singbox-manager-table tbody td{border-bottom:1px solid var(--border-color-medium)}',
	'.singbox-manager-table tbody tr:last-child td{border-bottom:0}',
	'.singbox-manager-table tbody tr:hover td{background:var(--background-color-low)}',
	'.singbox-manager-table tbody tr:not(.singbox-manager-group-row) td[colspan]{text-align:center;color:var(--text-color-medium);padding:18px 10px}',
	'.singbox-manager-table th:last-child,.singbox-manager-table td:last-child{text-align:right}',
	'.singbox-manager-table .singbox-manager-actions{justify-content:flex-end}',
	'.singbox-manager-table tbody tr.singbox-manager-current td{background:rgba(34,113,177,.10)}',
	'.singbox-manager-table td.singbox-manager-wrap{white-space:normal;overflow-wrap:anywhere;max-width:280px}',
	'.singbox-manager-group-row td{background:var(--background-color-low);font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);text-align:left!important}',

	/* ---- modal forms ---- */
	'.singbox-manager-modal-form{display:grid;gap:12px;min-width:min(520px,90vw)}',
	'.singbox-manager-modal-form.wide{grid-template-columns:repeat(auto-fit,minmax(180px,1fr));min-width:min(760px,90vw)}',
	'.singbox-manager-modal-form label{display:grid;gap:4px;font-size:12px;color:var(--text-color-medium)}',
	'.singbox-manager-check{display:flex!important;align-items:center;gap:8px}',
	'.singbox-manager-modal-form.wide .singbox-manager-check{margin-top:20px}',
	'.singbox-manager-import-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}',
	'.singbox-manager-import-grid label{display:grid;gap:4px;font-size:12px;color:var(--text-color-medium)}',
	'.singbox-manager-import-link{grid-column:1/-1}',
	'.singbox-manager-import-link textarea{min-height:96px;resize:vertical}',
	'.singbox-manager-import-help{grid-column:1/-1;margin:0;font-size:12px;color:var(--text-color-medium)}',

	/* ---- per-device override editor ---- */
	'.singbox-manager-device-editor{display:grid;gap:10px;padding-top:12px;border-top:1px solid var(--border-color-medium)}',
	'.singbox-manager-device-editor>h4{margin:0;font-size:13px;font-weight:600}',
	'.singbox-manager-devices{display:grid;gap:6px;overflow-x:auto}',
	// Fixed last two tracks (toggle, Remove) so the fr columns resolve to the
	// same widths in the header row (whose trailing cells are empty) as in the
	// data rows — otherwise the columns would not line up.
	'.singbox-manager-device-row{display:grid;grid-template-columns:minmax(90px,1.1fr) minmax(140px,1.3fr) minmax(104px,1fr) minmax(104px,1fr) minmax(96px,.9fr) minmax(90px,.9fr) 48px 48px 96px;gap:8px;align-items:center}',
	'.singbox-manager-device-row input.cbi-input-text,.singbox-manager-device-row select{width:100%;box-sizing:border-box;min-width:0;margin:0}',
	'.singbox-manager-device-row .singbox-manager-toggle{justify-self:center}',
	'.singbox-manager-device-row .cbi-button-remove{justify-self:end;white-space:nowrap}',
	'.singbox-manager-device-head{font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);font-weight:600;border-bottom:1px solid var(--border-color-medium);padding-bottom:6px}',
	'.singbox-manager-device-head span:nth-child(7),.singbox-manager-device-head span:nth-child(8){text-align:center}',

	/* ---- device picker chips ---- */
	'.singbox-manager-devicepicker{display:grid;gap:6px;margin-top:-4px}',
	'.singbox-manager-devicepicker-label{font-size:11px;color:var(--text-color-medium)}',
	'.singbox-manager-chips{display:flex;gap:6px;flex-wrap:wrap}',
	'.singbox-manager-chip{font-size:11px;padding:2px 8px!important;border-radius:12px}',

	/* ---- alerts + preformatted blocks ---- */
	'.singbox-manager-alert{display:flex;flex-wrap:wrap;gap:12px;align-items:center;justify-content:space-between;margin-bottom:12px;padding:12px 14px;border-radius:8px;border-left:4px solid #b91c1c;background:rgba(185,28,28,.10);font-size:13px}',
	'.singbox-manager-alert strong{display:block;margin-bottom:2px;color:#b91c1c}',
	'.singbox-manager-warning{padding:10px 12px;border-radius:8px;border-left:4px solid #b45309;background:rgba(180,83,9,.10);font-size:13px}',
	'.singbox-manager-preview{box-sizing:border-box;max-width:100%;min-height:60px;overflow:auto;padding:12px;border:1px solid var(--border-color-medium);border-radius:8px;background:var(--background-color-low);font:12px/1.5 monospace;white-space:pre}',
	'.singbox-manager-log{box-sizing:border-box;max-width:100%;min-height:360px;max-height:60vh;overflow:auto;padding:12px;border:1px solid var(--border-color-medium);border-radius:8px;background:var(--background-color-low);font:12px/1.5 monospace;white-space:pre-wrap;word-break:break-word}',
	'.singbox-manager-details{border:1px solid var(--border-color-medium);border-radius:8px;padding:0}',
	'.singbox-manager-details>summary{cursor:pointer;padding:10px 12px;font-size:13px;font-weight:600;color:var(--text-color-medium);list-style:none}',
	'.singbox-manager-details[open]>summary{border-bottom:1px solid var(--border-color-medium)}',
	'.singbox-manager-details>.singbox-manager-grid{padding:12px}',

	/* ---- dashboard hero ---- */
	'.singbox-manager-hero{display:flex;align-items:center;gap:20px;flex-wrap:wrap;border:1px solid var(--border-color-medium);border-radius:12px;padding:16px 20px;background:var(--background-color-high)}',
	'.singbox-manager-hero-status{display:flex;align-items:center;gap:14px;min-width:160px}',
	'.singbox-manager-dot{width:14px;height:14px;border-radius:50%;background:#9aa0a6;flex:0 0 auto;box-shadow:0 0 0 4px rgba(154,160,166,.18)}',
	'.singbox-manager-dot.on{background:#0f7a39;box-shadow:0 0 0 4px rgba(15,122,57,.18)}',
	'.singbox-manager-dot.idle{background:#d68b00;box-shadow:0 0 0 4px rgba(214,139,0,.18)}',
	'.singbox-manager-dot.off{background:#b42318;box-shadow:0 0 0 4px rgba(180,35,24,.18)}',
	'.singbox-manager-hero-state{font-size:22px;font-weight:700;line-height:1.1}',
	'.singbox-manager-hero-sub{font-size:12px;color:var(--text-color-medium)}',
	'.singbox-manager-hero-facts{display:grid;grid-template-columns:repeat(auto-fit,minmax(120px,1fr));gap:12px 20px;flex:1 1 320px}',
	'.singbox-manager-hero-facts>div{display:grid;gap:3px}',
	'.singbox-manager-hero-facts span{font-size:11px;color:var(--text-color-medium)}',
	'.singbox-manager-hero-facts strong{font-size:14px;overflow-wrap:anywhere}',
	'.singbox-manager-hero-facts strong.ok{color:#0f7a39}',
	'.singbox-manager-hero-facts strong.error{color:#b42318}',
	'.singbox-manager-hero-action{flex:0 0 auto}',
	'.singbox-manager-hero-action .btn{font-size:15px;padding:8px 26px}',

	/* ---- dashboard charts ---- */
	'.singbox-manager-charts{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:12px}',
	'.singbox-manager-chart{border:1px solid var(--border-color-medium);border-radius:12px;padding:14px;background:var(--background-color-high);display:grid;gap:8px}',
	'.singbox-manager-chart-head{display:flex;align-items:baseline;justify-content:space-between}',
	'.singbox-manager-chart-title{font-size:12px;color:var(--text-color-medium);font-weight:600}',
	'.singbox-manager-chart-rate{font-size:16px;font-weight:700}',
	'.singbox-manager-spark{height:72px}',
	'.singbox-manager-spark-empty{display:flex;align-items:center;justify-content:center;height:100%;font-size:12px;color:var(--text-color-medium)}',
	'.singbox-manager-chart-foot{font-size:11px;color:var(--text-color-medium)}',

	/* ---- dashboard metrics ---- */
	'.singbox-manager-metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px}',
	'.singbox-manager-metric{border:1px solid var(--border-color-medium);border-radius:12px;padding:14px;background:var(--background-color-high)}',
	'.singbox-manager-metric-label{font-size:12px;color:var(--text-color-medium);margin-bottom:6px}',
	'.singbox-manager-metric-value{font-size:18px;font-weight:600;overflow-wrap:anywhere}',
	'.singbox-manager-metric-value.ok{color:#0f7a39}',
	'.singbox-manager-metric-value.error{color:#b42318}',
	'.singbox-manager-logs{display:grid;gap:12px}',

	/* ---- latency / ordered lists ---- */
	'.singbox-manager-ms{font-variant-numeric:tabular-nums;font-weight:600}',
	'.singbox-manager-ms.fast{color:#0f7a39}',
	'.singbox-manager-ms.mid{color:#b45309}',
	'.singbox-manager-ms.slow,.singbox-manager-ms.down{color:#b42318}',
	'.singbox-manager-ms.none{color:var(--text-color-medium);font-weight:400}',
	'.singbox-manager-members{display:flex;gap:6px;flex-wrap:wrap}',
	'.singbox-manager-member{font-size:12px;padding:2px 8px;border-radius:12px;border:1px solid var(--border-color-medium);white-space:nowrap}',
	'.singbox-manager-member.now{border-color:#2271b1;background:rgba(34,113,177,.12)}',
	'.singbox-manager-member.off{opacity:.5;text-decoration:line-through}',
	'.singbox-manager-order{display:grid;gap:6px}',
	'.singbox-manager-order-row{display:grid;grid-template-columns:28px minmax(0,1fr) auto;gap:8px;align-items:center}',
	'.singbox-manager-route-row{display:grid;grid-template-columns:28px 48px minmax(0,1fr) 24px minmax(0,1fr) auto;gap:8px;align-items:center}',
	'.singbox-manager-route-row select{width:100%;min-width:0;margin:0}',
	'.singbox-manager-idx{font-size:12px;color:var(--text-color-medium);text-align:right}',
	'.singbox-manager-mono{font-family:monospace;font-size:12px}',
	'textarea.singbox-manager-mono{min-height:120px;resize:vertical;width:100%;box-sizing:border-box}',

	/* ---- responsive ---- */
	'@media(max-width:760px){',
	'.singbox-manager-grid{grid-template-columns:1fr}',
	'.singbox-manager-table{display:block;overflow-x:auto;white-space:nowrap}',
	'.singbox-manager-preview{white-space:pre-wrap;word-break:break-word}',
	'.singbox-manager-row{flex-direction:column;align-items:stretch}',
	'.singbox-manager-row-control{justify-content:flex-start}',
	'.singbox-manager-section{padding:14px}',
	'}'
].join('');

// ANSI SGR sequences (sing-box colours its log output). Matched both with the
// ESC byte and without it, since the syslog pipeline may have stripped or
// mangled the control character.
var ANSI_RE = /(?:\x1b|␛)?\[[0-9;]*m/g;

return baseclass.extend({
	// text wraps a value as a one-element child list. LuCI's E()/dom.append
	// assigns a lone string child via innerHTML; wrapping it in an array makes
	// it a text node instead, so names from subscriptions, DHCP hostnames, log
	// lines and backend errors can never inject markup into the admin session.
	text: function(value) {
		if (value === null || value === undefined || value === '')
			return [ '-' ];
		return [ String(value) ];
	},

	stripAnsi: function(value) {
		return String(value || '').replace(ANSI_RE, '');
	},

	// notify shows a success message that fades on its own (falling back to a
	// sticky one on LuCI builds without time-limited notifications).
	notify: function(message) {
		var body = E('p', {}, [ String(message) ]);
		if (typeof ui.addTimeLimitedNotification === 'function')
			ui.addTimeLimitedNotification(null, body, 5000, 'info');
		else
			ui.addNotification(null, body, 'info');
	},

	// error shows a sticky, red notification built from a backend result
	// ({ errors: [...] }) or a plain message.
	error: function(resultOrMessage, fallback) {
		var message = resultOrMessage;
		if (resultOrMessage && typeof resultOrMessage === 'object')
			message = (resultOrMessage.errors && resultOrMessage.errors.length)
				? resultOrMessage.errors.join('; ')
				: (resultOrMessage.message || fallback);
		ui.addNotification(null, E('p', {}, [ String(message || fallback || _('Request failed')) ]), 'danger');
	},


	// latency renders a probe result as a coloured "123 ms" / "down" badge.
	latency: function(ms, health, error) {
		if (health && health !== 'ok')
			return E('span', { 'class': 'singbox-manager-ms down', 'title': error || '' }, [ health === 'error' ? _('error') : _('down') ]);
		if (!ms)
			return E('span', { 'class': 'singbox-manager-ms none' }, [ '-' ]);
		var cls = ms < 200 ? 'fast' : (ms < 500 ? 'mid' : 'slow');
		return E('span', { 'class': 'singbox-manager-ms ' + cls }, [ ms + ' ms' ]);
	},

	field: function(root, name) {
		return root.querySelector('[name="%s"]'.format(name));
	},

	value: function(root, name) {
		var input = this.field(root, name);
		if (!input)
			return '';
		return input.type === 'checkbox' ? input.checked : input.value.trim();
	},

	// select builds a <select> from [{id, name}] or plain strings.
	select: function(name, options, current, attrs) {
		return E('select', Object.assign({ 'class': 'cbi-input-select', 'name': name }, attrs || {}), options.map(function(opt) {
			var id = typeof opt === 'string' ? opt : opt.id;
			var label = typeof opt === 'string' ? opt : (opt.label || opt.name || opt.id);
			return E('option', { 'value': id, 'selected': id === current ? 'selected' : null }, [ label ]);
		}));
	},

	checkbox: function(name, checked, label) {
		return E('label', { 'class': 'singbox-manager-check' }, [
			E('input', { 'type': 'checkbox', 'name': name, 'checked': checked ? 'checked' : null }),
			label
		]);
	},

	// refresh re-runs a view's load+render and swaps the page in place.
	refresh: function(view) {
		return view.load().then(function(data) {
			var replacement = view.render(data);
			var root = document.querySelector('.singbox-manager-page');
			if (root)
				root.parentNode.replaceChild(replacement, root);
		});
	},

	// result reports a backend result; returns true on success.
	result: function(result, success, failure) {
		if (result && result.ok) {
			if (result.pending)
				this.notify(result.message);
			else if (success)
				this.notify(result.warning ? success + ' — ' + result.warning : success);
			return true;
		}
		this.error(result, failure || _('Request failed'));
		return false;
	},

	// modal shows a form modal; save(root) returns a promise of a backend
	// result, and on success the modal closes and the view refreshes.
	modal: function(view, title, body, save, successText) {
		var self = this;
		ui.showModal(title, [
			E('div', { 'class': 'singbox-manager-modal-form' }, body),
			E('div', { 'class': 'right' }, [
				E('button', { 'class': 'btn cbi-button', 'click': ui.hideModal }, _('Cancel')),
				' ',
				E('button', {
					'class': 'btn cbi-button cbi-button-apply',
					'click': ui.createHandlerFn(view, function() {
						return save(document.querySelector('.singbox-manager-modal-form')).then(function(result) {
							if (!self.result(result, successText, _('Save failed')))
								return;
							ui.hideModal();
							return self.refresh(view);
						});
					})
				}, _('Save'))
			])
		]);
	},

	confirmDelete: function(view, title, text, remove) {
		var self = this;
		ui.showModal(title, [
			E('p', {}, [ text ]),
			E('div', { 'class': 'right' }, [
				E('button', { 'class': 'btn cbi-button', 'click': ui.hideModal }, _('Cancel')),
				' ',
				E('button', {
					'class': 'btn cbi-button cbi-button-remove',
					'click': ui.createHandlerFn(view, function() {
						return remove().then(function(result) {
							if (!self.result(result, _('Deleted'), _('Delete failed')))
								return;
							ui.hideModal();
							return self.refresh(view);
						});
					})
				}, _('Delete'))
			])
		]);
	},

	// orderedList is an editable, reorderable list of target IDs (used by
	// group members). options: [{id, name, kind}]. The list lives in the DOM;
	// read it back with orderedValues(root).
	orderedList: function(values, options) {
		var self = this;
		var box = E('div', { 'class': 'singbox-manager-order', 'data-ordered': '1' });
		var label = function(id) {
			var opt = options.filter(function(o) { return o.id === id; })[0];
			return opt ? '%s (%s)'.format(opt.name || opt.id, opt.kind) : id + ' (missing)';
		};
		var renumber = function() {
			box.querySelectorAll('.singbox-manager-idx').forEach(function(el, i) { el.textContent = (i + 1) + '.'; });
		};
		var row = function(id) {
			var el = E('div', { 'class': 'singbox-manager-order-row', 'data-value': id }, [
				E('span', { 'class': 'singbox-manager-idx' }),
				E('span', {}, [ label(id) ]),
				E('div', { 'class': 'singbox-manager-actions' }, [
					E('button', { 'type': 'button', 'class': 'btn cbi-button', 'title': _('Move up'), 'click': function() {
						if (el.previousElementSibling && el.previousElementSibling.dataset.value !== undefined)
							box.insertBefore(el, el.previousElementSibling);
						renumber();
					} }, [ '↑' ]),
					E('button', { 'type': 'button', 'class': 'btn cbi-button', 'title': _('Move down'), 'click': function() {
						var next = el.nextElementSibling;
						if (next && next.dataset.value !== undefined)
							box.insertBefore(next, el);
						renumber();
					} }, [ '↓' ]),
					E('button', { 'type': 'button', 'class': 'btn cbi-button cbi-button-remove', 'click': function() {
						el.remove();
						renumber();
					} }, [ '×' ])
				])
			]);
			return el;
		};
		var picker = self.select('_add', [ { id: '', name: _('— add member —') } ].concat(options.map(function(o) {
			return { id: o.id, name: '%s (%s)'.format(o.name || o.id, o.kind) };
		})), '');
		var adder = E('div', { 'class': 'singbox-manager-order-row' }, [
			E('span'),
			picker,
			E('button', { 'type': 'button', 'class': 'btn cbi-button cbi-button-add', 'click': function() {
				if (!picker.value)
					return;
				box.insertBefore(row(picker.value), adder);
				picker.value = '';
				renumber();
			} }, [ _('Add') ])
		]);
		values.forEach(function(id) { box.appendChild(row(id)); });
		box.appendChild(adder);
		renumber();
		return box;
	},

	orderedValues: function(root) {
		var box = root.querySelector('[data-ordered]');
		return box ? Array.prototype.map.call(box.querySelectorAll('[data-value]'), function(el) { return el.dataset.value; }) : [];
	},

	// inject adds the shared stylesheet to <head> once. Re-renders are common
	// (polling, refreshView replacing the page subtree); the id guard keeps a
	// single <style> element regardless of how often render() runs.
	inject: function() {
		if (document.getElementById('singbox-manager-theme'))
			return;
		var style = document.createElement('style');
		style.id = 'singbox-manager-theme';
		style.textContent = CSS;
		document.head.appendChild(style);
	}
});
