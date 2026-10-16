'use strict';
'require view';
'require rpc';
'require ui';
'require view.pikabox.theme as theme';

var callSettings = rpc.declare({ object: 'pikabox', method: 'settings', expect: { '': {} } });
var callSetSettings = rpc.declare({ object: 'pikabox', method: 'settings_set', params: [ 'settings' ], expect: { '': {} } });
var callExport = rpc.declare({ object: 'pikabox', method: 'config_export', expect: { '': {} } });
var callImport = rpc.declare({ object: 'pikabox', method: 'config_import', params: [ 'text', 'dry_run' ], expect: { '': {} } });
var callBackup = rpc.declare({ object: 'pikabox', method: 'backup_read', params: [ 'name' ], expect: { '': {} } });

var LOG_LEVELS = [ 'trace', 'debug', 'info', 'warn', 'error', 'fatal', 'panic' ];

var SUMMARY_LABELS = [
	[ 'outbounds', _('Outbounds') ], [ 'subscriptions', _('Subscriptions') ], [ 'outbound_groups', _('Outbound groups') ],
	[ 'rule_groups', _('Rule groups') ], [ 'routes', _('Routes') ], [ 'dns_servers', _('DNS servers') ],
	[ 'dns_rules', _('DNS rules') ], [ 'devices', _('Device overrides') ]
];

function download(name, text) {
	var url = URL.createObjectURL(new Blob([ text || '' ], { type: 'text/plain' }));
	var link = E('a', { 'href': url, 'download': name });
	document.body.appendChild(link);
	link.click();
	link.remove();
	window.setTimeout(function() { URL.revokeObjectURL(url); }, 1000);
}

// row is one labelled setting: label + hint on the left, control on the right.
function row(label, hint, control) {
	return E('div', { 'class': 'pikabox-row' }, [
		E('div', {}, [ E('div', { 'class': 'pikabox-row-label' }, [ label ]), hint ? E('div', { 'class': 'pikabox-row-hint' }, [ hint ]) : '' ]),
		E('div', { 'class': 'pikabox-row-control' }, [ control ])
	]);
}

function input(name, value, attrs) {
	return E('input', Object.assign({ 'class': 'cbi-input-text', 'name': name, 'value': value === undefined || value === null ? '' : String(value) }, attrs || {}));
}

function settingsSection(view, title, rows, keys) {
	var box = E('div', { 'class': 'pikabox-rows' }, rows);
	var dirty = E('span', { 'class': 'pkb-dirty', 'role': 'status' }, [ _('Unsaved changes') ]);
	box.addEventListener('input', function() { dirty.classList.add('on'); });
	box.addEventListener('change', function() { dirty.classList.add('on'); });
	return E('div', { 'class': 'pikabox-section' }, [
		E('h3', {}, [ title ]),
		box,
		E('div', { 'class': 'pikabox-actions', 'style': 'justify-content:flex-end' }, [
			dirty,
			E('button', { 'class': 'btn cbi-button cbi-button-apply', 'click': ui.createHandlerFn(view, function() {
				var payload = {};
				keys.forEach(function(key) { payload[key] = theme.value(box, key); });
				return callSetSettings(payload).then(function(result) {
					if (theme.result(result, result.reloaded ? _('Saved and applied') : _('Saved')))
						return theme.refresh(view);
				});
			}) }, _('Save'))
		])
	]);
}

function summaryTable(next, current) {
	return E('table', { 'class': 'pikabox-table' }, [
		E('thead', {}, E('tr', {}, [ E('th', {}, ''), E('th', {}, _('Now')), E('th', {}, _('After import')) ])),
		E('tbody', {}, SUMMARY_LABELS.map(function(pair) {
			var a = (current || {})[pair[0]] || 0, b = (next || {})[pair[0]] || 0;
			return E('tr', {}, [
				E('td', {}, [ pair[1] ]),
				E('td', {}, [ String(a) ]),
				E('td', {}, [ E('strong', { 'style': a !== b ? 'color:#2271b1' : '' }, [ String(b) ]) ])
			]);
		}))
	]);
}

// startImport validates the file first (dry run) and shows what would change;
// nothing is replaced until the user confirms.
function startImport(view, name, text) {
	return callImport(text, true).then(function(preview) {
		if (!preview.ok) {
			theme.error(preview, _('This file is not a valid configuration'));
			return;
		}
		ui.showModal(_('Import %s?').format(name), [
			E('p', {}, _('This replaces the whole configuration — outbounds, subscriptions, groups, rules, routing, DNS, and network settings. The current one is saved as a backup first. Whether the proxy is enabled or paused stays as it is on this router.')),
			summaryTable(preview.summary, preview.current),
			E('div', { 'class': 'right' }, [
				E('button', { 'class': 'btn cbi-button', 'click': ui.hideModal }, _('Cancel')),
				' ',
				E('button', { 'class': 'btn cbi-button cbi-button-negative', 'click': ui.createHandlerFn(view, function() {
					return callImport(text, false).then(function(result) {
						if (!theme.result(result, _('Configuration imported. Previous one saved as %s.').format(result.backup || '')))
							return;
						ui.hideModal();
						return theme.refresh(view);
					});
				}) }, _('Replace configuration'))
			])
		]);
	});
}

function readFile(file) {
	return new Promise(function(resolve, reject) {
		var reader = new FileReader();
		reader.onload = function() { resolve(reader.result); };
		reader.onerror = function() { reject(reader.error); };
		reader.readAsText(file);
	});
}

function renderBackup(view, data) {
	var fileInput = E('input', { 'type': 'file', 'accept': '.uci,.txt,.conf,text/plain', 'style': 'display:none' });
	var pick = function(file) {
		if (!file)
			return;
		if (file.size > 2 * 1024 * 1024) {
			theme.error(null, _('That file is larger than 2 MB — it is not a configuration export.'));
			return;
		}
		readFile(file).then(function(text) { return startImport(view, file.name, text); });
	};
	fileInput.addEventListener('change', function() { pick(fileInput.files[0]); fileInput.value = ''; });

	var drop = E('div', { 'class': 'pkb-drop', 'tabindex': '0', 'role': 'button', 'aria-label': _('Choose a configuration file to import') }, [
		E('strong', {}, [ _('Drop an exported .uci file here') ]),
		E('span', {}, [ _('or click to choose one — you will see what changes before anything is replaced') ])
	]);
	drop.addEventListener('click', function() { fileInput.click(); });
	drop.addEventListener('keydown', function(ev) { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); fileInput.click(); } });
	drop.addEventListener('dragover', function(ev) { ev.preventDefault(); drop.classList.add('pkb-over'); });
	drop.addEventListener('dragleave', function() { drop.classList.remove('pkb-over'); });
	drop.addEventListener('drop', function(ev) {
		ev.preventDefault();
		drop.classList.remove('pkb-over');
		pick(ev.dataTransfer.files[0]);
	});

	var backups = data.backups || [];
	return E('div', { 'class': 'pikabox-section' }, [
		E('h3', {}, [ _('Backup & restore') ]),
		E('div', { 'class': 'pkb-backup' }, [
			E('div', { 'class': 'pkb-backup-export' }, [
				E('div', { 'class': 'pikabox-row-label' }, [ _('Export') ]),
				E('p', { 'class': 'pikabox-hint' }, _('Downloads the full configuration as a UCI file. It contains server passwords, UUIDs and subscription URLs — store it like a password.')),
				E('button', { 'class': 'btn cbi-button cbi-button-action', 'click': ui.createHandlerFn(view, function() {
					return callExport().then(function(result) {
						if (theme.result(result))
							download(result.filename, result.text);
					});
				}) }, _('Download configuration'))
			]),
			E('div', {}, [
				E('div', { 'class': 'pikabox-row-label', 'style': 'margin-bottom:8px' }, [ _('Import') ]),
				drop,
				fileInput
			])
		]),
		backups.length ? E('div', {}, [
			E('div', { 'class': 'pikabox-row-label', 'style': 'margin:6px 0' }, [ _('Saved before earlier imports') ]),
			E('table', { 'class': 'pikabox-table' }, [
				E('tbody', {}, backups.map(function(b) {
					return E('tr', {}, [
						E('td', { 'class': 'pikabox-mono' }, [ b.name ]),
						E('td', {}, [ new Date(b.time).toLocaleString() ]),
						E('td', {}, E('div', { 'class': 'pikabox-actions' }, [
							E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() {
								return callBackup(b.name).then(function(result) { if (theme.result(result)) download(result.filename, result.text); });
							}) }, _('Download')),
							E('button', { 'class': 'btn cbi-button', 'click': ui.createHandlerFn(view, function() {
								return callBackup(b.name).then(function(result) { if (theme.result(result)) return startImport(view, b.name, result.text); });
							}) }, _('Restore…'))
						]))
					]);
				}))
			])
		]) : ''
	]);
}

return view.extend({
	handleSaveApply: null,
	handleSave: null,
	handleReset: null,

	load: function() {
		return callSettings();
	},

	render: function(data) {
		var view = this;
		data = data || {};
		var s = data.settings || {};
		var v = data.versions || {};
		theme.inject();

		return E('div', { 'class': 'pikabox-page narrow' }, [
			settingsSection(view, _('General'), [
				row(_('Log level'), _('sing-box log verbosity (Dashboard → Logs). info is a good default; debug is very chatty.'),
					theme.select('log_level', LOG_LEVELS, s.log_level || 'info')),
				row(_('Re-test interval'), _('How often every outbound and subscription server is delay-tested and each subscription re-picks its fastest servers. Minimum 1m.'),
					input('probe_interval', s.probe_interval, { 'placeholder': '10m', 'style': 'max-width:120px' })),
				row(_('Test URL'), _('Fetched through each outbound to measure latency. It must answer quickly with a small response (204 is ideal).'),
					input('test_url', s.test_url, { 'placeholder': 'https://www.gstatic.com/generate_204' })),
				row(_('Bulk test URL'), _('Downloaded through outbounds in use (about 256 KB, at most hourly each) to catch tunnels that pass latency tests but freeze on real traffic. Stalled servers are skipped by fastest/fallback picks. Type off to disable.'),
					input('bulk_test_url', s.bulk_test_url, { 'placeholder': 'https://speed.cloudflare.com/__down?bytes=262144' }))
			], [ 'log_level', 'probe_interval', 'test_url', 'bulk_test_url' ]),

			settingsSection(view, _('Ports & paths'), [
				row(_('Proxy listen address'), _('Mixed HTTP/SOCKS proxy inbound. 0.0.0.0 lets LAN devices use it directly; 127.0.0.1 keeps it on the router.'),
					input('mixed_listen', s.mixed_listen, { 'placeholder': '0.0.0.0', 'style': 'max-width:180px' })),
				row(_('Proxy port'), '', input('mixed_port', s.mixed_port, { 'type': 'number', 'min': '1', 'max': '65535', 'style': 'max-width:120px' })),
				row(_('Transparent proxy port'), _('Where the firewall sends TProxy traffic.'), input('tproxy_port', s.tproxy_port, { 'type': 'number', 'min': '1', 'max': '65535', 'style': 'max-width:120px' })),
				row(_('DNS capture port'), _('Where captured DNS queries are answered when DNS hijack is on.'), input('dns_port', s.dns_port, { 'type': 'number', 'min': '1', 'max': '65535', 'style': 'max-width:120px' })),
				row(_('Controller API'), _('sing-box API used for live connections, group switching and fallback. Keep it on 127.0.0.1; empty disables those features.'),
					input('api_listen', s.api_listen, { 'placeholder': '127.0.0.1:9090', 'style': 'max-width:180px' })),
				row(_('sing-box binary'), '', input('sing_box_bin', s.sing_box_bin, { 'placeholder': '/usr/bin/sing-box' }))
			], [ 'mixed_listen', 'mixed_port', 'tproxy_port', 'dns_port', 'api_listen', 'sing_box_bin' ]),

			renderBackup(view, data),

			E('div', { 'class': 'pikabox-section' }, [
				E('h3', {}, [ _('About') ]),
				E('div', { 'class': 'pikabox-grid' }, [
					E('span', { 'class': 'pikabox-label' }, [ 'sing-box' ]), E('span', {}, theme.text(v.sing_box)),
					E('span', { 'class': 'pikabox-label' }, [ 'OpenWrt' ]), E('span', {}, theme.text(v.openwrt))
				])
			])
		]);
	}
});
