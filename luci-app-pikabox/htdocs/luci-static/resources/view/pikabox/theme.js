'use strict';
'require baseclass';
'require ui';

// Shared visual theme for every Pika-box view. The stylesheet is the
// single source of truth for the card-based look modelled on the dashboard; it
// is injected once into <head> (keyed by id) so every page renders identically
// and re-renders (poll / refreshView) never duplicate it.
var CSS = [
	/* ---- page + section cards ---- */
	'.pikabox-page{display:grid;gap:16px}',
	'.pikabox-page.narrow{max-width:960px}',
	'.pikabox-dashboard{display:grid;gap:16px}',
	'.pikabox-live{display:grid;gap:16px}',
	'.pikabox-section{display:grid;gap:12px;border:1px solid var(--border-color-medium);border-radius:12px;padding:16px 20px;background:var(--background-color-high)}',
	'.pikabox-section>h3{margin:0;font-size:15px;font-weight:600}',
	'.pikabox-section-header{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}',
	'.pikabox-section-header h3{margin:0;font-size:15px;font-weight:600}',
	'.pikabox-hint{margin:0;font-size:12px;color:var(--text-color-medium)}',
	'.pikabox-inline-help{margin:0;font-size:12px;color:var(--text-color-medium)}',

	/* ---- badges / status ---- */
	'.pikabox-badge{font-size:11px;font-weight:600;padding:3px 10px;border-radius:999px;background:var(--border-color-medium);color:var(--text-color-medium);white-space:nowrap}',
	'.pikabox-badge.on{background:rgba(15,122,57,.15);color:#0f7a39}',
	'.pikabox-badge.off{background:rgba(180,35,24,.12);color:#b42318}',
	'.pikabox-badge.warn{background:rgba(180,83,9,.14);color:#b45309}',
	'.pikabox-status-ok{color:#0f7a39;font-weight:600}',
	'.pikabox-status-error{color:#b42318;font-weight:600}',

	/* ---- controls ---- */
	'.pikabox-controls{display:flex;gap:18px;flex-wrap:wrap}',
	'.pikabox-inline-control{display:flex;align-items:center;gap:10px;font-size:13px}',
	'.pikabox-filters{display:flex;gap:8px;flex-wrap:wrap}',
	'.pikabox-actions{display:flex;gap:6px;flex-wrap:wrap;align-items:center}',
	'.pikabox-toolbar{display:flex;gap:8px;flex-wrap:wrap}',

	/* ---- definition grid (label / value) ---- */
	'.pikabox-grid{display:grid;grid-template-columns:minmax(130px,190px) minmax(0,1fr);gap:10px 14px;align-items:start}',
	'.pikabox-label{font-size:12px;color:var(--text-color-medium);font-weight:600}',

	/* ---- setting rows (toggle list) ---- */
	'.pikabox-rows{display:grid;gap:0}',
	'.pikabox-row{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:12px 0;border-top:1px solid var(--border-color-medium)}',
	'.pikabox-row:first-child{border-top:0}',
	'.pikabox-row-label{font-size:13px;font-weight:600}',
	'.pikabox-row-hint{font-size:11px;color:var(--text-color-medium);margin-top:2px}',
	'.pikabox-row-control{flex:0 0 auto;display:flex;align-items:center}',
	'.pikabox-row-control input.cbi-input-text{min-width:min(280px,46vw)}',
	'.pikabox-meta{display:flex;gap:16px;flex-wrap:wrap;font-size:11px;color:var(--text-color-medium);padding-top:4px}',

	/* ---- toggle switch ---- */
	'.pikabox-toggle{position:relative;display:inline-block;width:42px;height:24px;flex:0 0 auto}',
	'.pikabox-toggle input{position:absolute;opacity:0;width:0;height:0}',
	'.pikabox-toggle span{position:absolute;inset:0;cursor:pointer;background:var(--border-color-medium);border-radius:24px;transition:.2s}',
	'.pikabox-toggle span:before{content:"";position:absolute;height:18px;width:18px;left:3px;top:3px;background:#fff;border-radius:50%;transition:.2s}',
	'.pikabox-toggle input:checked+span{background:#2271b1}',
	'.pikabox-toggle input:checked+span:before{transform:translateX(18px)}',

	/* ---- tables ---- */
	'.pikabox-table{width:100%;border-collapse:collapse;font-size:13px}',
	'.pikabox-table th,.pikabox-table td{padding:9px 10px;text-align:left;vertical-align:middle}',
	'.pikabox-table thead th{font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);font-weight:600;border-bottom:2px solid var(--border-color-medium);white-space:nowrap}',
	'.pikabox-table tbody td{border-bottom:1px solid var(--border-color-medium)}',
	'.pikabox-table tbody tr:last-child td{border-bottom:0}',
	'.pikabox-table tbody tr:hover td{background:var(--background-color-low)}',
	'.pikabox-table tbody tr:not(.pikabox-group-row) td[colspan]{text-align:center;color:var(--text-color-medium);padding:18px 10px}',
	'.pikabox-table th:last-child,.pikabox-table td:last-child{text-align:right}',
	'.pikabox-table .pikabox-actions{justify-content:flex-end}',
	'.pikabox-table tbody tr.pikabox-current td{background:rgba(34,113,177,.10)}',
	'.pikabox-table td.pikabox-wrap{white-space:normal;overflow-wrap:anywhere;max-width:280px}',
	'.pikabox-group-row td{background:var(--background-color-low);font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);text-align:left!important}',

	/* ---- modal forms ---- */
	'.pikabox-modal-form{display:grid;gap:12px;min-width:min(520px,90vw)}',
	'.pikabox-modal-form.wide{grid-template-columns:repeat(auto-fit,minmax(180px,1fr));min-width:min(760px,90vw)}',
	'.pikabox-modal-form label{display:grid;gap:4px;font-size:12px;color:var(--text-color-medium)}',
	'.pikabox-check{display:flex!important;align-items:center;gap:8px}',
	'.pikabox-modal-form.wide .pikabox-check{margin-top:20px}',
	'.pikabox-import-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}',
	'.pikabox-import-grid label{display:grid;gap:4px;font-size:12px;color:var(--text-color-medium)}',
	'.pikabox-import-link{grid-column:1/-1}',
	'.pikabox-import-link textarea{min-height:96px;resize:vertical}',
	'.pikabox-import-help{grid-column:1/-1;margin:0;font-size:12px;color:var(--text-color-medium)}',

	/* ---- per-device override editor ---- */
	'.pikabox-device-editor{display:grid;gap:10px;padding-top:12px;border-top:1px solid var(--border-color-medium)}',
	'.pikabox-device-editor>h4{margin:0;font-size:13px;font-weight:600}',
	'.pikabox-devices{display:grid;gap:6px;overflow-x:auto}',
	// Fixed last two tracks (toggle, Remove) so the fr columns resolve to the
	// same widths in the header row (whose trailing cells are empty) as in the
	// data rows — otherwise the columns would not line up.
	'.pikabox-device-row{display:grid;grid-template-columns:minmax(90px,1.1fr) minmax(140px,1.3fr) minmax(104px,1fr) minmax(104px,1fr) minmax(96px,.9fr) minmax(90px,.9fr) 48px 48px 96px;gap:8px;align-items:center}',
	'.pikabox-device-row input.cbi-input-text,.pikabox-device-row select{width:100%;box-sizing:border-box;min-width:0;margin:0}',
	'.pikabox-device-row .pikabox-toggle{justify-self:center}',
	'.pikabox-device-row .cbi-button-remove{justify-self:end;white-space:nowrap}',
	'.pikabox-device-head{font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);font-weight:600;border-bottom:1px solid var(--border-color-medium);padding-bottom:6px}',
	'.pikabox-device-head span:nth-child(7),.pikabox-device-head span:nth-child(8){text-align:center}',

	/* ---- device picker chips ---- */
	'.pikabox-devicepicker{display:grid;gap:6px;margin-top:-4px}',
	'.pikabox-devicepicker-label{font-size:11px;color:var(--text-color-medium)}',
	'.pikabox-chips{display:flex;gap:6px;flex-wrap:wrap}',
	'.pikabox-chip{font-size:11px;padding:2px 8px!important;border-radius:12px}',
	'.pikabox-tags{display:flex;gap:4px;flex-wrap:wrap}',
	'.pikabox-tag{font-size:11px;padding:1px 8px;border-radius:10px;border:1px solid var(--border-color-medium);white-space:nowrap}',

	/* ---- alerts + preformatted blocks ---- */
	'.pikabox-alert{display:flex;flex-wrap:wrap;gap:12px;align-items:center;justify-content:space-between;margin-bottom:12px;padding:12px 14px;border-radius:8px;border-left:4px solid #b91c1c;background:rgba(185,28,28,.10);font-size:13px}',
	'.pikabox-alert strong{display:block;margin-bottom:2px;color:#b91c1c}',
	'.pikabox-warning{padding:10px 12px;border-radius:8px;border-left:4px solid #b45309;background:rgba(180,83,9,.10);font-size:13px}',
	'.pikabox-preview{box-sizing:border-box;max-width:100%;min-height:60px;overflow:auto;padding:12px;border:1px solid var(--border-color-medium);border-radius:8px;background:var(--background-color-low);font:12px/1.5 monospace;white-space:pre}',
	'.pikabox-log{box-sizing:border-box;max-width:100%;min-height:360px;max-height:60vh;overflow:auto;padding:12px;border:1px solid var(--border-color-medium);border-radius:8px;background:var(--background-color-low);font:12px/1.5 monospace;white-space:pre-wrap;word-break:break-word}',
	'.pikabox-details{border:1px solid var(--border-color-medium);border-radius:8px;padding:0}',
	'.pikabox-details>summary{cursor:pointer;padding:10px 12px;font-size:13px;font-weight:600;color:var(--text-color-medium);list-style:none}',
	'.pikabox-details[open]>summary{border-bottom:1px solid var(--border-color-medium)}',
	'.pikabox-details>.pikabox-grid{padding:12px}',

	/* ---- dashboard hero ---- */
	'.pikabox-hero{display:flex;align-items:center;gap:20px;flex-wrap:wrap;border:1px solid var(--border-color-medium);border-radius:12px;padding:16px 20px;background:var(--background-color-high)}',
	'.pikabox-hero-status{display:flex;align-items:center;gap:14px;min-width:160px}',
	'.pikabox-dot{width:14px;height:14px;border-radius:50%;background:#9aa0a6;flex:0 0 auto;box-shadow:0 0 0 4px rgba(154,160,166,.18)}',
	'.pikabox-dot.on{background:#0f7a39;box-shadow:0 0 0 4px rgba(15,122,57,.18)}',
	'.pikabox-dot.idle{background:#d68b00;box-shadow:0 0 0 4px rgba(214,139,0,.18)}',
	'.pikabox-dot.off{background:#b42318;box-shadow:0 0 0 4px rgba(180,35,24,.18)}',
	'.pikabox-hero-state{font-size:22px;font-weight:700;line-height:1.1}',
	'.pikabox-hero-sub{font-size:12px;color:var(--text-color-medium)}',
	'.pikabox-hero-facts{display:grid;grid-template-columns:minmax(90px,.8fr) minmax(130px,170px) minmax(180px,2fr) minmax(100px,1fr);gap:12px 20px;flex:1 1 420px;min-width:0}',
	'.pikabox-hero-facts>div{min-width:0}',
	'.pikabox-hero-facts select{display:block;width:100%;min-width:0;max-width:100%;box-sizing:border-box;margin:0}',
	'.pkb-oneline{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
	'.pkb-actions{display:flex;align-items:center;gap:10px}',
	'.pkb-icon-group{display:inline-flex;border:1px solid var(--border-color-medium);border-radius:8px;overflow:hidden}',
	'.pkb-icon-btn{display:grid!important;place-items:center;width:38px;height:38px;padding:0!important;margin:0!important;border:0!important;border-radius:0!important;background:var(--background-color-high)!important;color:var(--text-color-medium)!important}',
	'.pkb-icon-btn+.pkb-icon-btn{border-left:1px solid var(--border-color-medium)!important}',
	'.pkb-icon-btn:hover:not([disabled]),.pkb-icon-btn:focus-visible{background:var(--background-color-low)!important;color:var(--text-color-high)!important}',
	'.pkb-icon-btn:focus-visible{outline:2px solid #2271b1;outline-offset:-2px}',
	'.pkb-icon-btn[disabled]{opacity:.4;cursor:not-allowed}',
	'.pkb-icon-btn svg{width:18px;height:18px;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}',
	'.pkb-icon-btn.spinning svg{animation:pkb-spin 1s linear infinite}',
	'@keyframes pkb-spin{to{transform:rotate(360deg)}}',
	'@media(max-width:900px){.pikabox-hero-facts{grid-template-columns:repeat(2,minmax(0,1fr))}}',
	'.pikabox-hero-facts>div{display:grid;gap:3px}',
	'.pikabox-hero-facts span{font-size:11px;color:var(--text-color-medium)}',
	'.pikabox-hero-facts strong{font-size:14px;overflow-wrap:anywhere}',
	'.pikabox-hero-facts strong.ok{color:#0f7a39}',
	'.pikabox-hero-facts strong.error{color:#b42318}',
	'.pikabox-hero-action{flex:0 0 auto}',
	'.pikabox-hero-action .btn{font-size:15px;padding:8px 26px}',

	/* ---- dashboard charts ---- */
	'.pikabox-charts{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:12px}',
	'.pikabox-chart{border:1px solid var(--border-color-medium);border-radius:12px;padding:14px;background:var(--background-color-high);display:grid;gap:8px}',
	'.pikabox-chart-head{display:flex;align-items:baseline;justify-content:space-between}',
	'.pikabox-chart-title{font-size:12px;color:var(--text-color-medium);font-weight:600}',
	'.pikabox-chart-rate{font-size:16px;font-weight:700}',
	'.pikabox-spark{height:72px}',
	'.pikabox-spark-empty{display:flex;align-items:center;justify-content:center;height:100%;font-size:12px;color:var(--text-color-medium)}',
	'.pikabox-chart-foot{font-size:11px;color:var(--text-color-medium)}',

	/* ---- dashboard metrics ---- */
	'.pikabox-metrics{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px}',
	'.pikabox-metric{border:1px solid var(--border-color-medium);border-radius:12px;padding:14px;background:var(--background-color-high)}',
	'.pikabox-metric-label{font-size:12px;color:var(--text-color-medium);margin-bottom:6px}',
	'.pikabox-metric-value{font-size:18px;font-weight:600;overflow-wrap:anywhere}',
	'.pikabox-metric-value.ok{color:#0f7a39}',
	'.pikabox-metric-value.error{color:#b42318}',
	'.pikabox-logs{display:grid;gap:12px}',

	/* ---- settings: backup & restore ---- */
	'.pkb-backup{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1.2fr);gap:20px;align-items:start}',
	'.pkb-backup-export{display:grid;gap:8px;justify-items:start}',
	'.pkb-backup-export p{margin:0}',
	'.pkb-drop{display:grid;gap:4px;place-items:center;text-align:center;padding:22px 16px;border:2px dashed var(--border-color-medium);border-radius:12px;background:var(--background-color-low);cursor:pointer;transition:border-color .15s,background .15s}',
	'.pkb-drop span{font-size:12px;color:var(--text-color-medium)}',
	'.pkb-drop:hover,.pkb-drop:focus-visible,.pkb-drop.pkb-over{border-color:#2271b1;background:rgba(34,113,177,.06);outline:none}',
	'@media(max-width:760px){.pkb-backup{grid-template-columns:1fr}}',

	/* ---- dashboard: brand banner ---- */
	'.pkb-banner{display:flex;align-items:center;gap:12px;padding:4px 2px}',
	'.pkb-banner img{flex:0 0 auto;width:44px;height:44px}',
	'.pkb-banner-text{display:grid;gap:1px;min-width:0}',
	'.pkb-banner-name{display:flex;align-items:baseline;gap:8px;font-size:20px;font-weight:800;letter-spacing:-.02em;line-height:1.1;color:var(--text-color-high)}',
	'.pkb-banner-ver{font-size:11px;font-weight:600;letter-spacing:.02em;color:var(--text-color-medium);font-variant-numeric:tabular-nums}',
	'.pkb-banner-tag{font-size:12px;color:var(--text-color-medium)}',

	/* ---- dashboard: exit path ---- */
	'.pkb-path{display:grid;grid-template-columns:minmax(0,1.6fr) minmax(240px,1fr);gap:16px;align-items:start}',
	'.pkb-hops{display:flex;flex-wrap:wrap;align-items:center;gap:8px}',
	'.pkb-hop{display:grid;gap:3px;padding:8px 12px;border:1px solid var(--border-color-medium);border-radius:10px;background:var(--background-color-high);min-width:0}',
	'.pkb-hop .pkb-pill{justify-self:start}',
	'.pkb-hop-meta{font-size:11px;color:var(--text-color-medium);white-space:nowrap}',
	'.pkb-hop.pkb-leaf{border-color:#2271b1;box-shadow:0 0 0 1px #2271b1 inset}',
	'.pkb-hop-arrow{color:var(--text-color-medium);font-size:16px}',
	'.pkb-hopwrap{display:inline-flex;align-items:center;gap:8px;min-width:0}',
	'.pkb-hop-start{font-size:12px;color:var(--text-color-medium)}',
	'.pkb-timeline{display:grid;gap:0;margin:0;padding:0;list-style:none;border-left:2px solid var(--border-color-medium)}',
	'.pkb-timeline li{position:relative;padding:4px 0 8px 14px;font-size:12px;line-height:1.35}',
	'.pkb-timeline li:before{content:"";position:absolute;left:-6px;top:8px;width:10px;height:10px;border-radius:50%;background:var(--background-color-high);border:2px solid #9aa0a6}',
	'.pkb-timeline li.pkb-ev-switch:before{border-color:#5b4bd6}',
	'.pkb-timeline li.pkb-ev-select:before{border-color:#0d7f86}',
	'.pkb-timeline li.pkb-ev-error:before{border-color:#b42318;background:#b42318}',
	'.pkb-timeline time{display:block;font-size:11px;color:var(--text-color-medium);font-variant-numeric:tabular-nums}',

	/* ---- dashboard: health board ---- */
	'.pkb-warnings{display:grid;gap:8px}',
	'.pkb-warning{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:4px 12px;align-items:center;padding:10px 12px;border-radius:10px;border-left:4px solid #b45309;background:rgba(180,83,9,.08);font-size:12px}',
	'.pkb-warning.pkb-error{border-left-color:#b42318;background:rgba(180,35,24,.08)}',
	'.pkb-warning strong{font-size:13px}',
	'.pkb-warning span{grid-column:1;color:var(--text-color-medium);overflow-wrap:anywhere}',
	'.pkb-warning a{grid-row:1/3;grid-column:2;font-size:12px;white-space:nowrap}',
	'.pkb-board{display:grid;grid-template-columns:repeat(auto-fill,minmax(230px,1fr));gap:10px}',
	'.pkb-card{display:grid;gap:6px;padding:12px;border:1px solid var(--border-color-medium);border-radius:12px;background:var(--background-color-high);min-width:0}',
	'.pkb-card-head{display:flex;align-items:center;justify-content:space-between;gap:8px;min-width:0}',
	'.pkb-card-ms{font-size:20px;font-weight:700;font-variant-numeric:tabular-nums;line-height:1}',
	'.pkb-card-ms small{font-size:11px;font-weight:600;margin-left:2px;color:var(--text-color-medium)}',
	'.pkb-card-ms.down{color:#b42318;font-size:15px}',
	'.pkb-card-ms.idle{color:var(--text-color-medium);font-size:15px}',
	'.pkb-card-foot{display:flex;justify-content:space-between;gap:8px;font-size:11px;color:var(--text-color-medium);min-width:0}',
	'.pkb-card-foot span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}',
	'.pkb-spark{height:28px}',
	'.pkb-spark svg{display:block;width:100%;height:100%}',
	'.pkb-card.pkb-down{border-color:rgba(180,35,24,.45)}',

	/* ---- dashboard: health graph (routes → groups → sources → servers) ---- */
	'.pkb-graph-scroll{overflow-x:auto;padding-bottom:4px}',
	'.pkb-graph{position:relative;display:flex;gap:56px;align-items:flex-start;min-width:max-content;padding:4px 2px}',
	'.pkb-graph>svg{position:absolute;inset:0;width:100%;height:100%;pointer-events:none;overflow:visible}',
	'.pkb-col{position:relative;z-index:1;display:flex;flex-direction:column;gap:10px;width:220px}',
	'.pkb-col:last-child{width:280px}',
	'.pkb-col-title{font-size:11px;font-weight:600;letter-spacing:.04em;text-transform:uppercase;color:var(--text-color-medium);margin-bottom:2px}',
	'.pkb-node{display:grid;gap:4px;padding:8px 10px;border:1px solid var(--border-color-medium);border-radius:10px;background:var(--background-color-high)}',
	'.pkb-node-head{display:flex;align-items:center;justify-content:space-between;gap:8px;min-width:0}',
	'.pkb-node-head .pkb-pill{min-width:0}',
	'.pkb-node-ms{font-size:13px;font-weight:700;font-variant-numeric:tabular-nums;white-space:nowrap}',
	'.pkb-node-ms.down{color:#b42318}',
	'.pkb-node-ms.idle{color:var(--text-color-medium);font-weight:600}',
	'.pkb-node-foot{font-size:11px;color:var(--text-color-medium);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
	'.pkb-node.pkb-down{border-color:rgba(180,35,24,.5)}',
	'.pkb-node.pkb-live{border-color:#2271b1;box-shadow:0 0 0 1px #2271b1 inset}',
	'.pkb-node.pkb-unused{opacity:.55;border-style:dashed}',
	'.pkb-node.pkb-rule{background:var(--background-color-low)}',
	'.pkb-node .pkb-spark{height:16px}',
	'.pkb-edge{fill:none;stroke:var(--border-color-strong,#b5bac1);stroke-width:1.2}',
	'.pkb-edge.pkb-on{stroke:#2271b1;stroke-width:2.4}',
	'.pkb-edge.pkb-dead{stroke:#d9534f;stroke-dasharray:4 4;opacity:.7}',
	'.pkb-graph-legend{display:flex;gap:16px;flex-wrap:wrap;font-size:11px;color:var(--text-color-medium)}',
	'.pkb-graph-legend i{display:inline-block;width:22px;height:0;margin-right:6px;vertical-align:middle;border-top:2px solid #2271b1}',
	'.pkb-graph-legend i.pkb-l-dead{border-top:2px dashed #d9534f}',
	'.pkb-graph-legend i.pkb-l-idle{border-top:1px solid #b5bac1}',

	/* ---- dashboard: connections ---- */
	'.pkb-conn-tools{display:flex;gap:12px;align-items:center;justify-content:space-between;flex-wrap:wrap}',
	'.pkb-conn-stats{display:flex;gap:14px;font-size:12px;color:var(--text-color-medium);font-variant-numeric:tabular-nums}',
	'.pkb-conn-wrap{max-height:420px;overflow:auto;border:1px solid var(--border-color-medium);border-radius:10px}',
	'.pkb-conn{width:100%;border-collapse:collapse;font-size:12px}',
	'.pkb-conn th{position:sticky;top:0;z-index:1;background:var(--background-color-high);font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);font-weight:600;text-align:left;padding:8px 10px;border-bottom:1px solid var(--border-color-medium);white-space:nowrap}',
	'.pkb-conn td{padding:6px 10px;border-bottom:1px solid var(--border-color-medium);vertical-align:middle}',
	'.pkb-conn tbody tr:last-child td{border-bottom:0}',
	'.pkb-conn td.pkb-num{text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}',
	'.pkb-conn th.pkb-num{text-align:right}',
	'.pkb-conn .pkb-host{font-family:monospace;max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}',
	'.pkb-conn .pkb-sub{display:block;font-size:11px;color:var(--text-color-medium)}',
	'.pkb-conn .pkb-chain{display:flex;gap:4px;align-items:center;flex-wrap:wrap}',
	'.pkb-conn .pkb-chain .pkb-pill{padding:3px 8px 3px 6px;font-size:11px}',
	'.pkb-conn tr.pkb-hot td{background:rgba(34,113,177,.06)}',
	'@media(max-width:900px){.pkb-path{grid-template-columns:1fr}}',

	/* ---- latency / ordered lists ---- */
	'.pikabox-ms{font-variant-numeric:tabular-nums;font-weight:600}',
	'.pikabox-ms.fast{color:#0f7a39}',
	'.pikabox-ms.mid{color:#b45309}',
	'.pikabox-ms.slow,.pikabox-ms.down{color:#b42318}',
	'.pikabox-ms.none{color:var(--text-color-medium);font-weight:400}',
	'.pikabox-members{display:flex;gap:6px;flex-wrap:wrap}',
	'.pikabox-member{font-size:12px;padding:2px 8px;border-radius:12px;border:1px solid var(--border-color-medium);white-space:nowrap}',
	'.pikabox-member.now{border-color:#2271b1;background:rgba(34,113,177,.12)}',
	'.pikabox-member.off{opacity:.5;text-decoration:line-through}',
	/* ---- kind pills: what a target is, at a glance ---- */
	'.pkb-pill{display:inline-flex;align-items:center;gap:6px;max-width:100%;font-size:12px;line-height:1;padding:5px 10px 5px 8px;border-radius:999px;border:1px solid transparent;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
	'.pkb-pill:before{content:"";flex:0 0 auto;width:7px;height:7px;border-radius:50%;background:currentColor}',
	'.pkb-pill small{font-size:10px;letter-spacing:.06em;text-transform:uppercase;opacity:.7}',
	'.pkb-k-group{color:#5b4bd6;background:rgba(91,75,214,.10);border-color:rgba(91,75,214,.25)}',
	'.pkb-k-subscription{color:#0d7f86;background:rgba(13,127,134,.10);border-color:rgba(13,127,134,.25)}',
	'.pkb-k-server{color:#0d7f86;background:transparent;border-color:rgba(13,127,134,.35);border-style:dashed}',
	'[data-darkmode="true"] .pkb-k-server{color:#5fd4db}',
	'.pkb-k-outbound{color:#4a5568;background:rgba(74,85,104,.10);border-color:rgba(74,85,104,.22)}',
	'.pkb-k-direct{color:#0f7a39;background:rgba(15,122,57,.10);border-color:rgba(15,122,57,.25)}',
	'.pkb-k-block{color:#b42318;background:rgba(180,35,24,.08);border-color:rgba(180,35,24,.25)}',
	'.pkb-k-rule{color:#8a5a00;background:rgba(214,139,0,.10);border-color:rgba(214,139,0,.28)}',
	'.pkb-k-missing{color:#b42318;border-style:dashed;border-color:#b42318}',
	'[data-darkmode="true"] .pkb-k-group{color:#a99cff}',
	'[data-darkmode="true"] .pkb-k-subscription{color:#5fd4db}',
	'[data-darkmode="true"] .pkb-k-outbound{color:#b8c2d3}',
	'[data-darkmode="true"] .pkb-k-direct{color:#5fd08f}',
	'[data-darkmode="true"] .pkb-k-block{color:#ff8a80}',
	'[data-darkmode="true"] .pkb-k-rule{color:#f5c15a}',

	/* ---- draggable lists (members, routes) ---- */
	'.pkb-list{display:grid;gap:6px;margin:0;padding:0;list-style:none}',
	'.pkb-item{display:grid;grid-template-columns:28px minmax(0,1fr) auto;align-items:center;gap:10px;padding:8px 10px;border:1px solid var(--border-color-medium);border-radius:10px;background:var(--background-color-high);transition:box-shadow .15s,border-color .15s}',
	'.pkb-item:hover{border-color:var(--border-color-strong,#9aa0a6)}',
	'.pkb-item.pkb-off{opacity:.55}',
	'.pkb-grip{display:grid;place-items:center;width:28px;height:28px;padding:0;border:0;border-radius:6px;background:transparent;color:var(--text-color-medium);cursor:grab;touch-action:none}',
	'.pkb-grip:hover,.pkb-grip:focus-visible{background:var(--background-color-low);color:var(--text-color-high)}',
	'.pkb-grip:focus-visible{outline:2px solid #2271b1;outline-offset:1px}',
	'.pkb-grip svg{width:12px;height:16px;fill:currentColor}',
	'.pkb-ghost{opacity:.35;border-style:dashed}',
	'.pkb-chosen{box-shadow:0 8px 24px rgba(0,0,0,.18);border-color:#2271b1}',
	'.pkb-drag{cursor:grabbing}',
	'.pkb-item-main{display:flex;align-items:center;gap:10px;min-width:0}',
	'.pkb-item-meta{font-size:12px;color:var(--text-color-medium);white-space:nowrap}',
	'.pkb-x{width:28px;height:28px;padding:0;border:0;border-radius:6px;background:transparent;color:var(--text-color-medium);font-size:18px;line-height:1;cursor:pointer}',
	'.pkb-x:hover,.pkb-x:focus-visible{background:rgba(180,35,24,.10);color:#b42318}',
	'.pkb-add{display:flex;gap:8px;align-items:center;margin-top:4px}',
	'.pkb-add select{flex:1 1 auto;min-width:0;margin:0}',
	'.pkb-empty{padding:14px;border:1px dashed var(--border-color-medium);border-radius:10px;text-align:center;font-size:12px;color:var(--text-color-medium)}',
	'.pkb-pos{font-size:11px;font-variant-numeric:tabular-nums;color:var(--text-color-medium);min-width:2.2em}',

	/* ---- route rows: [grip] [on] when <rule group> → <target> ---- */
	'.pkb-route{grid-template-columns:28px 44px minmax(0,1fr) 20px minmax(0,1fr) 28px}',
	'.pkb-route select,.pkb-final select{width:100%;min-width:0;margin:0}',
	'.pkb-arrow{color:var(--text-color-medium);text-align:center}',
	'.pkb-final{display:grid;grid-template-columns:28px 44px minmax(0,1fr) 20px minmax(0,1fr) 28px;align-items:center;gap:10px;padding:10px;border-radius:10px;background:var(--background-color-low)}',
	'.pkb-final strong{font-size:13px}',
	'.pkb-switch{position:relative;display:inline-block;width:38px;height:22px}',
	'.pkb-switch input{position:absolute;opacity:0;width:0;height:0}',
	'.pkb-switch span{position:absolute;inset:0;border-radius:22px;background:var(--border-color-medium);transition:.2s;cursor:pointer}',
	'.pkb-switch span:before{content:"";position:absolute;width:16px;height:16px;left:3px;top:3px;border-radius:50%;background:#fff;transition:.2s}',
	'.pkb-switch input:checked+span{background:#2271b1}',
	'.pkb-switch input:checked+span:before{transform:translateX(16px)}',
	'.pkb-switch input:focus-visible+span{outline:2px solid #2271b1;outline-offset:2px}',
	'select.pkb-target{border-left-width:4px!important}',
	'select.pkb-target[data-kind="group"]{border-left-color:#5b4bd6!important}',
	'select.pkb-target[data-kind="subscription"]{border-left-color:#0d7f86!important}',
	'select.pkb-target[data-kind="outbound"]{border-left-color:#4a5568!important}',
	'select.pkb-target[data-kind="direct"]{border-left-color:#0f7a39!important}',
	'select.pkb-target[data-kind="block"]{border-left-color:#b42318!important}',
	'select.pkb-rulegroup{border-left:4px solid #d68b00!important}',
	'.pkb-dirty{display:none;font-size:12px;color:#b45309}',
	'.pkb-dirty.on{display:inline}',

	/* ---- rule table editor ---- */
	'.pkb-tester{display:flex;gap:10px;align-items:center;flex-wrap:wrap}',
	'.pkb-tester .pkb-tester-input{flex:1 1 320px;width:auto}',
	'.pkb-tester-out{display:grid;gap:8px}',
	'.pkb-tester-out:empty{display:none}',
	'.pkb-tester-line{display:flex;gap:8px;align-items:center;flex-wrap:wrap}',
	'.pkb-arrow{color:var(--text-color-medium)}',
	'.pkb-rules-tools{display:flex;gap:12px;flex-wrap:wrap;align-items:center;justify-content:space-between}',
	'.pkb-presets{display:grid;gap:14px;max-height:56vh;overflow:auto;padding-right:4px}',
	'.pkb-presets fieldset{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:6px;margin:0;padding:0;border:0}',
	'.pkb-presets legend{grid-column:1/-1;padding:0 0 4px;font-size:11px;font-weight:600;letter-spacing:.04em;text-transform:uppercase;color:var(--text-color-medium)}',
	'.pkb-preset{display:flex!important;gap:10px;align-items:flex-start;padding:8px 10px;border:1px solid var(--border-color-medium);border-radius:10px;cursor:pointer;background:var(--background-color-high)}',
	'.pkb-preset:hover{border-color:#2271b1}',
	'.pkb-preset:has(input:checked){border-color:#2271b1;background:rgba(34,113,177,.08)}',
	'.pkb-preset input{margin-top:3px}',
	'.pkb-preset span{display:grid;gap:1px;min-width:0}',
	'.pkb-preset strong{font-size:13px;color:var(--text-color-high)}',
	'.pkb-preset small{font-size:11px;color:var(--text-color-medium)}',
	'.pkb-preset.pkb-taken{opacity:.55;cursor:default}',
	'.pkb-namebar{display:flex;gap:16px;align-items:flex-end;flex-wrap:wrap}',
	'.pkb-namebar>label:first-child{flex:1 1 260px}',
	'.pkb-namebar input.cbi-input-text{width:100%;box-sizing:border-box}',
	'.pkb-namebar .pikabox-check{margin:0 0 8px}',
	'.pkb-panel{display:flex;flex-direction:column;border:1px solid var(--border-color-medium);border-radius:10px;background:var(--background-color-low);overflow:hidden}',
	'.pkb-panel-head,.pkb-panel-foot{display:flex;gap:10px;align-items:center;justify-content:space-between;padding:8px 12px;background:var(--background-color-high)}',
	'.pkb-panel-head{border-bottom:1px solid var(--border-color-medium)}',
	'.pkb-panel-foot{border-top:1px solid var(--border-color-medium);justify-content:flex-start}',
	'.pkb-panel-head,.pkb-panel-foot{color:var(--text-color-high);font-weight:400}',
	'.pkb-panel-title{font-size:13px;font-weight:600;color:var(--text-color-high)}',
	'.pkb-panel-title .pkb-count{display:inline-block;margin-left:4px;padding:1px 8px;border-radius:999px;background:var(--background-color-low);font-weight:600}',
	'.pkb-panel-body{max-height:44vh;overflow:auto}',
	'.pkb-search{position:relative;display:block;width:220px;margin:0}',
	'.pkb-search:before{content:"";position:absolute;left:10px;top:50%;width:9px;height:9px;margin-top:-6px;border:1.5px solid var(--text-color-medium);border-radius:50%;pointer-events:none}',
	'.pkb-search:after{content:"";position:absolute;left:19px;top:50%;width:5px;height:1.5px;margin-top:3px;background:var(--text-color-medium);transform:rotate(45deg);pointer-events:none}',
	'.pkb-search input{width:100%;height:30px;box-sizing:border-box;margin:0;padding:0 10px 0 30px;border:1px solid var(--border-color-medium);border-radius:999px;background:var(--background-color-low);color:inherit;font-size:12px}',
	'.pkb-search input:focus{outline:none;border-color:#2271b1;box-shadow:0 0 0 2px rgba(34,113,177,.2)}',
	'.pkb-rules{width:100%;border-collapse:collapse}',
	'.pkb-rules thead th{position:sticky;top:0;z-index:1;background:var(--background-color-low);font-size:11px;text-transform:uppercase;letter-spacing:.04em;color:var(--text-color-medium);font-weight:600;text-align:left;padding:6px 12px;border-bottom:1px solid var(--border-color-medium)}',
	'.pkb-rules td{padding:5px 12px;vertical-align:middle;border-bottom:1px solid var(--border-color-medium)}',
	'.pkb-rules tbody tr:last-child td{border-bottom:0}',
	'.pkb-rules td:first-child{width:180px;padding-right:4px}',
	'.pkb-rules td:last-child{width:28px;padding-left:0}',
	'.pkb-rules select,.pkb-rules input{width:100%;box-sizing:border-box;margin:0;font-family:monospace;font-size:12px}',
	'.pkb-rules tr.pkb-invalid input{border-color:#b42318}',
	'@media(max-width:600px){.pkb-search{width:100%}.pkb-panel-head{flex-wrap:wrap}.pkb-rules td:first-child{width:130px}}',
	'.pkb-count{font-size:12px;color:var(--text-color-medium)}',
	'.modal:has(.pkb-wide){width:min(880px,94vw)!important;max-width:none!important}',
	'.pikabox-modal-form.pkb-wide{min-width:0;width:100%}',

	/* own chevron: the native arrow sits against the border in Chrome/macOS */
	'.pikabox-page select,.pikabox-dashboard select,.modal .pikabox-modal-form select{-webkit-appearance:none;appearance:none;padding-right:30px!important;background-image:url(\"data:image/svg+xml,%3Csvg xmlns=%27http://www.w3.org/2000/svg%27 width=%2710%27 height=%276%27 viewBox=%270 0 10 6%27%3E%3Cpath d=%27M1 1l4 4 4-4%27 fill=%27none%27 stroke=%27%23888%27 stroke-width=%271.5%27 stroke-linecap=%27round%27 stroke-linejoin=%27round%27/%3E%3C/svg%3E\");background-repeat:no-repeat;background-position:right 10px center;background-size:10px 6px}',
	'@media(prefers-reduced-motion:reduce){.pkb-item,.pkb-switch span,.pkb-switch span:before{transition:none}.pkb-icon-btn.spinning svg{animation:none}}',
	'@media(max-width:760px){.pkb-route,.pkb-final{grid-template-columns:28px 44px minmax(0,1fr) 28px}.pkb-route .pkb-arrow,.pkb-final .pkb-arrow{display:none}.pkb-route>select:nth-of-type(2),.pkb-final>select{grid-column:3}}',
	'.pikabox-mono{font-family:monospace;font-size:12px}',
	'textarea.pikabox-mono{min-height:120px;resize:vertical;width:100%;box-sizing:border-box}',

	/* ---- responsive ---- */
	'@media(max-width:760px){',
	'.pikabox-grid{grid-template-columns:1fr}',
	'.pikabox-table{display:block;overflow-x:auto;white-space:nowrap}',
	'.pikabox-preview{white-space:pre-wrap;word-break:break-word}',
	'.pikabox-row{flex-direction:column;align-items:stretch}',
	'.pikabox-row-control{justify-content:flex-start}',
	'.pikabox-section{padding:14px}',
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
			return E('span', { 'class': 'pikabox-ms down', 'title': error || '' }, [ health === 'error' ? _('error') : _('down') ]);
		if (!ms)
			return E('span', { 'class': 'pikabox-ms none' }, [ '-' ]);
		var cls = ms < 200 ? 'fast' : (ms < 500 ? 'mid' : 'slow');
		return E('span', { 'class': 'pikabox-ms ' + cls }, [ ms + ' ms' ]);
	},

	// bulk renders a bulk-download verdict: throughput when the tunnel
	// carried volume, the stall point when it froze, a dash when unchecked.
	bulk: function(state, kbps, error) {
		if (state === 'ok')
			return E('span', { 'class': 'pikabox-ms fast', 'title': _('A 256 KB download completed through this outbound') }, [
				kbps >= 1024 ? (kbps / 1024).toFixed(1) + ' MB/s' : kbps + ' KB/s' ]);
		if (state === 'stalled')
			return E('span', { 'class': 'pikabox-ms down', 'title': _('Answers latency tests but freezes on real traffic — typical DPI throttling. Not used for selection or fallback.') }, [ error || _('stalls') ]);
		return E('span', { 'class': 'pikabox-ms none', 'title': _('Not bulk-checked yet') }, [ '—' ]);
	},

	// ago formats an ISO time as "12s ago" / "5m ago" / "3h ago" / "2d ago".
	ago: function(iso) {
		if (!iso)
			return '';
		var s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
		if (s < 60)
			return _('%ds ago').format(s);
		if (s < 3600)
			return _('%dm ago').format(Math.floor(s / 60));
		if (s < 86400)
			return _('%dh ago').format(Math.floor(s / 3600));
		return _('%dd ago').format(Math.floor(s / 86400));
	},

	// timeAgo renders a relative time with the exact local time on hover.
	timeAgo: function(iso, empty) {
		if (!iso)
			return E('span', {}, [ empty || '-' ]);
		return E('time', { 'datetime': iso, 'title': new Date(iso).toLocaleString() }, [ this.ago(iso) ]);
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
		return E('label', { 'class': 'pikabox-check' }, [
			E('input', { 'type': 'checkbox', 'name': name, 'checked': checked ? 'checked' : null }),
			label
		]);
	},

	// refresh re-runs a view's load+render and swaps the page in place.
	refresh: function(view) {
		return view.load().then(function(data) {
			var replacement = view.render(data);
			var root = document.querySelector('.pikabox-page');
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
	modal: function(view, title, body, save, successText, wide, saveLabel) {
		var self = this;
		ui.showModal(title, [
			E('div', { 'class': 'pikabox-modal-form' + (wide ? ' pkb-wide' : '') }, body),
			E('div', { 'class': 'right' }, [
				E('button', { 'class': 'btn cbi-button', 'click': ui.hideModal }, _('Cancel')),
				' ',
				E('button', {
					'class': 'btn cbi-button cbi-button-apply',
					'click': ui.createHandlerFn(view, function() {
						return save(document.querySelector('.pikabox-modal-form')).then(function(result) {
							if (!self.result(result, successText, _('Save failed')))
								return;
							ui.hideModal();
							return self.refresh(view);
						});
					})
				}, saveLabel || _('Save'))
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
		return E('span', { 'class': 'pkb-pill pkb-k-' + cls, 'title': title || '' }, [
			String(name),
			(kind && kind !== 'builtin' && kind !== 'rule') ? E('small', {}, [ kind === 'subscription' ? _('sub') : kind ]) : ''
		]);
	},

	grip: function() {
		var btn = E('button', { 'type': 'button', 'class': 'pkb-grip', 'aria-label': _('Drag to reorder; Alt+Up/Down moves') });
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
			item.parentNode.dispatchEvent(new CustomEvent('pkb-reorder', { bubbles: true }));
		});
		return btn;
	},

	// sortable makes a list's [data-sortable-item] children draggable by
	// their .pkb-grip, loading the bundled SortableJS once. onChange runs
	// after any reorder (drag or keyboard).
	sortable: function(list, onChange) {
		list.addEventListener('pkb-reorder', function() { if (onChange) onChange(); });
		var attach = function() {
			window.Sortable.create(list, {
				handle: '.pkb-grip',
				draggable: '[data-sortable-item]',
				animation: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 160,
				ghostClass: 'pkb-ghost',
				chosenClass: 'pkb-chosen',
				dragClass: 'pkb-drag',
				onEnd: function() { if (onChange) onChange(); }
			});
		};
		if (window.Sortable)
			return attach();
		if (!sortableLoading) {
			sortableLoading = new Promise(function(resolve, reject) {
				var script = document.createElement('script');
				script.src = L.resource('pikabox/Sortable.min.js');
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
		var list = E('ol', { 'class': 'pkb-list', 'data-ordered': '1' });
		var empty = E('div', { 'class': 'pkb-empty' }, [ _('No members yet — add one below.') ]);
		var renumber = function() {
			var items = list.querySelectorAll('[data-sortable-item]');
			items.forEach(function(el, i) { el.querySelector('.pkb-pos').textContent = '#' + (i + 1); });
			empty.style.display = items.length ? 'none' : '';
		};
		var item = function(id) {
			var o = byId[id] || { id: id, name: id, kind: '' };
			var el = E('li', { 'class': 'pkb-item', 'data-sortable-item': '1', 'data-value': id }, [
				self.grip(),
				E('div', { 'class': 'pkb-item-main' }, [
					E('span', { 'class': 'pkb-pos' }),
					self.pill(o.name || o.id, o.kind, o.kind ? '' : _('Missing — it was deleted'))
				]),
				E('button', { 'type': 'button', 'class': 'pkb-x', 'aria-label': _('Remove %s').format(o.name || o.id), 'click': function() {
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
		return E('div', {}, [ list, empty, E('div', { 'class': 'pkb-add' }, [ picker ]) ]);
	},

	orderedValues: function(root) {
		var list = root.querySelector('[data-ordered]');
		return list ? Array.prototype.map.call(list.querySelectorAll('[data-value]'), function(el) { return el.dataset.value; }) : [];
	},

	// inject adds the shared stylesheet to <head> once. Re-renders are common
	// (polling, refreshView replacing the page subtree); the id guard keeps a
	// single <style> element regardless of how often render() runs.
	inject: function() {
		if (document.getElementById('pikabox-theme'))
			return;
		var style = document.createElement('style');
		style.id = 'pikabox-theme';
		style.textContent = CSS;
		document.head.appendChild(style);
	}
});
