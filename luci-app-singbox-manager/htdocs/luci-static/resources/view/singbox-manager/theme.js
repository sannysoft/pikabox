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
	'.singbox-manager-hero-facts{display:grid;grid-template-columns:minmax(90px,.8fr) minmax(130px,170px) minmax(180px,2fr) minmax(100px,1fr);gap:12px 20px;flex:1 1 420px;min-width:0}',
	'.singbox-manager-hero-facts>div{min-width:0}',
	'.singbox-manager-hero-facts select{display:block;width:100%;min-width:0;max-width:100%;box-sizing:border-box;margin:0}',
	'.sbm-oneline{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
	'.sbm-actions{display:flex;align-items:center;gap:10px}',
	'.sbm-icon-group{display:inline-flex;border:1px solid var(--border-color-medium);border-radius:8px;overflow:hidden}',
	'.sbm-icon-btn{display:grid!important;place-items:center;width:38px;height:38px;padding:0!important;margin:0!important;border:0!important;border-radius:0!important;background:var(--background-color-high)!important;color:var(--text-color-medium)!important}',
	'.sbm-icon-btn+.sbm-icon-btn{border-left:1px solid var(--border-color-medium)!important}',
	'.sbm-icon-btn:hover:not([disabled]),.sbm-icon-btn:focus-visible{background:var(--background-color-low)!important;color:var(--text-color-high)!important}',
	'.sbm-icon-btn:focus-visible{outline:2px solid #2271b1;outline-offset:-2px}',
	'.sbm-icon-btn[disabled]{opacity:.4;cursor:not-allowed}',
	'.sbm-icon-btn svg{width:18px;height:18px;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}',
	'.sbm-icon-btn.spinning svg{animation:sbm-spin 1s linear infinite}',
	'@keyframes sbm-spin{to{transform:rotate(360deg)}}',
	'@media(max-width:900px){.singbox-manager-hero-facts{grid-template-columns:repeat(2,minmax(0,1fr))}}',
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

	/* ---- dashboard: exit path ---- */
	'.sbm-path{display:grid;grid-template-columns:minmax(0,1.6fr) minmax(240px,1fr);gap:16px;align-items:start}',
	'.sbm-hops{display:flex;flex-wrap:wrap;align-items:center;gap:8px}',
	'.sbm-hop{display:grid;gap:3px;padding:8px 12px;border:1px solid var(--border-color-medium);border-radius:10px;background:var(--background-color-high);min-width:0}',
	'.sbm-hop .sbm-pill{justify-self:start}',
	'.sbm-hop-meta{font-size:11px;color:var(--text-color-medium);white-space:nowrap}',
	'.sbm-hop.sbm-leaf{border-color:#2271b1;box-shadow:0 0 0 1px #2271b1 inset}',
	'.sbm-hop-arrow{color:var(--text-color-medium);font-size:16px}',
	'.sbm-hopwrap{display:inline-flex;align-items:center;gap:8px;min-width:0}',
	'.sbm-hop-start{font-size:12px;color:var(--text-color-medium)}',
	'.sbm-timeline{display:grid;gap:0;margin:0;padding:0;list-style:none;border-left:2px solid var(--border-color-medium)}',
	'.sbm-timeline li{position:relative;padding:4px 0 8px 14px;font-size:12px;line-height:1.35}',
	'.sbm-timeline li:before{content:"";position:absolute;left:-6px;top:8px;width:10px;height:10px;border-radius:50%;background:var(--background-color-high);border:2px solid #9aa0a6}',
	'.sbm-timeline li.sbm-ev-switch:before{border-color:#5b4bd6}',
	'.sbm-timeline li.sbm-ev-select:before{border-color:#0d7f86}',
	'.sbm-timeline li.sbm-ev-error:before{border-color:#b42318;background:#b42318}',
	'.sbm-timeline time{display:block;font-size:11px;color:var(--text-color-medium);font-variant-numeric:tabular-nums}',

	/* ---- dashboard: health board ---- */
	'.sbm-warnings{display:grid;gap:8px}',
	'.sbm-warning{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:4px 12px;align-items:center;padding:10px 12px;border-radius:10px;border-left:4px solid #b45309;background:rgba(180,83,9,.08);font-size:12px}',
	'.sbm-warning.sbm-error{border-left-color:#b42318;background:rgba(180,35,24,.08)}',
	'.sbm-warning strong{font-size:13px}',
	'.sbm-warning span{grid-column:1;color:var(--text-color-medium);overflow-wrap:anywhere}',
	'.sbm-warning a{grid-row:1/3;grid-column:2;font-size:12px;white-space:nowrap}',
	'.sbm-board{display:grid;grid-template-columns:repeat(auto-fill,minmax(230px,1fr));gap:10px}',
	'.sbm-card{display:grid;gap:6px;padding:12px;border:1px solid var(--border-color-medium);border-radius:12px;background:var(--background-color-high);min-width:0}',
	'.sbm-card-head{display:flex;align-items:center;justify-content:space-between;gap:8px;min-width:0}',
	'.sbm-card-ms{font-size:20px;font-weight:700;font-variant-numeric:tabular-nums;line-height:1}',
	'.sbm-card-ms small{font-size:11px;font-weight:600;margin-left:2px;color:var(--text-color-medium)}',
	'.sbm-card-ms.down{color:#b42318;font-size:15px}',
	'.sbm-card-ms.idle{color:var(--text-color-medium);font-size:15px}',
	'.sbm-card-foot{display:flex;justify-content:space-between;gap:8px;font-size:11px;color:var(--text-color-medium);min-width:0}',
	'.sbm-card-foot span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}',
	'.sbm-spark{height:28px}',
	'.sbm-spark svg{display:block;width:100%;height:100%}',
	'.sbm-card.sbm-down{border-color:rgba(180,35,24,.45)}',

	/* ---- dashboard: connections ---- */
	'.sbm-conn-tools{display:flex;gap:12px;align-items:center;justify-content:space-between;flex-wrap:wrap}',
	'.sbm-conn-stats{display:flex;gap:14px;font-size:12px;color:var(--text-color-medium);font-variant-numeric:tabular-nums}',
	'.sbm-conn-wrap{max-height:420px;overflow:auto;border:1px solid var(--border-color-medium);border-radius:10px}',
	'.sbm-conn{width:100%;border-collapse:collapse;font-size:12px}',
	'.sbm-conn th{position:sticky;top:0;z-index:1;background:var(--background-color-high);font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);font-weight:600;text-align:left;padding:8px 10px;border-bottom:1px solid var(--border-color-medium);white-space:nowrap}',
	'.sbm-conn td{padding:6px 10px;border-bottom:1px solid var(--border-color-medium);vertical-align:middle}',
	'.sbm-conn tbody tr:last-child td{border-bottom:0}',
	'.sbm-conn td.sbm-num{text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}',
	'.sbm-conn th.sbm-num{text-align:right}',
	'.sbm-conn .sbm-host{font-family:monospace;max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}',
	'.sbm-conn .sbm-sub{display:block;font-size:11px;color:var(--text-color-medium)}',
	'.sbm-conn .sbm-chain{display:flex;gap:4px;align-items:center;flex-wrap:wrap}',
	'.sbm-conn .sbm-chain .sbm-pill{padding:3px 8px 3px 6px;font-size:11px}',
	'.sbm-conn tr.sbm-hot td{background:rgba(34,113,177,.06)}',
	'@media(max-width:900px){.sbm-path{grid-template-columns:1fr}}',

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
	/* ---- kind pills: what a target is, at a glance ---- */
	'.sbm-pill{display:inline-flex;align-items:center;gap:6px;max-width:100%;font-size:12px;line-height:1;padding:5px 10px 5px 8px;border-radius:999px;border:1px solid transparent;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
	'.sbm-pill:before{content:"";flex:0 0 auto;width:7px;height:7px;border-radius:50%;background:currentColor}',
	'.sbm-pill small{font-size:10px;letter-spacing:.06em;text-transform:uppercase;opacity:.7}',
	'.sbm-k-group{color:#5b4bd6;background:rgba(91,75,214,.10);border-color:rgba(91,75,214,.25)}',
	'.sbm-k-subscription{color:#0d7f86;background:rgba(13,127,134,.10);border-color:rgba(13,127,134,.25)}',
	'.sbm-k-outbound{color:#4a5568;background:rgba(74,85,104,.10);border-color:rgba(74,85,104,.22)}',
	'.sbm-k-direct{color:#0f7a39;background:rgba(15,122,57,.10);border-color:rgba(15,122,57,.25)}',
	'.sbm-k-block{color:#b42318;background:rgba(180,35,24,.08);border-color:rgba(180,35,24,.25)}',
	'.sbm-k-rule{color:#8a5a00;background:rgba(214,139,0,.10);border-color:rgba(214,139,0,.28)}',
	'.sbm-k-missing{color:#b42318;border-style:dashed;border-color:#b42318}',
	'[data-darkmode="true"] .sbm-k-group{color:#a99cff}',
	'[data-darkmode="true"] .sbm-k-subscription{color:#5fd4db}',
	'[data-darkmode="true"] .sbm-k-outbound{color:#b8c2d3}',
	'[data-darkmode="true"] .sbm-k-direct{color:#5fd08f}',
	'[data-darkmode="true"] .sbm-k-block{color:#ff8a80}',
	'[data-darkmode="true"] .sbm-k-rule{color:#f5c15a}',

	/* ---- draggable lists (members, routes) ---- */
	'.sbm-list{display:grid;gap:6px;margin:0;padding:0;list-style:none}',
	'.sbm-item{display:grid;grid-template-columns:28px minmax(0,1fr) auto;align-items:center;gap:10px;padding:8px 10px;border:1px solid var(--border-color-medium);border-radius:10px;background:var(--background-color-high);transition:box-shadow .15s,border-color .15s}',
	'.sbm-item:hover{border-color:var(--border-color-strong,#9aa0a6)}',
	'.sbm-item.sbm-off{opacity:.55}',
	'.sbm-grip{display:grid;place-items:center;width:28px;height:28px;padding:0;border:0;border-radius:6px;background:transparent;color:var(--text-color-medium);cursor:grab;touch-action:none}',
	'.sbm-grip:hover,.sbm-grip:focus-visible{background:var(--background-color-low);color:var(--text-color-high)}',
	'.sbm-grip:focus-visible{outline:2px solid #2271b1;outline-offset:1px}',
	'.sbm-grip svg{width:12px;height:16px;fill:currentColor}',
	'.sbm-ghost{opacity:.35;border-style:dashed}',
	'.sbm-chosen{box-shadow:0 8px 24px rgba(0,0,0,.18);border-color:#2271b1}',
	'.sbm-drag{cursor:grabbing}',
	'.sbm-item-main{display:flex;align-items:center;gap:10px;min-width:0}',
	'.sbm-item-meta{font-size:12px;color:var(--text-color-medium);white-space:nowrap}',
	'.sbm-x{width:28px;height:28px;padding:0;border:0;border-radius:6px;background:transparent;color:var(--text-color-medium);font-size:18px;line-height:1;cursor:pointer}',
	'.sbm-x:hover,.sbm-x:focus-visible{background:rgba(180,35,24,.10);color:#b42318}',
	'.sbm-add{display:flex;gap:8px;align-items:center;margin-top:4px}',
	'.sbm-add select{flex:1 1 auto;min-width:0;margin:0}',
	'.sbm-empty{padding:14px;border:1px dashed var(--border-color-medium);border-radius:10px;text-align:center;font-size:12px;color:var(--text-color-medium)}',
	'.sbm-pos{font-size:11px;font-variant-numeric:tabular-nums;color:var(--text-color-medium);min-width:2.2em}',

	/* ---- route rows: [grip] [on] when <rule group> → <target> ---- */
	'.sbm-route{grid-template-columns:28px 44px minmax(0,1fr) 20px minmax(0,1fr) 28px}',
	'.sbm-route select,.sbm-final select{width:100%;min-width:0;margin:0}',
	'.sbm-arrow{color:var(--text-color-medium);text-align:center}',
	'.sbm-final{display:grid;grid-template-columns:28px 44px minmax(0,1fr) 20px minmax(0,1fr) 28px;align-items:center;gap:10px;padding:10px;border-radius:10px;background:var(--background-color-low)}',
	'.sbm-final strong{font-size:13px}',
	'.sbm-switch{position:relative;display:inline-block;width:38px;height:22px}',
	'.sbm-switch input{position:absolute;opacity:0;width:0;height:0}',
	'.sbm-switch span{position:absolute;inset:0;border-radius:22px;background:var(--border-color-medium);transition:.2s;cursor:pointer}',
	'.sbm-switch span:before{content:"";position:absolute;width:16px;height:16px;left:3px;top:3px;border-radius:50%;background:#fff;transition:.2s}',
	'.sbm-switch input:checked+span{background:#2271b1}',
	'.sbm-switch input:checked+span:before{transform:translateX(16px)}',
	'.sbm-switch input:focus-visible+span{outline:2px solid #2271b1;outline-offset:2px}',
	'select.sbm-target{border-left-width:4px!important}',
	'select.sbm-target[data-kind="group"]{border-left-color:#5b4bd6!important}',
	'select.sbm-target[data-kind="subscription"]{border-left-color:#0d7f86!important}',
	'select.sbm-target[data-kind="outbound"]{border-left-color:#4a5568!important}',
	'select.sbm-target[data-kind="direct"]{border-left-color:#0f7a39!important}',
	'select.sbm-target[data-kind="block"]{border-left-color:#b42318!important}',
	'select.sbm-rulegroup{border-left:4px solid #d68b00!important}',
	'.sbm-dirty{display:none;font-size:12px;color:#b45309}',
	'.sbm-dirty.on{display:inline}',

	/* ---- rule table editor ---- */
	'.sbm-namebar{display:flex;gap:16px;align-items:flex-end;flex-wrap:wrap}',
	'.sbm-namebar>label:first-child{flex:1 1 260px}',
	'.sbm-namebar input.cbi-input-text{width:100%;box-sizing:border-box}',
	'.sbm-namebar .singbox-manager-check{margin:0 0 8px}',
	'.sbm-panel{display:flex;flex-direction:column;border:1px solid var(--border-color-medium);border-radius:10px;background:var(--background-color-low);overflow:hidden}',
	'.sbm-panel-head,.sbm-panel-foot{display:flex;gap:10px;align-items:center;justify-content:space-between;padding:8px 12px;background:var(--background-color-high)}',
	'.sbm-panel-head{border-bottom:1px solid var(--border-color-medium)}',
	'.sbm-panel-foot{border-top:1px solid var(--border-color-medium);justify-content:flex-start}',
	'.sbm-panel-head,.sbm-panel-foot{color:var(--text-color-high);font-weight:400}',
	'.sbm-panel-title{font-size:13px;font-weight:600;color:var(--text-color-high)}',
	'.sbm-panel-title .sbm-count{display:inline-block;margin-left:4px;padding:1px 8px;border-radius:999px;background:var(--background-color-low);font-weight:600}',
	'.sbm-panel-body{max-height:44vh;overflow:auto}',
	'.sbm-search{position:relative;display:block;width:220px;margin:0}',
	'.sbm-search:before{content:"";position:absolute;left:10px;top:50%;width:9px;height:9px;margin-top:-6px;border:1.5px solid var(--text-color-medium);border-radius:50%;pointer-events:none}',
	'.sbm-search:after{content:"";position:absolute;left:19px;top:50%;width:5px;height:1.5px;margin-top:3px;background:var(--text-color-medium);transform:rotate(45deg);pointer-events:none}',
	'.sbm-search input{width:100%;height:30px;box-sizing:border-box;margin:0;padding:0 10px 0 30px;border:1px solid var(--border-color-medium);border-radius:999px;background:var(--background-color-low);color:inherit;font-size:12px}',
	'.sbm-search input:focus{outline:none;border-color:#2271b1;box-shadow:0 0 0 2px rgba(34,113,177,.2)}',
	'.sbm-rules{width:100%;border-collapse:collapse}',
	'.sbm-rules thead th{position:sticky;top:0;z-index:1;background:var(--background-color-low);font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);font-weight:600;text-align:left;padding:6px 12px;border-bottom:1px solid var(--border-color-medium)}',
	'.sbm-rules td{padding:5px 12px;vertical-align:middle;border-bottom:1px solid var(--border-color-medium)}',
	'.sbm-rules tbody tr:last-child td{border-bottom:0}',
	'.sbm-rules td:first-child{width:180px;padding-right:4px}',
	'.sbm-rules td:last-child{width:28px;padding-left:0}',
	'.sbm-rules select,.sbm-rules input{width:100%;box-sizing:border-box;margin:0;font-family:monospace;font-size:12px}',
	'.sbm-rules tr.sbm-invalid input{border-color:#b42318}',
	'@media(max-width:600px){.sbm-search{width:100%}.sbm-panel-head{flex-wrap:wrap}.sbm-rules td:first-child{width:130px}}',
	'.sbm-count{font-size:12px;color:var(--text-color-medium)}',
	'.modal:has(.sbm-wide){width:min(880px,94vw)!important;max-width:none!important}',
	'.singbox-manager-modal-form.sbm-wide{min-width:0;width:100%}',

	/* own chevron: the native arrow sits against the border in Chrome/macOS */
	'.singbox-manager-page select,.singbox-manager-dashboard select,.modal .singbox-manager-modal-form select{-webkit-appearance:none;appearance:none;padding-right:30px!important;background-image:url(\"data:image/svg+xml,%3Csvg xmlns=%27http://www.w3.org/2000/svg%27 width=%2710%27 height=%276%27 viewBox=%270 0 10 6%27%3E%3Cpath d=%27M1 1l4 4 4-4%27 fill=%27none%27 stroke=%27%23888%27 stroke-width=%271.5%27 stroke-linecap=%27round%27 stroke-linejoin=%27round%27/%3E%3C/svg%3E\");background-repeat:no-repeat;background-position:right 10px center;background-size:10px 6px}',
	'@media(prefers-reduced-motion:reduce){.sbm-item,.sbm-switch span,.sbm-switch span:before{transition:none}.sbm-icon-btn.spinning svg{animation:none}}',
	'@media(max-width:760px){.sbm-route,.sbm-final{grid-template-columns:28px 44px minmax(0,1fr) 28px}.sbm-route .sbm-arrow,.sbm-final .sbm-arrow{display:none}.sbm-route>select:nth-of-type(2),.sbm-final>select{grid-column:3}}',
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

// Single in-flight load of the bundled SortableJS.
var sortableLoading = null;

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
	modal: function(view, title, body, save, successText, wide) {
		var self = this;
		ui.showModal(title, [
			E('div', { 'class': 'singbox-manager-modal-form' + (wide ? ' sbm-wide' : '') }, body),
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

	// pill renders a target as a coloured, labelled chip so its kind
	// (group / subscription / outbound / direct / block) reads at a glance.
	pill: function(name, kind, title) {
		var cls = kind === 'builtin' ? (name === 'block' ? 'block' : 'direct') : (kind || 'missing');
		return E('span', { 'class': 'sbm-pill sbm-k-' + cls, 'title': title || '' }, [
			String(name),
			(kind && kind !== 'builtin') ? E('small', {}, [ kind === 'subscription' ? _('sub') : kind ]) : ''
		]);
	},

	grip: function() {
		var btn = E('button', { 'type': 'button', 'class': 'sbm-grip', 'aria-label': _('Drag to reorder; Alt+Up/Down moves') });
		btn.innerHTML = '<svg viewBox="0 0 12 16" aria-hidden="true"><circle cx="3" cy="3" r="1.5"/><circle cx="9" cy="3" r="1.5"/><circle cx="3" cy="8" r="1.5"/><circle cx="9" cy="8" r="1.5"/><circle cx="3" cy="13" r="1.5"/><circle cx="9" cy="13" r="1.5"/></svg>';
		// Keyboard reordering, so drag & drop is never the only way.
		btn.addEventListener('keydown', function(ev) {
			if (!ev.altKey || (ev.key !== 'ArrowUp' && ev.key !== 'ArrowDown'))
				return;
			ev.preventDefault();
			var item = btn.closest('[data-sortable-item]');
			var sibling = ev.key === 'ArrowUp' ? item.previousElementSibling : item.nextElementSibling;
			if (!sibling || !sibling.hasAttribute('data-sortable-item'))
				return;
			item.parentNode.insertBefore(item, ev.key === 'ArrowUp' ? sibling : sibling.nextSibling);
			btn.focus();
			item.parentNode.dispatchEvent(new CustomEvent('sbm-reorder', { bubbles: true }));
		});
		return btn;
	},

	// sortable makes a list's [data-sortable-item] children draggable by
	// their .sbm-grip, loading the bundled SortableJS once. onChange runs
	// after any reorder (drag or keyboard).
	sortable: function(list, onChange) {
		list.addEventListener('sbm-reorder', function() { if (onChange) onChange(); });
		var attach = function() {
			window.Sortable.create(list, {
				handle: '.sbm-grip',
				draggable: '[data-sortable-item]',
				animation: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 160,
				ghostClass: 'sbm-ghost',
				chosenClass: 'sbm-chosen',
				dragClass: 'sbm-drag',
				onEnd: function() { if (onChange) onChange(); }
			});
		};
		if (window.Sortable)
			return attach();
		if (!sortableLoading) {
			sortableLoading = new Promise(function(resolve, reject) {
				var script = document.createElement('script');
				script.src = L.resource('singbox-manager/Sortable.min.js');
				script.onload = resolve;
				script.onerror = reject;
				document.head.appendChild(script);
			});
		}
		sortableLoading.then(attach);
	},

	// orderedList is a draggable list of target IDs (group members).
	// options: [{id, name, kind, latency_ms?}]; read back with orderedValues.
	orderedList: function(values, options) {
		var self = this;
		var byId = {};
		options.forEach(function(o) { byId[o.id] = o; });
		var list = E('ol', { 'class': 'sbm-list', 'data-ordered': '1' });
		var empty = E('div', { 'class': 'sbm-empty' }, [ _('No members yet — add one below.') ]);
		var renumber = function() {
			var items = list.querySelectorAll('[data-sortable-item]');
			items.forEach(function(el, i) { el.querySelector('.sbm-pos').textContent = '#' + (i + 1); });
			empty.style.display = items.length ? 'none' : '';
		};
		var item = function(id) {
			var o = byId[id] || { id: id, name: id, kind: '' };
			var el = E('li', { 'class': 'sbm-item', 'data-sortable-item': '1', 'data-value': id }, [
				self.grip(),
				E('div', { 'class': 'sbm-item-main' }, [
					E('span', { 'class': 'sbm-pos' }),
					self.pill(o.name || o.id, o.kind, o.kind ? '' : _('Missing — it was deleted'))
				]),
				E('button', { 'type': 'button', 'class': 'sbm-x', 'aria-label': _('Remove %s').format(o.name || o.id), 'click': function() {
					el.remove();
					renumber();
				} }, [ '×' ])
			]);
			return el;
		};
		values.forEach(function(id) { list.appendChild(item(id)); });
		var picker = self.select('_add', [ { id: '', name: _('Add member…') } ].concat(options.map(function(o) {
			return { id: o.id, name: '%s — %s'.format(o.name || o.id, o.kind === 'builtin' ? _('built-in') : o.kind) };
		})), '', {
			'change': function() {
				if (!picker.value)
					return;
				list.appendChild(item(picker.value));
				picker.value = '';
				renumber();
			}
		});
		self.sortable(list, renumber);
		renumber();
		return E('div', {}, [ list, empty, E('div', { 'class': 'sbm-add' }, [ picker ]) ]);
	},

	orderedValues: function(root) {
		var list = root.querySelector('[data-ordered]');
		return list ? Array.prototype.map.call(list.querySelectorAll('[data-value]'), function(el) { return el.dataset.value; }) : [];
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
