package app

import (
	"html/template"
	"strings"
)

func visualHTML(data []byte, language string) ([]byte, error) {
	const page = `<!doctype html>
<html lang="{{.HTMLLang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'">
<title>Norns Visual Snapshot</title>
<style>
:root{color-scheme:light;--bg:#f3f5f8;--panel:#fff;--panel-soft:#f7f9fc;--line:#e0e5ed;--line-strong:#bdc8d8;--text:#222a38;--muted:#667387;--green:#167354;--green-soft:#e8f3ee;--amber:#ad650c;--amber-soft:#fff3dd;--red:#b83b3b;--red-soft:#fbeaea;--blue:#28649b;--blue-soft:#eaf2f9;--violet:#75549c;--violet-soft:#f1ecf7;--shadow:0 8px 28px rgba(32,49,77,.09);--accent:#3563c5;--accent-soft:#eaf0fd;--active-surface:#f6f9ff;--heading:#24344f;--code-bg:#202c40;--canvas-bg:#f6f8fa;--canvas-grid:#e2e8f04d;--module-shadow:0 3px 14px rgba(32,49,77,.04);--control-shadow:0 4px 14px rgba(32,49,77,.12)}
*{box-sizing:border-box}html{max-width:100%;scroll-behavior:smooth}body{max-width:100%;margin:0;overflow-x:hidden;background:var(--bg);color:var(--text);font:15px/1.65 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;letter-spacing:0}button,input,select{font:inherit;color:inherit}button{cursor:pointer}button:focus-visible,input:focus-visible,select:focus-visible,[tabindex]:focus-visible{outline:2px solid var(--accent);outline-offset:2px}button:disabled{cursor:not-allowed;opacity:.45}
.topbar{position:sticky;top:0;z-index:20;display:flex;align-items:center;gap:20px;height:58px;padding:0 max(18px,calc((100vw - 1440px)/2));border-bottom:1px solid var(--line);background:var(--panel);color:var(--heading);backdrop-filter:blur(12px)}.brand{font-size:18px;font-weight:720;white-space:nowrap}.top-summary{display:flex;align-items:center;gap:16px;color:var(--muted);font-size:13px}.top-summary strong{color:var(--text);font-size:14px}.runtime{display:inline-flex;align-items:center;gap:7px}.runtime-dot{width:8px;height:8px;border-radius:50%;background:#98a29e}.runtime.active .runtime-dot{background:var(--green);box-shadow:0 0 0 4px var(--green-soft)}.generated{margin-left:auto;color:var(--muted);font-size:12px;white-space:nowrap}
main{width:min(1440px,100%);margin:0 auto;padding:24px 22px 60px}.module{scroll-margin-top:74px;padding:22px;margin:0 0 18px;border:1px solid var(--line);border-radius:8px;background:var(--panel);box-shadow:var(--module-shadow)}.module-head{display:flex;align-items:flex-end;justify-content:space-between;gap:18px;margin-bottom:16px;padding-bottom:16px;border-bottom:1px solid var(--line)}.module-title{color:var(--heading);margin:0;font-size:21px;line-height:1.3}.module-note{margin:6px 0 0;color:var(--muted);font-size:13px}.module-meta{color:var(--muted);font-size:13px}
.empty-state{padding:40px 18px;border:1px dashed var(--line-strong);border-radius:6px;text-align:center;color:var(--muted);background:var(--panel-soft)}.empty-state[hidden]{display:none}
.task-list{position:relative;display:grid;min-width:0;gap:10px;padding-left:30px}.task-list::before{position:absolute;top:13px;bottom:13px;left:10px;width:1px;background:var(--line-strong);content:""}.task-item{--task-color:#98a29e;--task-soft:#edf0ef;position:relative;min-width:0;max-width:100%}.task-item.open{--task-color:#2d6fae;--task-soft:#e9f2fb}.task-item.closed{--task-color:#218061;--task-soft:#e8f4ef}.task-item::before{position:absolute;z-index:2;top:16px;left:-25px;width:10px;height:10px;border:2px solid var(--panel);border-radius:50%;background:var(--task-color);box-shadow:0 0 0 1px var(--task-color);content:""}.task-item.expanded::before{box-shadow:0 0 0 1px var(--task-color),0 0 0 5px var(--task-soft)}.task-toggle{display:grid;grid-template-columns:minmax(0,1fr) auto auto auto;align-items:center;gap:12px;width:100%;min-width:0;min-height:64px;padding:10px 14px;border:1px solid var(--line);border-radius:6px;background:var(--panel);text-align:left}.task-toggle:hover{border-color:var(--line-strong);background:var(--panel-soft)}.task-preview{font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.task-start{color:var(--muted);font-size:12px;white-space:nowrap}.status,.outcome-badge{display:inline-flex;align-items:center;gap:6px;padding:3px 7px;border-radius:4px;background:#edf0ef;color:#59645f;font-size:12px;font-weight:650;text-transform:uppercase}.status::before,.outcome-badge::before{content:"";width:6px;height:6px;border-radius:50%;background:currentColor}.status.open,.status.running{background:var(--blue-soft);color:var(--blue)}.status.todo,.outcome-badge.inconclusive,.outcome-badge.unachievable{background:var(--amber-soft);color:var(--amber)}.status.finished,.status.closed,.outcome-badge.succeeded{background:var(--green-soft);color:var(--green)}.outcome-badge.failed{background:var(--red-soft);color:var(--red)}.outcome-badge.cancelled{background:#edf0ef;color:#66716d}.task-toggle .status{background:var(--task-soft);color:var(--task-color)}.task-chevron{color:var(--muted);font-size:17px;transition:transform .18s ease}.task-item.expanded .task-chevron{transform:rotate(180deg)}.task-details{display:none;width:100%;min-width:0;padding:14px;border:1px solid var(--line);border-top:0;border-radius:0 0 6px 6px;background:var(--panel-soft)}.task-item.expanded .task-toggle{border-color:var(--accent);border-radius:6px 6px 0 0}.task-item.expanded .task-details{display:grid;gap:12px;border-color:var(--accent)}.task-block{min-width:0;max-width:100%;padding:16px;border:1px solid var(--line);border-radius:5px;background:var(--panel)}.task-block-head{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:8px}.task-block-title{color:var(--muted);font-size:12px;font-weight:700}.task-block-id{color:var(--muted);font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.task-block-value,.task-cognition .markdown{font-size:15px;font-weight:400;overflow-wrap:anywhere;line-height:1.7}.task-cognition .markdown{max-height:320px;overflow:auto}.task-cognition .markdown p{margin-bottom:12px}.task-meta{display:grid;min-width:0;max-width:100%;border:1px solid var(--line);border-radius:5px;background:var(--panel)}.task-meta.open-meta{grid-template-columns:repeat(4,minmax(0,1fr))}.task-meta.closed-meta{grid-template-columns:repeat(3,minmax(0,1fr))}.task-meta-item{min-width:0;padding:12px 14px;border-right:1px solid var(--line)}.task-meta-item:nth-child(4n),.closed-meta .task-meta-item:nth-child(3n){border-right:0}.closed-meta .task-meta-item:nth-child(n+4){border-top:1px solid var(--line)}.field-label{display:block;margin-bottom:5px;color:var(--muted);font-size:12px;text-transform:uppercase}.field-value{overflow-wrap:anywhere}.task-reason{border-left:3px solid #9aa8a2}
.graph-summary{display:grid;width:100%;max-width:100%;min-width:0;gap:0;margin-bottom:12px;border:1px solid var(--line);border-radius:6px;overflow:hidden}.graph-stat-row{display:grid;grid-template-columns:minmax(180px,1.35fr) repeat(4,minmax(100px,1fr));background:var(--panel)}.graph-stat-row+.graph-stat-row{border-top:1px solid var(--line)}.graph-stat-heading{display:flex;align-items:baseline;gap:6px;padding:12px 14px;background:var(--panel-soft);font-size:13px;font-weight:720}.graph-stat-heading small,.metric-cn{color:var(--muted);font-size:11px;font-weight:500}.metric{padding:12px 14px;border-left:1px solid var(--line)}.metric-label{display:flex;align-items:baseline;gap:5px;font-size:12px;font-weight:600}.metric-value{margin-top:4px;font-size:20px;font-weight:720;line-height:1.3}
.graph-shell{position:relative;border:1px solid var(--line);border-radius:7px;background:var(--panel);overflow:hidden}.graph-shell:fullscreen{display:grid;grid-template-rows:1fr;width:100vw;height:100vh;border:0;border-radius:0}.graph-shell:fullscreen .graph-body,.graph-shell:fullscreen .graph-main,.graph-shell:fullscreen .viewport{height:100%;min-height:0}.graph-body{display:grid;grid-template-columns:minmax(0,1fr) 360px;grid-template-rows:minmax(0,1fr);height:680px;min-height:0}.graph-main{position:relative;min-width:0;min-height:0}.graph-controls{position:absolute;z-index:6;bottom:15px;left:15px;display:flex;align-items:center;overflow:hidden;border:1px solid var(--line-strong);border-radius:6px;background:var(--panel);box-shadow:var(--control-shadow)}.graph-control{display:grid;width:38px;height:36px;padding:0;place-items:center;border:0;border-left:1px solid var(--line);background:var(--panel);color:var(--text)}.graph-control:first-child{border-left:0}.graph-control:hover,.graph-control:focus-visible{background:var(--accent-soft);color:var(--accent)}.graph-control svg{width:17px;height:17px;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}.zoom-value{display:flex;width:58px;height:36px;align-items:center;justify-content:center;gap:1px;border-left:1px solid var(--line);background:var(--panel-soft);color:var(--muted);font:12px ui-monospace,SFMono-Regular,Menlo,monospace;font-variant-numeric:tabular-nums}.zoom-value input{width:34px;padding:0;border:0;outline:0;background:transparent;color:inherit;text-align:right;font:inherit}.zoom-value input::-webkit-inner-spin-button,.zoom-value input::-webkit-outer-spin-button{-webkit-appearance:none;margin:0}.zoom-value input{-moz-appearance:textfield}.graph-search{position:absolute;z-index:7;bottom:62px;left:15px;display:none;width:min(350px,calc(100% - 30px));padding:8px;border:1px solid var(--line-strong);border-radius:6px;background:var(--panel);box-shadow:var(--shadow)}.graph-search.open{display:flex;gap:6px}.graph-search input{width:100%;height:36px;padding:0 10px;border:1px solid var(--line-strong);border-radius:5px}.graph-search button{width:36px;height:36px;flex:0 0 36px;border:0;border-radius:4px;background:var(--panel-soft);font-size:19px}
.viewport{position:relative;height:680px;overflow:hidden;background-color:var(--canvas-bg);background-image:linear-gradient(var(--canvas-grid) 1px,transparent 1px),linear-gradient(90deg,var(--canvas-grid) 1px,transparent 1px);background-size:28px 28px;touch-action:none;cursor:grab}.viewport.dragging{cursor:grabbing}.stage{position:absolute;left:0;top:0;transform-origin:0 0}.edges{position:absolute;inset:0;overflow:visible}.edge-layer,.edge{pointer-events:none}.edge{fill:none;stroke:#a5aeaa;stroke-width:1.55;opacity:.72;transition:opacity .15s,stroke .15s,stroke-width .15s}.edge.retry{stroke-dasharray:6 5}.edge.related{stroke:#737e79;stroke-width:2.3;opacity:1}.edge.unrelated{opacity:.08}.node{color:#17211f;cursor:pointer;outline:none;transition:opacity .15s,filter .15s}.node:hover{filter:drop-shadow(0 7px 8px rgba(23,33,31,.17))}.node.search-match{filter:drop-shadow(0 0 6px rgba(40,100,155,.55))}.node.search-match .node-shape{stroke:var(--blue);stroke-width:2.5}.node.search-dimmed{opacity:.18}.node-shape{fill:#fff;stroke:#aab7b2;stroke-width:1.7;transition:fill .15s,stroke .15s,stroke-width .15s}.node-status-ring{fill:none;stroke-width:3.5;opacity:.92;pointer-events:none}.node.cognition.task-cognition .node-shape{fill:url(#cognition-bronze);stroke:#955d30}.node.cognition.regular .node-shape{fill:url(#cognition-silver);stroke:#7b858c}.node.cognition.result-cognition .node-shape{fill:url(#cognition-gold);stroke:#9a7414}.node.exploration .node-shape{fill:#fff;stroke:#8e9c98}.node.exploration.todo .node-status-ring{stroke:#d4882f}.node.exploration.running .node-status-ring{stroke:#477dbd}.node.exploration.finished .node-status-ring{stroke:#17684e}.node.selected{opacity:1;filter:drop-shadow(0 5px 6px rgba(23,33,31,.24))}.node.selected .node-shape{stroke:#17211f;stroke-width:2.4}.node:focus .node-shape,.node:hover .node-shape{stroke:#17211f}.node.related{opacity:1}.node.unrelated{opacity:.16}.node-copy{height:100%;padding:13px 17px;overflow:visible;pointer-events:none}.node.exploration .node-copy{padding-right:46px}.node-heading{display:flex;align-items:center;gap:7px;min-width:0}.node-icon{width:15px;height:15px;flex:0 0 auto;fill:none;stroke:currentColor;stroke-width:1.9;stroke-linecap:round;stroke-linejoin:round}.node-icon-glyph{transform-box:view-box;transform-origin:12px 12px}.node.exploration.running .node-icon-glyph{animation:icon-spin 1.6s linear infinite}@keyframes icon-spin{to{transform:rotate(360deg)}}.node-kind{display:block;color:#52605b;font-size:11px;font-weight:750}.node.cognition.task-cognition .node-heading{color:#653918}.node.cognition.regular .node-heading{color:#4e565b}.node.cognition.result-cognition .node-heading{color:#6b4e08}.node.exploration.todo .node-heading{color:#9c5708}.node.exploration.running .node-heading{color:#275dad}.node.exploration.finished .node-heading{color:#17684e}.node.exploration .node-kind{color:currentColor}.node-title{display:block;margin-top:5px;font-size:13px;font-weight:650;line-height:1.45;white-space:normal;overflow-wrap:anywhere}.node-port{stroke:#fff;stroke-width:1.6;pointer-events:none;transition:opacity .15s}.node.cognition.task-cognition .node-port{fill:#955d30}.node.cognition.regular .node-port{fill:#7b858c}.node.cognition.result-cognition .node-port{fill:#9a7414}.node.exploration.todo .node-port{fill:#d4882f}.node.exploration.running .node-port{fill:#477dbd}.node.exploration.finished .node-port{fill:#17684e}.node-port.search-dimmed,.node-port.unrelated{opacity:.18}.graph-empty{position:absolute;inset:0;display:grid;place-items:center;color:var(--muted);pointer-events:none}.graph-empty[hidden]{display:none}
.detail{position:relative;min-width:0;min-height:0;max-width:100%;border-left:1px solid var(--line);background:var(--panel);overflow:auto;overscroll-behavior:contain;scrollbar-gutter:stable}.detail-head{position:sticky;top:0;z-index:2;display:flex;align-items:center;justify-content:space-between;min-height:50px;padding:0 18px;border-bottom:1px solid var(--line);background:var(--panel)}.detail-head h3{margin:0;font-size:13px;text-transform:uppercase;color:var(--muted)}.detail-close{display:none;width:30px;height:30px;border:0;background:transparent;font-size:20px}.detail-body{padding:18px}.detail-empty{padding-top:34px;text-align:center;color:var(--muted)}.detail-title{font-size:18px;font-weight:700;overflow-wrap:anywhere;line-height:1.45}.detail-id{margin:3px 0 14px;color:var(--muted);font:11px/1.45 ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}.detail-id.detail-id-tail{margin:17px 0 0;padding-top:13px;border-top:1px solid var(--line)}.detail-content{margin:14px 0 17px;padding:15px 0 16px;border-top:1px solid var(--line);border-bottom:1px solid var(--line)}.fields{margin:0}.fields dt{margin-top:14px;color:var(--muted);font-size:12px;text-transform:uppercase}.fields dd{margin:5px 0;overflow-wrap:anywhere}.relation-list{display:flex;flex-wrap:wrap;gap:5px}.relation-link{max-width:100%;padding:3px 6px;border:1px solid var(--line);border-radius:4px;background:var(--panel-soft);color:var(--accent);font:13px/1.5 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.relation-link:hover{border-color:var(--accent);background:var(--accent-soft)}
.markdown{color:var(--text);overflow-wrap:anywhere;line-height:1.7}.markdown>:first-child{margin-top:0}.markdown>:last-child{margin-bottom:0}.markdown p{margin:0 0 12px}.markdown h1,.markdown h2,.markdown h3,.markdown h4,.markdown h5,.markdown h6{margin:18px 0 9px;line-height:1.4}.markdown h1{font-size:22px}.markdown h2{font-size:19px}.markdown h3{font-size:17px}.markdown h4,.markdown h5,.markdown h6{font-size:15px}.markdown ul,.markdown ol{margin:8px 0 14px;padding-left:22px}.markdown li+li{margin-top:5px}.markdown blockquote{margin:9px 0;padding:4px 0 4px 12px;border-left:3px solid var(--line-strong);color:var(--muted)}.markdown code{padding:2px 4px;border-radius:3px;background:var(--panel-soft);font:13px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace}.markdown pre{margin:9px 0;padding:13px 14px;overflow:auto;border:1px solid var(--code-bg);border-radius:5px;background:var(--code-bg);color:#f3f6fa}.markdown pre code{padding:0;background:transparent;color:inherit;white-space:pre}.markdown a{color:var(--accent);text-underline-offset:2px}.markdown hr{margin:13px 0;border:0;border-top:1px solid var(--line)}.markdown table{width:100%;margin:9px 0;border-collapse:collapse;font-size:13px}.markdown th,.markdown td{padding:8px 10px;border:1px solid var(--line);text-align:left;vertical-align:top}.markdown th{background:var(--panel-soft)}
.exploration-controls{display:flex;align-items:flex-end;gap:10px;flex-wrap:wrap;margin-bottom:14px}.filter-field{display:grid;gap:5px}.status-filter-field{margin-left:14px}.filter-label{color:var(--muted);font-size:12px;font-weight:700;text-transform:uppercase}.select-wrap{position:relative;width:240px}.select-wrap svg{position:absolute;right:11px;top:50%;width:16px;height:16px;fill:none;stroke:var(--muted);stroke-width:2;stroke-linecap:round;stroke-linejoin:round;pointer-events:none;transform:translateY(-50%)}.exploration-task-filter{width:240px;height:38px;padding:0 36px 0 11px;border:1px solid var(--line-strong);border-radius:6px;appearance:none;background:var(--panel);font-size:13px}.exploration-task-filter:hover{border-color:var(--line-strong)}.filter-segmented{display:flex;overflow:hidden;border:1px solid var(--line-strong);border-radius:6px;background:var(--panel)}.filter{height:36px;padding:0 13px;border:0;border-left:1px solid var(--line);background:var(--panel);color:var(--muted);font-size:13px}.filter:first-child{border-left:0}.filter:hover{background:var(--panel-soft)}.filter[aria-pressed=true]{background:var(--accent-soft);color:var(--accent);font-weight:680;box-shadow:inset 0 -2px var(--accent)}.exploration-count{margin-left:auto;padding-bottom:8px;color:var(--muted);font-size:13px}.exploration-list{display:grid;gap:10px}.exploration-item{--exploration-color:#59645f;border:1px solid var(--line);border-left:4px solid var(--exploration-color);border-radius:6px;background:var(--panel);overflow:visible}.exploration-item.running{--exploration-color:var(--blue)}.exploration-item.todo{--exploration-color:var(--amber)}.exploration-item.finished:not([data-outcome]),.exploration-item.finished[data-outcome="succeeded"]{--exploration-color:var(--green)}.exploration-item.finished[data-outcome="inconclusive"],.exploration-item.finished[data-outcome="unachievable"]{--exploration-color:var(--amber)}.exploration-item.finished[data-outcome="failed"]{--exploration-color:var(--red)}.exploration-item.finished[data-outcome="cancelled"]{--exploration-color:#66716d}.exploration-item.active{border-color:var(--accent);border-left-color:var(--exploration-color);background:var(--active-surface)}.exploration-toggle{display:grid;grid-template-columns:auto minmax(0,1fr) auto auto;align-items:center;gap:11px;width:100%;min-height:64px;padding:11px 14px;border:0;border-radius:5px;background:transparent;text-align:left}.exploration-toggle:hover{background:var(--panel-soft)}.exploration-item.active .exploration-toggle{border-radius:5px 5px 0 0}.exploration-index{color:var(--muted);font:12px ui-monospace,SFMono-Regular,Menlo,monospace}.exploration-title{font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.exploration-sub{display:block;margin-top:2px;color:var(--muted);font:12px/1.5 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.exploration-time{color:var(--muted);font-size:12px;white-space:nowrap}.exploration-details{display:none;margin:0 14px 14px;padding:16px;border-top:1px solid var(--line);background:var(--panel)}.exploration-item.active .exploration-details{display:grid;grid-template-columns:minmax(0,1.4fr) minmax(260px,1fr);gap:22px}.exploration-details-content{min-width:0}.exploration-details-content h4,.exploration-details-fields h4{margin:0 0 8px;color:var(--muted);font-size:12px;text-transform:uppercase}.exploration-attributes{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 14px;margin:0}.exploration-attribute{min-width:0}.exploration-attribute dt{color:var(--muted);font-size:12px;text-transform:uppercase}.exploration-attribute dd{margin:4px 0;font-size:13px;overflow-wrap:anywhere}.exploration-source-list{display:flex;flex-wrap:wrap;gap:4px}.exploration-source{max-width:100%;padding:2px 5px;border:1px solid var(--line);border-radius:4px;background:var(--panel-soft);font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.exploration-related{position:fixed;z-index:30;right:max(12px,calc((100vw - 1440px)/2 + 22px));top:50%;width:min(680px,calc(100vw - 24px));max-height:calc(100vh - 32px);padding:16px;border:1px solid var(--line-strong);border-radius:7px;background:var(--panel);box-shadow:0 18px 54px rgba(32,49,77,.2);overflow:auto;transform:translateY(-50%)}.exploration-related[hidden]{display:none}.related-head{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:12px}.related-head h3{margin:0;font-size:16px}.related-close{width:30px;height:30px;border:0;background:transparent;font-size:20px}.related-stack{display:grid;gap:8px}.related-item{padding:11px 12px;border:1px solid var(--line);border-radius:5px;background:var(--panel-soft)}.related-kind{color:var(--muted);font-size:12px;font-weight:720;text-transform:uppercase}.related-title{margin:2px 0 5px;font-weight:680}.related-id{margin-top:7px;color:var(--muted);font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
.task-cognition .markdown{margin-top:2px;padding:10px 12px;border:1px solid var(--line);border-radius:5px;background:var(--panel-soft)}
.zoom-value{display:grid;grid-template-columns:3ch auto;gap:1px;place-content:center;font:12px ui-monospace,SFMono-Regular,Menlo,monospace}.zoom-value input{width:3ch;text-align:right}
.node-kind,.related-kind{text-transform:none}
.relation-token{display:inline-block;max-width:100%;padding:3px 6px;border:1px solid var(--line);border-radius:4px;background:var(--panel-soft);color:var(--muted);font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.fields dt{text-transform:none;font-size:12px;margin-top:14px}
.exploration-details-fields{min-width:0;padding-left:22px;border-left:1px solid var(--line)}.exploration-details-content h4,.exploration-details-fields h4,.exploration-attribute dt{text-transform:none}.exploration-attribute.wide{grid-column:1/-1}.exploration-source-list{display:grid;justify-items:start}.exploration-source-list.empty{display:block;color:var(--muted)}
.related-item>.markdown{margin:9px 0 10px;padding:10px 11px;border:1px solid var(--line);border-radius:5px;background:var(--panel)}.related-meta{display:grid;gap:4px;margin:0}.related-meta-row{display:grid;grid-template-columns:auto minmax(0,1fr);gap:5px}.related-meta dt{color:var(--muted);font-size:12px}.related-meta dd{margin:0;font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
@media(max-width:1000px){.task-meta.open-meta{grid-template-columns:repeat(2,minmax(0,1fr))}.task-meta.closed-meta{grid-template-columns:repeat(3,minmax(0,1fr))}.graph-stat-row{grid-template-columns:minmax(150px,1.2fr) repeat(4,minmax(80px,1fr))}.graph-body{grid-template-columns:1fr;height:620px}.detail{position:absolute;right:0;top:0;bottom:0;z-index:8;display:none;width:min(390px,92vw);border-left:1px solid var(--line);box-shadow:-8px 0 24px rgba(23,33,31,.14)}.detail.visible{display:block}.detail-close{display:block}.viewport{height:620px}}
@media(max-width:700px){main{max-width:100%;padding:12px 9px 40px}.topbar{padding:0 12px}.brand{font-size:16px}.generated,.top-summary .node-total{display:none}.module{max-width:100%;min-width:0;padding:15px;margin-bottom:12px}.module-head{align-items:flex-start}.module-title{color:var(--heading);font-size:20px;line-height:1.3}.task-list{padding-left:23px;gap:10px}.task-list::before{left:7px}.task-item::before{left:-21px}.task-toggle{grid-template-columns:minmax(0,1fr) auto auto;min-height:64px;padding:10px 14px}.task-start{display:none;font-size:12px}.task-meta.open-meta,.task-meta.closed-meta{grid-template-columns:1fr 1fr}.task-meta-item,.closed-meta .task-meta-item:nth-child(n+4){border-top:1px solid var(--line);border-right:0}.task-meta-item:nth-child(odd){border-right:1px solid var(--line)}.task-block-id{max-width:58%;font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace}.graph-summary{max-width:100%;overflow:hidden}.graph-stat-row{min-width:0;grid-template-columns:repeat(2,minmax(0,1fr))}.graph-stat-heading{grid-column:1/-1;font-size:13px;padding:12px 14px}.graph-stat-row .metric:nth-child(even){border-left:0}.graph-stat-row .metric:nth-child(n+4){border-top:1px solid var(--line)}.graph-body{height:580px}.viewport{height:580px}.graph-controls{right:12px;bottom:12px;left:12px;overflow-x:auto}.graph-search{right:12px;bottom:60px;left:12px;width:auto}.filter-field:first-child,.select-wrap,.exploration-task-filter{width:100%}.status-filter-field{margin-left:0}.exploration-count{width:100%;margin:2px 0 0;font-size:13px}.exploration-toggle{grid-template-columns:auto minmax(0,1fr) auto;min-height:64px;padding:11px 14px}.exploration-time{display:none;font-size:12px}.exploration-toggle .status{grid-column:2}.exploration-item.active .exploration-details{grid-template-columns:1fr}.exploration-attributes{grid-template-columns:1fr}.exploration-related{right:12px;left:12px;width:auto}}
@media(max-width:700px){.exploration-details-fields{padding-top:14px;padding-left:0;border-top:1px solid var(--line);border-left:0}.exploration-attribute.wide{grid-column:auto}}
.task-overview{min-width:0;display:grid;gap:5px}.task-progress{color:var(--muted);font-size:12px;font-weight:400}.task-state,.exploration-state{display:flex;align-items:center;justify-content:flex-end;gap:5px;flex-wrap:wrap}.task-reason{border-left:3px solid var(--line-strong)}.task-purpose-block .task-block-value{line-height:1.7}.exploration-identity{min-width:0}.exploration-sub{font:12px/1.5 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.detail-context{display:flex;align-items:center;gap:6px;flex-wrap:wrap;margin-bottom:10px}.relation-list{flex-direction:column;align-items:stretch}.relation-link{text-align:left;white-space:normal;overflow-wrap:anywhere;font:13px/1.5 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.relation-link:hover{border-color:var(--accent);background:var(--accent-soft)}.relation-link small{display:block;color:var(--muted);font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace}.related-item>summary{cursor:pointer;list-style:none}.related-item>summary::-webkit-details-marker{display:none}.related-item>summary::after{display:block;margin-top:6px;color:var(--accent);font-size:12px;content:attr(data-expand-label)}.related-item[open]>summary::after{content:attr(data-collapse-label)}.related-locate{display:inline-flex;margin-top:12px;padding:6px 9px;border:1px solid var(--line-strong);border-radius:4px;background:var(--panel-soft);color:var(--accent);font-size:13px}.related-locate:hover{background:var(--accent-soft)}
@media(max-width:700px){.task-state{max-width:110px}.exploration-state{grid-column:2;justify-content:flex-start}}
.exploration-reason{margin-top:18px;padding-top:16px;border-top:1px solid var(--line)}.exploration-reason h4{margin:0 0 10px;color:var(--muted);font-size:12px;font-weight:700}
.node-selection-outline{display:none;fill:none;stroke:#8ec5ff;stroke-width:10;stroke-opacity:.7;vector-effect:non-scaling-stroke;pointer-events:none}.node.selected{opacity:1;filter:drop-shadow(0 0 9px rgba(11,111,234,.5))}.node.selected .node-selection-outline{display:block}.node.selected.cognition .node-shape,.node.selected.exploration .node-shape{stroke:#0b6fea;stroke-width:2.8;vector-effect:non-scaling-stroke}.node.selected .node-port.in,.node.selected .node-port.out{fill:#0b6fea;stroke:#fff;stroke-width:2;vector-effect:non-scaling-stroke}
.detail-actions{display:grid;gap:7px;margin-top:12px;padding-top:12px;border-top:1px solid var(--line)}.detail-action-row{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:7px}.detail-action{min-height:34px;padding:6px 9px;border:1px solid var(--line-strong);border-radius:5px;background:var(--panel-soft);color:var(--text);font-size:12px}.detail-action:hover{border-color:var(--accent);background:var(--accent-soft)}.detail-action[aria-pressed="true"]{border-color:var(--accent);background:var(--accent-soft);color:var(--accent)}.detail-scope-note{width:100%;margin:1px 0 0;color:var(--muted);font-size:12px}.exploration-output-actions{margin-top:16px;padding-top:16px;border-top:1px solid var(--line)}.exploration-output-actions .detail-action{background:var(--panel);color:var(--accent)}.graph-search.open{display:grid;gap:7px}.search-input-row{display:flex;align-items:center;gap:6px;min-width:0}.search-input-row input{min-width:0}.search-navigation{display:flex;align-items:center;justify-content:space-between;gap:8px;color:var(--muted);font-size:12px}.search-navigation>div{display:flex;gap:4px}.search-navigation button{width:28px;height:26px;padding:3px;border:1px solid var(--line);border-radius:4px;background:var(--panel-soft)}.search-navigation button svg{width:15px;height:15px}.graph-control:disabled,.search-navigation button:disabled{opacity:.4;cursor:default}
.graph-view-toggle{width:38px}.graph-view-toggle[aria-pressed="true"]{background:var(--accent-soft);color:var(--accent)}.graph-shell[data-graph-view="knowledge"] .node-port{display:none}
.graph-shell[data-graph-view="knowledge"] .node{--node-color:#7b858c;--node-soft:#edf0f3}.graph-shell[data-graph-view="knowledge"] .node.task-cognition{--node-color:#955d30;--node-soft:#f5eadd}.graph-shell[data-graph-view="knowledge"] .node.result-cognition{--node-color:#9a7414;--node-soft:#faf2d9}.graph-shell[data-graph-view="knowledge"] .node.todo{--node-color:#d4882f;--node-soft:#fff3dd}.graph-shell[data-graph-view="knowledge"] .node.running{--node-color:#477dbd;--node-soft:#eaf2f9}.graph-shell[data-graph-view="knowledge"] .node.finished{--node-color:#17684e;--node-soft:#e8f3ee}
.graph-shell[data-graph-view="knowledge"] .node .node-shape{fill:var(--panel);stroke:var(--line-strong);stroke-width:1.3}.knowledge-ring{fill:none;stroke:var(--node-color);stroke-width:2.5;pointer-events:none}.knowledge-root-halo{fill:var(--node-soft);fill-opacity:.6;stroke:var(--node-color);stroke-opacity:.18;stroke-width:1.5;pointer-events:none}.knowledge-root-orbit{fill:none;stroke:var(--node-color);stroke-opacity:.22;stroke-width:1;stroke-dasharray:3 7;pointer-events:none}
.graph-shell[data-graph-view="knowledge"] .node-copy{display:flex;flex-direction:column;align-items:center;justify-content:center;padding:24px;text-align:center;overflow:hidden}.graph-shell[data-graph-view="knowledge"] .node-heading{justify-content:center}.graph-shell[data-graph-view="knowledge"] .node-heading .node-kind{display:none}.graph-shell[data-graph-view="knowledge"] .node-icon{width:52px;height:52px;color:var(--node-color)}.graph-shell[data-graph-view="knowledge"] .node-title{display:none}.graph-shell[data-graph-view="knowledge"] .node.knowledge-root .node-copy{padding:26px}.graph-shell[data-graph-view="knowledge"] .node.knowledge-root .node-icon{width:64px;height:64px}.graph-shell[data-graph-view="knowledge"] .node.knowledge-root .knowledge-ring{stroke-width:3.5}.graph-shell[data-graph-view="knowledge"] .node.selected .node-shape,.graph-shell[data-graph-view="knowledge"] .node:focus .node-shape{stroke:var(--accent);stroke-width:2.5}
@media(prefers-reduced-motion:reduce){html{scroll-behavior:auto}.task-chevron,.exploration-chevron,.node,.edge{transition:none}}
.graph-legend.knowledge-legend{display:none}.graph-shell[data-graph-view="knowledge"]{border-color:#d6e2ef;box-shadow:0 5px 24px rgba(62,89,132,.045)}.knowledge-guides{pointer-events:none}.knowledge-orbit{fill:none;stroke:#d3deeb;stroke-width:1;stroke-dasharray:2 10;opacity:.32;vector-effect:non-scaling-stroke}
.graph-shell[data-graph-view="knowledge"] .edge{stroke:var(--edge-color);stroke-width:calc(var(--graph-edge-width,1px)*1.5);stroke-linecap:round;stroke-linejoin:round;opacity:.55}.graph-shell[data-graph-view="knowledge"] .edge.retry{stroke-dasharray:calc(var(--graph-edge-width,1px)*5) calc(var(--graph-edge-width,1px)*6)}.graph-shell[data-graph-view="knowledge"] .edge.related{stroke:var(--edge-color);stroke-width:calc(var(--graph-edge-width,1px)*2.2);opacity:.95}.graph-shell[data-graph-view="knowledge"] .edge.unrelated{opacity:.055}
.graph-legend{position:absolute;top:14px;left:14px;z-index:4;display:flex;align-items:center;flex-wrap:wrap;gap:12px;max-width:calc(100% - 28px);padding:9px 12px;border:1px solid rgba(210,224,242,.85);border-radius:9px;background:rgba(255,255,255,.92);box-shadow:0 3px 14px rgba(58,86,126,.04);color:#586b83;font-size:12px;line-height:1.3;pointer-events:none;backdrop-filter:blur(8px)}.graph-shell[data-graph-view="knowledge"] .knowledge-legend{display:flex}.graph-shell[data-graph-view="knowledge"] .plane-legend{display:none}.plane-legend,.knowledge-legend{flex-direction:column;align-items:flex-start;gap:8px}.legend-row{display:flex;align-items:center;flex-wrap:wrap;gap:8px 12px}.legend-group-name{min-width:28px;color:#78869a;font-weight:600}.legend-row>span{display:inline-flex;align-items:center;gap:6px;white-space:nowrap}.plane-legend i{display:inline-flex;width:20px;height:14px;border:0;background:none;flex:none}.legend-metal{width:100%;height:100%;overflow:visible}.legend-metal rect{stroke:var(--legend-color);stroke-width:1.5;filter:brightness(.88) saturate(1.3)}.legend-row-exploration i{position:relative;width:18px;height:14px;border:0;border-radius:0;background:var(--legend-color);clip-path:polygon(0 0,72% 0,100% 50%,72% 100%,0 100%)}.legend-row-exploration i::after{content:"";position:absolute;inset:2px;background:#fff;clip-path:inherit}.legend-task{--legend-color:#955d30;--legend-soft:#f5eadd}.legend-result{--legend-color:#9a7414;--legend-soft:#faf2d9}.legend-regular{--legend-color:#7b858c;--legend-soft:#edf0f3}.legend-todo{--legend-color:#d4882f}.legend-running{--legend-color:#477dbd}.legend-finished{--legend-color:#17684e}.graph-shell[data-graph-view="knowledge"] .graph-controls{border-color:#cddcf0;border-radius:9px;background:rgba(255,255,255,.94);box-shadow:0 5px 20px rgba(65,94,136,.1)}.graph-shell[data-graph-view="knowledge"] .node{filter:drop-shadow(0 3px 5px rgba(49,73,109,.08))}.graph-shell[data-graph-view="knowledge"] .node.selected{filter:drop-shadow(0 0 9px rgba(70,126,222,.3))}.graph-shell[data-graph-view="knowledge"] .node.unrelated{filter:none}
@media(max-width:700px){.graph-legend{max-width:calc(100% - 20px);top:10px;left:10px;gap:8px;padding:7px 9px;font-size:11px;line-height:1.3}.legend-row{gap:6px 9px}.knowledge-help{display:none!important}}
.knowledge-label-frame{overflow:visible;transform:scale(var(--knowledge-label-scale,1));transform-origin:0 0;pointer-events:none}.knowledge-label-wrap{display:block;width:100%;pointer-events:none}.knowledge-label-text{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:var(--knowledge-label-lines,2);overflow:hidden;overflow-wrap:anywhere}.knowledge-label-copy{display:block;overflow:hidden;max-width:100%;padding:5px 8px;border:1px solid #e2e8f0;border-radius:6px;background:rgba(255,255,255,.96);box-shadow:0 2px 6px rgba(40,61,92,.04);color:#34435a;font:600 13px/1.5 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;text-align:left;overflow-wrap:anywhere;pointer-events:auto}.graph-shell .node.knowledge-root .knowledge-label-copy{border-color:#d7c7b7;color:#70451f;font-size:13px;font-weight:700}.node.selected .knowledge-label-copy,.node.previewed .knowledge-label-copy,.node:focus-visible .knowledge-label-copy{border-color:#83a7e7;background:#f7faff;color:#244e91}.node.preview-unrelated{opacity:.22}.graph-shell[data-graph-view="knowledge"] .edge.preview-related{opacity:.95;stroke-width:calc(var(--graph-edge-width,1px)*2.2)}.graph-shell[data-graph-view="knowledge"] .edge.preview-unrelated{opacity:.08}
.knowledge-label-link{stroke:#8b9bb1;stroke-width:var(--graph-edge-width,1px);opacity:.45;pointer-events:none}.knowledge-root .knowledge-label-link{stroke:#a37545;opacity:.7}
.knowledge-legend i{display:inline-flex;width:14px;height:14px;flex:none}.knowledge-legend .legend-metal circle{fill:none;stroke-width:3;filter:brightness(.88) saturate(1.3)}.knowledge-legend .legend-exploration i{position:relative;background:var(--legend-color);clip-path:polygon(25% 0,75% 0,100% 50%,75% 100%,25% 100%,0 50%)}.knowledge-legend .legend-exploration i::after{content:"";position:absolute;inset:2px;background:#fff;clip-path:inherit}.knowledge-help{display:none}.graph-shell[data-graph-view="knowledge"] .knowledge-help{display:block;position:absolute;right:16px;bottom:24px;margin:0;color:#6b7d94;font-size:11px;pointer-events:none}

#graph-module{scroll-margin-top:76px}
.branch-dock{position:absolute;z-index:5;left:18px;right:18px;bottom:12px;display:grid;gap:8px;pointer-events:none}.branch-dock-tools{display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:8px 12px}
.graph-shell .graph-body{grid-template-columns:minmax(0,1fr)}
.graph-shell .viewport{background-color:var(--canvas-bg)}
.graph-shell[data-graph-view=plane] .viewport{background-size:28px 28px;background-position:0 0}
.graph-shell[data-graph-view=knowledge]{border-color:#d9e3e9;border-radius:12px;box-shadow:0 8px 34px #233f5410}
.graph-shell[data-graph-view=knowledge] .graph-body{height:800px;grid-template-columns:minmax(0,1fr)}
.graph-shell[data-graph-view=knowledge] .viewport{height:100%;min-height:0;background-image:none}
.graph-shell[data-graph-view=knowledge] .viewport::before{content:"";position:absolute;inset:0;pointer-events:none;background-image:radial-gradient(#8ea5b1 1.1px,transparent 1.1px);background-size:32px 32px;background-position:var(--branch-grid-x,0px) var(--branch-grid-y,0px);opacity:.44}
.graph-shell .detail{position:absolute;inset:0 0 0 auto;z-index:8;display:none;width:min(350px,90%);background:#fffffff5;backdrop-filter:blur(18px);box-shadow:-8px 0 35px #30495e13}
.graph-shell .detail.visible{display:block}.graph-shell .detail-close{display:block}
.graph-shell[data-graph-view=knowledge] .node{filter:none}.graph-shell[data-graph-view=knowledge] .node-copy{padding:10px}.graph-shell[data-graph-view=knowledge] .node-icon{width:23px;height:23px}.graph-shell[data-graph-view=knowledge] .node.knowledge-root .node-copy{padding:16px}.graph-shell[data-graph-view=knowledge] .node.knowledge-root .node-icon{width:37px;height:37px}.graph-shell[data-graph-view=knowledge] .knowledge-ring{stroke-width:2}.graph-shell[data-graph-view=knowledge] .node.knowledge-root .knowledge-ring{stroke-width:2.8}.graph-shell[data-graph-view=knowledge] .node.cognition .knowledge-ring{stroke-width:4;filter:brightness(.88) saturate(1.3)}.graph-shell[data-graph-view=knowledge] .node.cognition.knowledge-root .knowledge-ring{stroke-width:5}.knowledge-root-orbit{display:none}.knowledge-root-halo{fill-opacity:.4}
.graph-shell[data-graph-view=knowledge] .node .node-shape{stroke:#b8c9d1;stroke-width:1.1;fill:#ffffff}.graph-shell[data-graph-view=knowledge] .node.knowledge-root .node-shape{fill:#fffaf0;stroke:#c4a36f}.graph-shell[data-graph-view=knowledge] .node.selected{filter:drop-shadow(0 0 6px #477dbd4d)}
.branch-canopy{fill-opacity:.035;stroke-opacity:.13;stroke-width:1;vector-effect:non-scaling-stroke}.branch-canopy.branch-inactive{fill-opacity:.01;stroke-opacity:.04}
.graph-shell[data-graph-view=knowledge] .edge.backbone{opacity:.66;stroke-width:calc(var(--graph-edge-width)*1.25)}.graph-shell[data-graph-view=knowledge] .edge.cross-edge{opacity:.055;stroke-width:calc(var(--graph-edge-width)*.8)}.graph-shell[data-graph-view=knowledge][data-all-relations=true] .edge.cross-edge{opacity:.31}
.graph-shell[data-graph-view=knowledge] .edge.related,.graph-shell[data-graph-view=knowledge] .edge.preview-related{opacity:1;stroke-width:calc(var(--graph-edge-width)*1.8)}.graph-shell[data-graph-view=knowledge] .edge.unrelated,.graph-shell[data-graph-view=knowledge] .edge.preview-unrelated{opacity:.035}.graph-shell[data-graph-view=knowledge] .node.branch-inactive{opacity:.12}.graph-shell[data-graph-view=knowledge] .edge.branch-inactive{opacity:.015}
.knowledge-particles{pointer-events:none}
.graph-shell[data-graph-view=knowledge][data-knowledge-density] .node .knowledge-label{display:block!important}.knowledge-label-copy{padding:3px 6px;border:0;border-radius:4px;background:#fcfdfdf2;box-shadow:none;color:#506373;font-size:12px;line-height:1.45;font-weight:500}.node.exploration .knowledge-label-copy{color:#263f50;font-weight:650}.node.branch-entry .knowledge-label-copy{color:#214b56;font-weight:700}.graph-shell .node.knowledge-root .knowledge-label-copy{padding:6px 9px;border:1px solid #e8d9be;background:#fffcf4;font-size:13px;color:#77572b}.node.selected .knowledge-label-copy,.node.previewed .knowledge-label-copy{background:#edf4ff;border:1px solid #bad0ed;color:#234f87}.knowledge-label-link{opacity:.25}
.knowledge-legend{gap:8px;padding:11px 14px;border-color:#dee7eb;border-radius:8px;background:#ffffffed;box-shadow:none;font-size:11px}.knowledge-help{display:none!important}
.branch-navigation{display:flex;gap:7px;min-width:0;overflow:auto;scrollbar-width:thin;padding:2px 0;pointer-events:auto}.branch-chip{display:inline-flex;align-items:center;gap:7px;max-width:260px;min-width:0;flex:none;padding:7px 10px;border:1px solid #dfe7ec;border-radius:7px;background:#ffffffee;color:#647889;font-size:11px;line-height:1.4;box-shadow:0 2px 6px #203a4a05}.branch-chip::before{content:"";flex:none;width:6px;height:6px;border-radius:50%;background:var(--branch-color)}.branch-chip span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.branch-chip b{font-size:10px;font-weight:650;color:var(--branch-color);padding:1px 5px;border-radius:4px;background:#f1f5f7}.branch-chip[aria-pressed=true],.branch-chip:hover{border-color:var(--branch-color);color:#284553;background:#fff}
.branch-footer{display:flex;justify-content:space-between;gap:10px;color:#8a99a4;font-size:10px;letter-spacing:.1px}.graph-shell[data-graph-view=plane] .branch-navigation,.graph-shell[data-graph-view=plane] .branch-footer{display:none}.branch-dock .graph-controls{position:static;flex:none;max-width:100%;pointer-events:auto;border-color:#dce5e9;box-shadow:0 3px 14px #2643540d}
.graph-search{bottom:calc(var(--branch-dock-height,110px) + 26px)}
.graph-shell[data-graph-view=knowledge]:fullscreen .graph-body{height:100%;min-height:0}
@media(min-width:1600px){main{max-width:1600px}.graph-shell[data-graph-view=knowledge] .graph-body{height:860px}}
@media(max-width:800px){.graph-shell[data-graph-view=knowledge] .graph-body{height:720px}.branch-dock{left:12px;right:12px}.branch-chip{max-width:210px}.branch-footer>span:last-child{display:none}}
@media(prefers-reduced-motion:reduce){.knowledge-particles{display:none!important}}
</style>
</head>
<body>
<header class="topbar"><div class="brand" data-i18n="brand">Norns Visual Snapshot</div><div class="top-summary"><span><strong id="task-count">0</strong> <span data-i18n="tasks">Tasks</span></span><span class="node-total"><strong id="node-count">0</strong> <span data-i18n="nodes">Nodes</span></span><span id="runtime" class="runtime"><i class="runtime-dot"></i><span></span></span></div><div id="generated" class="generated"></div></header>
<main>
<section class="module" id="tasks-module"><div class="module-head"><div><h1 class="module-title" data-i18n="tasksTitle">Tasks</h1><p class="module-note" data-i18n="tasksNote">Task history and lifecycle</p></div><div id="task-module-meta" class="module-meta"></div></div><div id="task-list" class="task-list"></div><div id="task-empty" class="empty-state" hidden data-i18n="noTasks">This Subject has no Tasks</div></section>
<section class="module" id="graph-module"><div class="module-head"><div><h2 class="module-title" data-i18n="graphTitle">Graph</h2><p class="module-note" data-i18n="graphNote">Task-driven cognition and exploration state graph</p></div></div><div class="graph-summary"><div class="graph-stat-row"><div class="graph-stat-heading" data-i18n="explorationNodes">Explorations</div><div class="metric"><div class="metric-label" data-i18n="total">Total</div><div id="metric-total" class="metric-value">0</div></div><div class="metric"><div class="metric-label" data-i18n="todoExplorations">Todo</div><div id="metric-todo" class="metric-value">0</div></div><div class="metric"><div class="metric-label" data-i18n="runningExplorations">Running</div><div id="metric-running" class="metric-value">0</div></div><div class="metric"><div class="metric-label" data-i18n="finishedExplorations">Finished</div><div id="metric-finished" class="metric-value">0</div></div></div><div class="graph-stat-row"><div class="graph-stat-heading" data-i18n="cognitionNodes">Cognitions</div><div class="metric"><div class="metric-label" data-i18n="total">Total</div><div id="metric-cognitions" class="metric-value">0</div></div><div class="metric"><div class="metric-label" data-i18n="taskRelatedCognitions">Task-Related</div><div id="metric-relevant" class="metric-value">0</div></div><div class="metric"><div class="metric-label" data-i18n="commonCognitions">Common</div><div id="metric-regular" class="metric-value">0</div></div><div class="metric"><div class="metric-label" data-i18n="taskInitCognitions">Task-Init</div><div id="metric-task-cognitions" class="metric-value">0</div></div></div></div>
<div id="graph-shell" class="graph-shell"><div class="graph-body"><div class="graph-main"><div id="viewport" class="viewport"><div id="stage" class="stage"><svg id="edges" class="edges" data-i18n-aria="graphAria" aria-label="Cognition and Exploration graph"></svg></div><div class="graph-legend plane-legend" data-i18n-aria="planeLegend" aria-label="Node types and states"><div class="legend-row"><strong class="legend-group-name" data-i18n="knowledgeCognition">Cognition</strong><span class="legend-task"><i aria-hidden="true"><svg class="legend-metal" viewBox="0 0 20 14"><rect x="1" y="1" width="18" height="12" rx="2" fill="url(#cognition-bronze)"></rect></svg></i><span data-i18n="legendTaskInit">Task-Init Cognition</span></span><span class="legend-result"><i aria-hidden="true"><svg class="legend-metal" viewBox="0 0 20 14"><rect x="1" y="1" width="18" height="12" rx="2" fill="url(#cognition-gold)"></rect></svg></i><span data-i18n="legendTaskRelated">Directly Task-Related Cognition</span></span><span class="legend-regular"><i aria-hidden="true"><svg class="legend-metal" viewBox="0 0 20 14"><rect x="1" y="1" width="18" height="12" rx="2" fill="url(#cognition-silver)"></rect></svg></i><span data-i18n="legendCommon">Common Cognition</span></span></div><div class="legend-row legend-row-exploration"><strong class="legend-group-name" data-i18n="knowledgeExploration">Exploration</strong><span class="legend-todo"><i aria-hidden="true"></i><span data-i18n="legendTodoExploration">Todo Exploration</span></span><span class="legend-running"><i aria-hidden="true"></i><span data-i18n="legendRunningExploration">Running Exploration</span></span><span class="legend-finished"><i aria-hidden="true"></i><span data-i18n="legendFinishedExploration">Finished Exploration</span></span></div></div><div class="graph-legend knowledge-legend" data-i18n-aria="planeLegend" aria-label="Node types and states"><div class="legend-row"><strong class="legend-group-name" data-i18n="knowledgeCognition">Cognition</strong><span class="legend-task"><i aria-hidden="true"><svg class="legend-metal" viewBox="0 0 14 14"><circle cx="7" cy="7" r="5" stroke="url(#cognition-bronze)"></circle></svg></i><span data-i18n="legendTaskInit">Task-Init Cognition</span></span><span class="legend-result"><i aria-hidden="true"><svg class="legend-metal" viewBox="0 0 14 14"><circle cx="7" cy="7" r="5" stroke="url(#cognition-gold)"></circle></svg></i><span data-i18n="legendTaskRelated">Directly Task-Related Cognition</span></span><span class="legend-regular"><i aria-hidden="true"><svg class="legend-metal" viewBox="0 0 14 14"><circle cx="7" cy="7" r="5" stroke="url(#cognition-silver)"></circle></svg></i><span data-i18n="legendCommon">Common Cognition</span></span></div><div class="legend-row legend-row-exploration"><strong class="legend-group-name" data-i18n="knowledgeExploration">Exploration</strong><span class="legend-exploration legend-todo"><i aria-hidden="true"></i><span data-i18n="legendTodoExploration">Todo Exploration</span></span><span class="legend-exploration legend-running"><i aria-hidden="true"></i><span data-i18n="legendRunningExploration">Running Exploration</span></span><span class="legend-exploration legend-finished"><i aria-hidden="true"></i><span data-i18n="legendFinishedExploration">Finished Exploration</span></span></div></div><p class="knowledge-help" data-i18n="knowledgeHelp">Hover to trace connections · Click to inspect</p><div id="graph-empty" class="graph-empty" hidden data-i18n="noGraphMatch">No nodes match the current search</div><div id="graph-search" class="graph-search"><div class="search-input-row"><input id="search" type="search" data-i18n-placeholder="searchPlaceholder" data-i18n-aria="searchNodes" placeholder="Search ID, title, or content; separate terms with spaces" aria-label="Search nodes"><button id="search-close" type="button" data-i18n-title="closeSearch" data-i18n-aria="closeSearch" title="Close search" aria-label="Close search">&times;</button></div><div class="search-navigation"><span id="search-status" role="status" aria-live="polite"></span><div><button id="search-previous" type="button" data-i18n-title="previousMatch" data-i18n-aria="previousMatch" disabled><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m14 6-6 6 6 6" fill="none" stroke="currentColor" stroke-width="2"/></svg></button><button id="search-next" type="button" data-i18n-title="nextMatch" data-i18n-aria="nextMatch" disabled><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m10 6 6 6-6 6" fill="none" stroke="currentColor" stroke-width="2"/></svg></button></div></div></div><div class="graph-controls" data-i18n-aria="graphControls" aria-label="Graph controls"><button id="zoom-out" class="graph-control" type="button" data-i18n-title="zoomOut" data-i18n-aria="zoomOut" title="Zoom out" aria-label="Zoom out"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12h14"/></svg></button><label class="zoom-value" data-i18n-title="zoomHint" title="Enter a zoom percentage from 1 to 250"><input id="zoom-value" type="number" min="1" max="250" value="100" inputmode="numeric" data-i18n-aria="zoomPercent" aria-label="Zoom percentage"><span>%</span></label><button id="zoom-in" class="graph-control" type="button" data-i18n-title="zoomIn" data-i18n-aria="zoomIn" title="Zoom in" aria-label="Zoom in"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg></button><button id="fit" class="graph-control" type="button" data-i18n-title="fitAll" data-i18n-aria="fitAll" title="Fit all" aria-label="Fit all"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 3H5a2 2 0 0 0-2 2v3M16 3h3a2 2 0 0 1 2 2v3M8 21H5a2 2 0 0 1-2-2v-3M16 21h3a2 2 0 0 0 2-2v-3"/></svg></button><button id="fullscreen" class="graph-control" type="button" data-i18n-title="fullscreen" data-i18n-aria="fullscreen" title="Fullscreen" aria-label="Fullscreen"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 3H5a2 2 0 0 0-2 2v3M16 3h3a2 2 0 0 1 2 2v3M8 21H5a2 2 0 0 1-2-2v-3M16 21h3a2 2 0 0 0 2-2v-3"/><path d="M9 9l-6-6M15 9l6-6M9 15l-6 6M15 15l6 6"/></svg></button><button id="search-toggle" class="graph-control" type="button" data-i18n-title="searchNodes" data-i18n-aria="searchNodes" title="Search nodes" aria-label="Search nodes" aria-expanded="false"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg></button><button id="graph-view-toggle" class="graph-control graph-view-toggle" type="button" data-i18n-title="switchToKnowledge" data-i18n-aria="switchToKnowledge" aria-pressed="false"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="5" cy="6" r="2.5"/><circle cx="18" cy="5" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="m7.5 6 8-1M6.2 8.2l4.6 7.6M16.8 7.3l-3.6 8.4"/></svg></button></div></div></div><aside id="detail" class="detail"><div class="detail-head"><h3 data-i18n="nodeDetails">Node details</h3><button id="detail-close" class="detail-close" type="button" data-i18n-title="closeDetails" data-i18n-aria="closeDetails" title="Close details" aria-label="Close details">&times;</button></div><div id="detail-body" class="detail-body detail-empty" data-i18n="selectNode">Select a node to view its content and relationships</div></aside></div></div></section>
<section class="module" id="explorations-module"><div class="module-head"><div><h2 class="module-title" data-i18n="explorationsTitle">Explorations</h2><p class="module-note" data-i18n="explorationsNote">Exploration queue and output Cognitions</p></div></div><div id="exploration-controls" class="exploration-controls"><label class="filter-field"><span class="filter-label" data-i18n="taskFilterLabel">Task</span><span class="select-wrap"><select id="exploration-task-filter" class="exploration-task-filter" data-i18n-aria="filterTask" aria-label="Filter Task"><option value="" data-i18n="allTasks">All Tasks</option></select><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg></span></label><div class="filter-field status-filter-field"><span class="filter-label" data-i18n="status">Status</span><div class="filter-segmented"><button class="filter" data-status="" type="button" aria-pressed="true" data-i18n="all">All</button><button class="filter" data-status="todo" type="button" aria-pressed="false" data-i18n="todo">Todo</button><button class="filter" data-status="running" type="button" aria-pressed="false" data-i18n="running">Running</button><button class="filter" data-status="finished" type="button" aria-pressed="false" data-i18n="finished">Finished</button></div></div><span id="exploration-count" class="exploration-count"></span></div><div id="exploration-list" class="exploration-list"></div><div id="exploration-empty" class="empty-state" hidden data-i18n="noExplorations">No Explorations match the current filters</div></section>
</main>
<script type="application/json" id="norns-data">{{.Data}}</script>
<script>
'use strict';
const data=JSON.parse(document.getElementById('norns-data').textContent);
const messages={
  en:{tasks:'Tasks',tasksTitle:'Tasks',nodes:'Nodes',tasksNote:'Task history and lifecycle',noTasks:'This Subject has no Tasks',graphTitle:'Graph',graphNote:'Task-driven cognition and exploration state graph',explorationNodes:'Explorations',cognitionNodes:'Cognitions',total:'Total',todo:'Todo',running:'Running',finished:'Finished',todoExplorations:'Todo',runningExplorations:'Running',finishedExplorations:'Finished',taskRelatedCognitions:'Task-Related',commonCognitions:'Common',taskInitCognitions:'Task-Init',graphAria:'Cognition and Exploration graph',noGraphMatch:'No nodes match the current search',searchPlaceholder:'Search ID, title, or content; separate terms with spaces',searchNodes:'Search nodes',closeSearch:'Close search',graphControls:'Graph controls',zoomOut:'Zoom out',zoomHint:'Enter a zoom percentage from 1 to 250',zoomPercent:'Zoom percentage',zoomIn:'Zoom in',fitAll:'Fit all',fullscreen:'Fullscreen',exitFullscreen:'Exit fullscreen',nodeDetails:'Node details',closeDetails:'Close details',selectNode:'Select a node to view its content and relationships',explorationsTitle:'Explorations',explorationsNote:'Exploration queue and output Cognitions',taskFilterLabel:'Task',filterTask:'Filter Task',allTasks:'All Tasks',status:'Status',all:'All',noExplorations:'No Explorations match the current filters',open:'Open',closed:'Closed',succeeded:'Succeeded',unachievable:'Unachievable',inconclusive:'Inconclusive',failed:'Failed',cancelled:'Cancelled',taskCount:'Tasks: {count}',taskPurpose:'Task Purpose',taskInitCognition:'Task-Init Cognition',createdAt:'Created At',duration:'Duration',maxConcurrency:'Max Concurrency',closedAt:'Closed At',outcome:'Outcome',outcomeReason:'Outcome Reason',taskStatus:'Task Status',taskRelatedCognition:'Task-Related Cognition',commonCognition:'Common Cognition',exploration:'Exploration',upstream:'Upstream',downstream:'Downstream',locate:'Locate {id}',explorationId:'Exploration ID',cognitionId:'Cognition ID',outputCognitions:'Output Cognitions',closeOutputs:'Close output Cognitions',content:'Content',properties:'Properties',executorId:'Executor ID',assignedAt:'Assigned At',finishedAt:'Finished At',sessionFile:'Session File',retryFrom:'Retry From',taskId:'Task ID',sourceCognitions:'Source Cognitions',explorationCount:'{count} Explorations',outputs:'outputs',completedAt:'Completed {time}, ran for {duration}',startedAt:'Started {time}, running for {duration}',createdAtTime:'Created {time}',outputsAria:'{title} output Cognitions',runtimeActive:'Runtime active',runtimeIdle:'Runtime idle',generatedAt:'Generated {time}',day:'day',days:'days',hour:'hour',hours:'hours',minute:'min',minutes:'minutes',second:'sec',seconds:'seconds'},
  zh:{tasks:'任务',tasksTitle:'任务记录',nodes:'节点',tasksNote:'任务历史与生命周期',noTasks:'当前 Subject 尚无 Task',graphTitle:'图记录',graphNote:'以任务为驱动的持续认知与探索状态图',explorationNodes:'探索节点',cognitionNodes:'认知节点',total:'总数',todo:'待分配',running:'进行中',finished:'已完成',todoExplorations:'待分配探索',runningExplorations:'进行中探索',finishedExplorations:'已完成探索',taskRelatedCognitions:'任务相关认知',commonCognitions:'常规认知',taskInitCognitions:'任务初始认知',graphAria:'认知与探索关系图',noGraphMatch:'当前搜索没有匹配节点',searchPlaceholder:'搜索 ID、标题或内容；空格分隔多个条件',searchNodes:'搜索节点',closeSearch:'关闭搜索',graphControls:'图谱控制',zoomOut:'缩小',zoomHint:'输入 1 到 250 的缩放百分比',zoomPercent:'缩放百分比',zoomIn:'放大',fitAll:'适应全部',fullscreen:'全屏',exitFullscreen:'退出全屏',nodeDetails:'节点详情',closeDetails:'关闭详情',selectNode:'选择一个节点查看内容与关系',explorationsTitle:'探索记录',explorationsNote:'探索队列与产出认知',taskFilterLabel:'任务',filterTask:'筛选任务',allTasks:'全部任务',status:'状态',all:'全部',noExplorations:'当前筛选没有 Exploration',open:'进行中',closed:'已关闭',succeeded:'成功',unachievable:'无法达成',inconclusive:'无定论',failed:'失败',cancelled:'已取消',taskCount:'任务总数：{count}',taskPurpose:'任务目标',taskInitCognition:'任务初始认知',createdAt:'创建时间',duration:'持续时间',maxConcurrency:'最大并发',closedAt:'关闭时间',outcome:'结果类型',outcomeReason:'结果理由',taskStatus:'任务状态',taskRelatedCognition:'任务相关认知',commonCognition:'常规认知',exploration:'探索',upstream:'上游关系',downstream:'下游关系',locate:'定位 {id}',explorationId:'Exploration ID',cognitionId:'Cognition ID',outputCognitions:'产出认知',closeOutputs:'关闭产出认知',content:'内容',properties:'属性',executorId:'执行器 ID',assignedAt:'分配时间',finishedAt:'完成时间',sessionFile:'会话文件',retryFrom:'重试来源',taskId:'Task ID',sourceCognitions:'来源认知',explorationCount:'{count} 条探索',outputs:'项产出',completedAt:'完成于 {time}，共执行 {duration}',startedAt:'开始于 {time}，已执行 {duration}',createdAtTime:'创建于 {time}',outputsAria:'{title} 的产出认知',runtimeActive:'Runtime 运行中',runtimeIdle:'Runtime 空闲',generatedAt:'生成于 {time}',day:'天',days:'天',hour:'小时',hours:'小时',minute:'分',minutes:'分钟',second:'秒',seconds:'秒'}
};
messages.en.brand='Norns Visual Snapshot';
messages.zh.brand='Norns 可视化快照';
Object.assign(messages.en,{planeView:'Planar',knowledgeView:'Knowledge graph',switchToKnowledge:'Switch to knowledge graph',switchToPlane:'Switch to planar view'});
Object.assign(messages.zh,{planeView:'平面',knowledgeView:'知识图谱',switchToKnowledge:'切换为知识图谱',switchToPlane:'切换为平面视图'});
Object.assign(messages.en,{focusSelection:'Focus related nodes',fullChain:'Show full chain',fullChainHint:'Highlight the complete upstream and downstream chain',neighborsHint:'Highlight directly connected nodes only',scopeDirect:'Highlighting direct connections',scopeFull:'Highlighting the complete chain',viewExploration:'View exploration record',viewCognitionOutputs:'View cognition outputs',viewOutputs:'View output Cognitions ({count})',searchCount:'{count} matches',searchPosition:'{index} / {total}',previousMatch:'Previous match (Shift+Enter)',nextMatch:'Next match (Enter)'});
Object.assign(messages.zh,{focusSelection:'聚焦关联节点',fullChain:'显示完整链路',fullChainHint:'高亮全部上游与下游链路',neighborsHint:'仅高亮直接上下游',scopeDirect:'当前高亮：直接关联',scopeFull:'当前高亮：完整链路',viewExploration:'查看探索记录',viewCognitionOutputs:'查看认知产出',viewOutputs:'查看产出认知（{count}）',searchCount:'{count} 个匹配节点',searchPosition:'{index} / {total}',previousMatch:'上一个匹配（Shift+Enter）',nextMatch:'下一个匹配（Enter）'});
Object.assign(messages.en,{explorationProgress:'{finished} / {total} explorations completed',noTaskExplorations:'No explorations yet',taskOutputs:'{count} output Cognitions',showContent:'Read content ↓',hideContent:'Collapse content ↑',locateInGraph:'Locate in graph',untitledNode:'Untitled node'});
Object.assign(messages.zh,{explorationProgress:'已完成 {finished} / {total} 条探索',noTaskExplorations:'尚未发起探索',taskOutputs:'{count} 项产出认知',showContent:'展开正文 ↓',hideContent:'收起正文 ↑',locateInGraph:'在图谱中定位',untitledNode:'未命名节点'});
Object.assign(messages.en,{knowledgeRelations:'Graph relations',knowledgeSource:'Source',knowledgeOutput:'Output',knowledgeExtends:'Extension',knowledgeRetry:'Retry'});
Object.assign(messages.zh,{knowledgeRelations:'图谱关系',knowledgeSource:'来源',knowledgeOutput:'产出',knowledgeExtends:'延伸',knowledgeRetry:'重试'});
Object.assign(messages.en,{planeLegend:'Node types and states',legendTaskInit:'Task-Init Cognition',legendTaskRelated:'Directly Task-Related Cognition',legendCommon:'Common Cognition',legendTodoExploration:'Todo Exploration',legendRunningExploration:'Running Exploration',legendFinishedExploration:'Finished Exploration',knowledgeCognition:'Cognition',knowledgeExploration:'Exploration',knowledgeHelp:'Hover to trace connections · Click to inspect'});
Object.assign(messages.zh,{planeLegend:'节点类型与状态',legendTaskInit:'任务初始化认知',legendTaskRelated:'任务直接关联认知',legendCommon:'常规认知',legendTodoExploration:'待分配探索',legendRunningExploration:'进行中探索',legendFinishedExploration:'已完成探索',knowledgeCognition:'认知',knowledgeExploration:'探索',knowledgeHelp:'悬停高亮关联 · 点击查看详情'});
Object.assign(messages.en,{allDirections:'All directions',emphasizeCrossLinks:'Emphasize cross-links',readBackbone:'Read backbone',locateInitialCognition:'Locate initial cognition',branchNodeCount:'{count} nodes; focus branch',showEntireGraph:'Show entire graph',branchStatus:'Branch: {nodes} nodes · {labels} titles shown',graphStatus:'Graph: {nodes} nodes · {labels} titles shown',graphNavigationHint:'Scroll to zoom · Drag to pan · Focus a branch · Select a node to read'});
Object.assign(messages.zh,{allDirections:'全部方向',emphasizeCrossLinks:'强调跨链关系',readBackbone:'回到主干阅读',locateInitialCognition:'定位初始认知',branchNodeCount:'{count} 个节点；点击聚焦',showEntireGraph:'显示完整图谱',branchStatus:'当前分支 {nodes} 个节点 · 当前展示 {labels} 个标题',graphStatus:'全图 {nodes} 个节点 · 当前展示 {labels} 个标题',graphNavigationHint:'滚轮缩放 · 拖动平移 · 点击分支聚焦 · 点击节点阅读'});
const language=data.language==='zh'?'zh':'en',locale=language==='zh'?'zh-CN':'en-US';
const t=(key,values={})=>Object.entries(values).reduce((value,[name,replacement])=>value.replaceAll('{'+name+'}',()=>String(replacement)),messages[language][key]??messages.en[key]??key);
for(const item of document.querySelectorAll('[data-i18n]'))item.textContent=t(item.dataset.i18n);
for(const item of document.querySelectorAll('[data-i18n-title]'))item.title=t(item.dataset.i18nTitle);
for(const item of document.querySelectorAll('[data-i18n-aria]'))item.setAttribute('aria-label',t(item.dataset.i18nAria));
for(const item of document.querySelectorAll('[data-i18n-placeholder]'))item.placeholder=t(item.dataset.i18nPlaceholder);
const el=id=>document.getElementById(id);
const byId=new Map();
for(const c of data.cognitions)byId.set(c.id,{...c,kind:'cognition'});
for(const e of data.explorations)byId.set(e.id,{...e,kind:'exploration'});
const taskById=new Map(data.tasks.map(task=>[task.id,task]));
const statusNames={open:t('open'),closed:t('closed'),todo:t('todo'),running:t('running'),finished:t('finished')};
const outcomeNames={succeeded:t('succeeded'),unachievable:t('unachievable'),inconclusive:t('inconclusive'),failed:t('failed'),cancelled:t('cancelled')};
let explorationFilter='';
let explorationTaskFilter='';
let openRelatedRow=null;
let visible=[];
let nodeElements=new Map();
let nodePortElements=new Map();
let nodePositions=new Map();
let explorationElements=new Map();
let edgeElements=[];
let selectedNodeId=null;
let graphView='plane';
let knowledgeLabelFrame=null,knowledgeLabelMeasure=null;
const graphViewStates=new Map();
let traceSelection=false,searchMatches=[],searchIndex=-1;
let scale=1,panX=24,panY=24,drag=null,stageWidth=900,stageHeight=540;

function text(value){return value===undefined||value===null||value===''?'-':String(value)}
function unit(value,singular,plural){return value+' '+t(value===1?singular:plural)}
function formatDate(value){if(!value)return '-';const date=new Date(value);return Number.isNaN(date.getTime())?String(value):new Intl.DateTimeFormat(locale,{dateStyle:'medium',timeStyle:'short'}).format(date)}
function formatDateSeconds(value){if(!value)return '-';const date=new Date(value);return Number.isNaN(date.getTime())?String(value):new Intl.DateTimeFormat(locale,{dateStyle:'medium',timeStyle:'medium'}).format(date)}
function formatDuration(start,end){if(!start)return '-';const a=new Date(start).getTime();const b=end?new Date(end).getTime():new Date(data.generated_at).getTime();if(!Number.isFinite(a)||!Number.isFinite(b))return '-';let seconds=Math.max(0,Math.floor((b-a)/1000));const days=Math.floor(seconds/86400);seconds%=86400;const hours=Math.floor(seconds/3600);seconds%=3600;const minutes=Math.floor(seconds/60);if(days)return unit(days,'day','days')+' '+unit(hours,'hour','hours');if(hours)return unit(hours,'hour','hours')+' '+unit(minutes,'minute','minutes');if(minutes)return unit(minutes,'minute','minutes');return unit(seconds,'second','seconds')}
function formatElapsed(start,end){if(!start||!end)return '-';const a=new Date(start).getTime(),b=new Date(end).getTime();if(!Number.isFinite(a)||!Number.isFinite(b))return '-';let minutes=Math.max(0,Math.floor((b-a)/60000));const hours=Math.floor(minutes/60);minutes%=60;return hours?unit(hours,'hour','hours')+' '+unit(minutes,'minute','minutes'):unit(minutes,'minute','minutes')}
function statusBadge(status){const badge=document.createElement('span');badge.className='status '+status;badge.textContent=statusNames[status]||status;return badge}
function outcomeName(outcome){return outcomeNames[outcome]||outcome}
function outcomeBadge(outcome){const badge=document.createElement('span');badge.className='outcome-badge '+outcome;badge.textContent=outcomeName(outcome);return badge}
function explorationBadge(item){return item.status==='finished'&&item.outcome?outcomeBadge(item.outcome):statusBadge(item.status)}
function explorationReason(item){if(item.status!=='finished')return null;const section=document.createElement('section');section.className='exploration-reason';const heading=document.createElement('h4');heading.textContent=t('outcomeReason');section.append(heading,renderMarkdown('exploration-reason:'+item.id,item.outcome_reason||'-'));return section}
function explorationStateLabel(item){const status=statusNames[item.status]||item.status;return item.status==='finished'&&item.outcome?status+' · '+outcomeName(item.outcome):status}
function renderMarkdown(id,fallback){const root=document.createElement('div');root.className='markdown';const rendered=data.rendered_content[id];if(typeof rendered==='string')root.innerHTML=rendered;else root.textContent=fallback||'';return root}
function truncate(value,limit=72){const source=String(value||'');return source.length>limit?source.slice(0,limit).trimEnd()+'…':source}
function taskBlock(title,value,className=''){const block=document.createElement('div');block.className=('task-block '+className).trim();const head=document.createElement('div');head.className='task-block-head';const label=document.createElement('span');label.className='task-block-title';label.textContent=title;head.append(label);const body=document.createElement('div');body.className='task-block-value';body.textContent=text(value);block.append(head,body);return{block,head,body}}
function appendMeta(parent,label,value){const field=document.createElement('div');field.className='task-meta-item';const name=document.createElement('span');name.className='field-label';name.textContent=label;const body=document.createElement('div');body.className='field-value';if(value&&typeof value==='object'&&value.nodeType)body.append(value);else body.textContent=text(value);field.append(name,body);parent.append(field)}
function renderTasks(){
  el('task-count').textContent=String(data.tasks.length);el('node-count').textContent=String(byId.size);el('task-module-meta').textContent=t('taskCount',{count:data.tasks.length});el('task-empty').hidden=data.tasks.length!==0;
  const list=el('task-list');list.replaceChildren();const openTask=data.tasks.find(task=>task.status==='open');
  const tasks=[...data.tasks].sort((a,b)=>String(a.created_at).localeCompare(String(b.created_at))||a.id.localeCompare(b.id));
  for(const task of tasks){
    const item=document.createElement('article');item.className='task-item '+task.status+(openTask&&task.id===openTask.id?' expanded':'');item.dataset.id=task.id;
    const toggle=document.createElement('button');toggle.type='button';toggle.className='task-toggle';toggle.setAttribute('aria-expanded',String(item.classList.contains('expanded')));
    const overview=document.createElement('div');overview.className='task-overview';const preview=document.createElement('div');preview.className='task-preview';preview.textContent=truncate(task.purpose);preview.title=task.purpose;const progress=document.createElement('div');progress.className='task-progress';const explorations=data.explorations.filter(exploration=>exploration.task_id===task.id);const outputCount=new Set(explorations.flatMap(exploration=>outputCognitions(exploration.id).map(cognition=>cognition.id))).size;progress.textContent=(explorations.length?t('explorationProgress',{finished:explorations.filter(exploration=>exploration.status==='finished').length,total:explorations.length}):t('noTaskExplorations'))+' · '+t('taskOutputs',{count:outputCount});overview.append(preview,progress);const started=document.createElement('time');started.className='task-start';started.dateTime=task.created_at;started.textContent=formatDate(task.created_at);const state=document.createElement('span');state.className='task-state';state.append(statusBadge(task.status));if(task.outcome)state.append(outcomeBadge(task.outcome));const chevron=document.createElement('span');chevron.className='task-chevron';chevron.textContent='⌄';toggle.append(overview,started,state,chevron);
    const details=document.createElement('div');details.className='task-details';const purpose=taskBlock(t('taskPurpose'),task.purpose,'task-purpose-block');const taskID=document.createElement('span');taskID.className='task-block-id';taskID.textContent=task.id;purpose.head.append(taskID);const cognition=byId.get(task.init_cognition_ref);const cognitionBlock=taskBlock(t('taskInitCognition'),'', 'task-cognition');const initID=document.createElement('span');initID.className='task-block-id';initID.textContent=task.init_cognition_ref;cognitionBlock.head.append(initID);cognitionBlock.body.replaceWith(renderMarkdown(task.init_cognition_ref,cognition?cognition.content:'-'));
    const meta=document.createElement('div');meta.className='task-meta '+(task.closed_at?'closed-meta':'open-meta');appendMeta(meta,t('createdAt'),formatDate(task.created_at));appendMeta(meta,t('duration'),formatDuration(task.created_at,task.closed_at));appendMeta(meta,t('maxConcurrency'),task.max_concurrency);if(task.closed_at){appendMeta(meta,t('closedAt'),formatDate(task.closed_at));appendMeta(meta,t('outcome'),outcomeBadge(task.outcome));appendMeta(meta,t('taskStatus'),statusBadge(task.status))}else appendMeta(meta,t('taskStatus'),statusBadge(task.status));details.append(purpose.block,cognitionBlock.block,meta);
    if(task.closed_at&&task.outcome_reason){const reason=taskBlock(t('outcomeReason'),'', 'task-reason');reason.body.replaceWith(renderMarkdown('task-reason:'+task.id,task.outcome_reason));details.insertBefore(reason.block,cognitionBlock.block)}item.append(toggle,details);
    toggle.addEventListener('click',()=>{const expanding=!item.classList.contains('expanded');item.classList.toggle('expanded',expanding);toggle.setAttribute('aria-expanded',String(expanding))});list.append(item);
  }
}
function cognitionVariant(node){if(node.origin==='task')return 'task-cognition';if(node.task_purpose_relevant)return 'result-cognition';return 'regular'}
const knowledgeThemes = {
  'task-cognition': { color: '#955d30', metal: 'cognition-bronze' },
  'result-cognition': { color: '#9a7414', metal: 'cognition-gold' },
  regular: { color: '#7b858c', metal: 'cognition-silver' },
  todo: { color: '#d4882f' },
  running: { color: '#477dbd' },
  finished: { color: '#17684e' }
};
function knowledgeNodeTheme(node){const variant=node?.kind==='cognition'?cognitionVariant(node):node?.status;return Object.hasOwn(knowledgeThemes,variant)?variant:'regular'}
function knowledgeEdgeTheme(edge){return knowledgeNodeTheme(byId.get(edge.to))}
function cognitionLabel(node){const variant=cognitionVariant(node);return variant==='task-cognition'?t('taskInitCognition'):variant==='result-cognition'?t('taskRelatedCognition'):t('commonCognition')}
function renderMetrics(){const count=status=>data.explorations.filter(item=>item.status===status).length;const taskCognitions=data.cognitions.filter(item=>item.origin==='task').length;const relevant=data.cognitions.filter(item=>item.origin!=='task'&&item.task_purpose_relevant).length;el('metric-total').textContent=String(data.explorations.length);el('metric-todo').textContent=String(count('todo'));el('metric-running').textContent=String(count('running'));el('metric-finished').textContent=String(count('finished'));el('metric-cognitions').textContent=String(data.cognitions.length);el('metric-relevant').textContent=String(relevant);el('metric-regular').textContent=String(data.cognitions.length-taskCognitions-relevant);el('metric-task-cognitions').textContent=String(taskCognitions)}
function graphMatches(node){const terms=el('search').value.trim().toLowerCase().split(/\s+/).filter(Boolean);if(!terms.length)return true;const hay=[node.id,node.title,node.content,node.status,node.origin,node.task_id].filter(Boolean).join(' ').toLowerCase();return terms.every(term=>hay.includes(term))}
function graphNodeHeight(node){const probe=document.createElement('div');probe.className='node-title';probe.style.cssText='position:fixed;visibility:hidden;pointer-events:none;left:-10000px;top:0;width:'+(node.kind==='exploration'?187:216)+'px;margin:0';probe.textContent=node.title;document.body.append(probe);const titleHeight=probe.getBoundingClientRect().height;probe.remove();return Math.max(92,Math.ceil(titleHeight+58))}
function graphNodeCompare(a,b){return String(a.created_at).localeCompare(String(b.created_at))||a.id.localeCompare(b.id)}
const graphColumnGap=300,graphArrowGap=8;
const knowledgeState = {
  palette: ['#387d86', '#7374b6', '#bc8650', '#729b63', '#b56b88', '#5889b8'],
  branches: [],
  parent: new Map(),
  parentEdges: new Map(),
  branchOf: new Map(),
  children: new Map(),
  subtreeSize: new Map(),
  depth: new Map(),
  activeBranch: null,
  allRelations: false,
  labelCount: 0,
  hoveredId: null,
  hoverAnchor: null,
  labelPlacements: new Map(),
  root: null,
};

function branchBuildSkeleton(nodes, edges) {
  const ordered = [...nodes].sort(knowledgeCreatedCompare);
  const byNode = new Map(nodes.map(node => [node.id, node]));
  const incoming = new Map(nodes.map(node => [node.id, []]));
  const root = ordered.find(node => node.kind === 'cognition' && node.origin === 'task') || ordered[0];
  const parent = new Map(), parentEdges = new Map(), branchOf = new Map(), depth = new Map();
  const children = new Map(nodes.map(node => [node.id, []]));
  const visiting = new Set(), assigned = new Set();
  for (const edge of edges) {
    if (byNode.has(edge.from) && byNode.has(edge.to) && edge.from !== edge.to) incoming.get(edge.to).push(edge);
  }
  if (!root) return { root: null, parent, parentEdges, branchOf, depth, children, subtreeSize: new Map() };
  assigned.add(root.id);
  depth.set(root.id, 0);
  branchOf.set(root.id, null);
  const attach = node => {
    if (assigned.has(node.id)) return;
    visiting.add(node.id);
    const candidates = incoming.get(node.id).filter(edge => !visiting.has(edge.from));
    for (const edge of [...candidates].sort((first, second) => knowledgeCreatedCompare(byNode.get(first.from), byNode.get(second.from)))) {
      attach(byNode.get(edge.from));
    }
    const available = candidates.filter(edge => assigned.has(edge.from));
    const support = new Map();
    for (const edge of available) {
      const branch = branchOf.get(edge.from);
      if (branch) support.set(branch, (support.get(branch) || 0) + 1);
    }
    const semantic = edge => node.kind === 'cognition'
      ? edge.kind === 'output' || edge.kind === 'extends' ? 10000 : 0
      : edge.kind === 'retry' ? 10000 : edge.from === root.id ? 9000 : edge.kind === 'source' ? 1000 : 0;
    const score = edge => semantic(edge) + (support.get(branchOf.get(edge.from)) || 0) * 100 + (depth.get(edge.from) || 0) * 4;
    available.sort((first, second) => score(second) - score(first)
      || knowledgeCreatedCompare(byNode.get(second.from), byNode.get(first.from))
      || first.kind.localeCompare(second.kind));
    const chosen = available[0];
    const parentID = chosen?.from || root.id;
    parent.set(node.id, parentID);
    if (chosen) parentEdges.set(node.id, chosen);
    children.get(parentID).push(node.id);
    depth.set(node.id, depth.get(parentID) + 1);
    branchOf.set(node.id, parentID === root.id ? node.id : branchOf.get(parentID));
    visiting.delete(node.id);
    assigned.add(node.id);
  };
  for (const node of ordered) attach(node);
  for (const ids of children.values()) ids.sort((first, second) => knowledgeCreatedCompare(byNode.get(first), byNode.get(second)));
  const subtreeSize = new Map();
  const measure = id => {
    const size = 1 + children.get(id).reduce((sum, child) => sum + measure(child), 0);
    subtreeSize.set(id, size);
    return size;
  };
  measure(root.id);
  return { root: root.id, parent, parentEdges, branchOf, depth, children, subtreeSize };
}

function branchPackTree(id, children, crossSize, separation, spacing = new Map()) {
  const descendants = children.get(id) || [];
  const local = spacing.get(id) || { across: 0, along: 0 };
  const halfWidth = (crossSize + local.across) / 2;
  if (!descendants.length) return { points: [{ id, depth: 0, cross: 0, extra: 0 }], left: [-halfWidth], right: [halfWidth] };
  const packed = [], left = [], right = [];
  for (const child of descendants) {
    const subtree = branchPackTree(child, children, crossSize, separation, spacing);
    let shift = 0;
    if (packed.length) {
      for (let level = 0; level < subtree.left.length; level++) {
        if (right[level] !== undefined) shift = Math.max(shift, right[level] + separation - subtree.left[level]);
      }
    }
    packed.push({ subtree, shift });
    for (let level = 0; level < subtree.left.length; level++) {
      left[level] = Math.min(left[level] ?? Infinity, subtree.left[level] + shift);
      right[level] = Math.max(right[level] ?? -Infinity, subtree.right[level] + shift);
    }
  }
  const middle = (packed[0].shift + packed.at(-1).shift) / 2;
  return {
    points: [{ id, depth: 0, cross: 0, extra: 0 }, ...packed.flatMap(({ subtree, shift }) => subtree.points.map(point => ({ ...point, depth: point.depth + 1, cross: point.cross + shift - middle, extra: point.extra + local.along })))],
    left: [-halfWidth, ...left.map(value => value - middle)],
    right: [halfWidth, ...right.map(value => value - middle)],
  };
}

function branchAllocateSectors(weights) {
  if (weights.length === 1) return [Math.PI * 1.65];
  if (weights.length === 2) return [Math.PI * .88, Math.PI * .88];
  const gap = Math.min(.22, Math.PI / weights.length * .22);
  const available = Math.PI * 2 - weights.length * gap;
  const minimum = Math.min(.7, available / weights.length * .65);
  const maximum = Math.PI * .92;
  const result = new Array(weights.length).fill(minimum);
  let remaining = available - minimum * weights.length;
  let active = weights.map((weight, index) => index);
  while (remaining > .00001 && active.length) {
    const total = active.reduce((sum, index) => sum + weights[index], 0);
    const capped = active.filter(index => result[index] + remaining * weights[index] / total > maximum);
    if (!capped.length) {
      for (const index of active) result[index] += remaining * weights[index] / total;
      break;
    }
    for (const index of capped) { remaining -= maximum - result[index]; result[index] = maximum; }
    active = active.filter(index => !capped.includes(index));
  }
  return result;
}

function branchHull(points) {
  const sorted = [...points].sort((first, second) => first.x - second.x || first.y - second.y);
  const turn = (first, second, third) => (second.x - first.x) * (third.y - first.y) - (second.y - first.y) * (third.x - first.x);
  const lower = [], upper = [];
  for (const point of sorted) { while (lower.length > 1 && turn(lower.at(-2), lower.at(-1), point) <= 0) lower.pop(); lower.push(point); }
  for (const point of [...sorted].reverse()) { while (upper.length > 1 && turn(upper.at(-2), upper.at(-1), point) <= 0) upper.pop(); upper.push(point); }
  return [...lower.slice(0, -1), ...upper.slice(0, -1)];
}

function branchCubicPoint(curve, ratio) {
  const inverse = 1 - ratio;
  return {
    x: inverse ** 3 * curve.start.x + 3 * inverse ** 2 * ratio * curve.first.x + 3 * inverse * ratio ** 2 * curve.second.x + ratio ** 3 * curve.end.x,
    y: inverse ** 3 * curve.start.y + 3 * inverse ** 2 * ratio * curve.first.y + 3 * inverse * ratio ** 2 * curve.second.y + ratio ** 3 * curve.end.y,
  };
}

function branchSampleCubic(curve) {
  const distance = (first, second) => Math.hypot(first.x - second.x, first.y - second.y);
  const perimeter = distance(curve.start, curve.first) + distance(curve.first, curve.second) + distance(curve.second, curve.end);
  const count = Math.min(768, Math.max(12, Math.ceil(perimeter / 5)));
  return Array.from({ length: count + 1 }, (_, index) => branchCubicPoint(curve, index / count));
}

function branchRepairRoute(input, grid, edge) {
  const points = input.map(point => ({ ...point }));
  const source = grid.byId.get(edge.from), target = grid.byId.get(edge.to);
  let repairs = 0;
  for (let pass = 0; pass < 8; pass++) {
    let unsafe = -1;
    for (let index = 1; index < points.length; index++) {
      if (!Number.isFinite(knowledgeRouteScore(points[index - 1], points[index], grid, edge))) { unsafe = index; break; }
    }
    if (unsafe < 0) break;
    const start = { ...points[unsafe - 1], index: source?.index || 0 }, end = { ...points[unsafe], index: target?.index || 0 };
    const routed = knowledgeGridPath(start, end, grid);
    if (!routed || routed.length < 3) break;
    const detour = knowledgeSimplifyPath(routed, grid, edge);
    if (detour.length < 3) break;
    points.splice(unsafe, 0, ...detour.slice(1, -1).map(point => ({ x: point.x, y: point.y })));
    repairs++;
  }
  return { points, repairs };
}

function branchCurveClear(curve, obstacles, padding = 12) {
  const left = Math.min(curve.start.x, curve.first.x, curve.second.x, curve.end.x);
  const right = Math.max(curve.start.x, curve.first.x, curve.second.x, curve.end.x);
  const top = Math.min(curve.start.y, curve.first.y, curve.second.y, curve.end.y);
  const bottom = Math.max(curve.start.y, curve.first.y, curve.second.y, curve.end.y);
  return obstacles.every(obstacle => {
    const clearance = obstacle.radius + padding;
    if (obstacle.x + clearance < left || obstacle.x - clearance > right || obstacle.y + clearance < top || obstacle.y - clearance > bottom) return true;
    for (let index = 1; index < curve.samples.length; index++) {
      if (knowledgeSegmentDistance(obstacle, curve.samples[index - 1], curve.samples[index]) < clearance) return false;
    }
    return true;
  });
}

function branchDirectCurve(grid, edge, input) {
  const source = grid.byId.get(edge.from), target = grid.byId.get(edge.to);
  if (!source || !target) return null;
  const deltaX = target.x - source.x, deltaY = target.y - source.y, distance = Math.hypot(deltaX, deltaY);
  if (distance < 1) return null;
  const obstacles = grid.obstacles.filter(obstacle => obstacle.id !== edge.from && obstacle.id !== edge.to);
  const direction = { x: -deltaY / distance, y: deltaX / distance };
  const deviation = input.slice(1, -1).reduce((sum, point) => sum + (point.x - source.x) * direction.x + (point.y - source.y) * direction.y, 0);
  const sign = deviation < 0 ? -1 : 1;
  const gentle = Math.min(64, distance * .065) * sign;
  const bends = [gentle, -gentle, 0, ...[.14, .26, .42, .64].flatMap(fraction => [distance * fraction * sign, -distance * fraction * sign])];
  const round = point => ({ x: graphPathNumber(point.x), y: graphPathNumber(point.y) });
  const format = point => point.x + ' ' + point.y;
  for (const bend of bends) {
    const first = round({ x: source.x + deltaX / 3 + direction.x * bend, y: source.y + deltaY / 3 + direction.y * bend });
    const second = round({ x: source.x + deltaX * 2 / 3 + direction.x * bend, y: source.y + deltaY * 2 / 3 + direction.y * bend });
    const start = round(knowledgeNodeBoundary({ ...source.position, id: source.id }, first));
    const end = round(knowledgeNodeBoundary({ ...target.position, id: target.id }, second, graphArrowGap));
    const curve = { start, first, second, end };
    curve.samples = branchSampleCubic(curve);
    if (curve.samples.some(point => point.x < 12 || point.y < 12 || point.x > stageWidth - 12 || point.y > stageHeight - 12)) continue;
    if (!branchCurveClear(curve, obstacles)) continue;
    return { path: 'M ' + format(start) + ' C ' + format(first) + ' ' + format(second) + ' ' + format(end), samples: curve.samples, curves: [curve], smoothKind: 'cubic', smoothingPasses: 0, pathRepairs: 0, compactKind: 'single-cubic' };
  }
  return null;
}

function branchCompactRoute(points, grid, edge) {
  const obstacles = grid.obstacles.filter(obstacle => obstacle.id !== edge.from && obstacle.id !== edge.to);
  const result = [points[0]];
  let current = 0;
  while (current < points.length - 1) {
    let limit = points.length - 1, next = current + 1;
    for (let index = current + 1; index < points.length; index++) {
      if (points[index].anchor) { limit = index; break; }
    }
    for (let candidate = limit; candidate > next; candidate--) {
      if (obstacles.every(obstacle => knowledgeSegmentDistance(obstacle, points[current], points[candidate]) >= obstacle.radius + 12)) { next = candidate; break; }
    }
    result.push(points[next]);
    current = next;
  }
  return result;
}

function branchClearHover() {
  if (!knowledgeState.hoveredId) return;
  knowledgeState.hoveredId = null;
  knowledgeState.hoverAnchor = null;
  const focused = document.activeElement?.closest?.('.node');
  previewKnowledgeNode(focused?.dataset.id || null);
}

function branchPointerMove(event) {
  if (graphView !== 'knowledge') return;
  if (event.buttons || event.pointerType === 'touch') { branchClearHover(); return; }
  const viewport = el('viewport').getBoundingClientRect();
  const point = { x: event.clientX - viewport.left, y: event.clientY - viewport.top };
  const localRect = element => {
    const rect = element.getBoundingClientRect();
    return { x: rect.x - viewport.left, y: rect.y - viewport.top, width: rect.width, height: rect.height };
  };
  const hovered = nodeElements.get(knowledgeState.hoveredId);
  if (hovered) {
    const label = hovered.querySelector('.knowledge-label');
    const regions = [knowledgeState.hoverAnchor, localRect(hovered.querySelector('.node-shape'))];
    if (label.getAttribute('visibility') === 'visible') regions.push(localRect(label.querySelector('.knowledge-label-copy')));
    if (regions.some(rect => rect && point.x >= rect.x - 4 && point.x <= rect.x + rect.width + 4 && point.y >= rect.y - 4 && point.y <= rect.y + rect.height + 4)) return;
  }
  const item = event.target.closest?.('.node');
  const id = item?.dataset.id || null;
  if (id === knowledgeState.hoveredId) return;
  knowledgeState.hoveredId = id;
  const label = item?.querySelector('.knowledge-label');
  knowledgeState.hoverAnchor = label?.getAttribute('visibility') === 'visible' ? localRect(label.querySelector('.knowledge-label-copy')) : null;
  const focused = document.activeElement?.closest?.('.node');
  previewKnowledgeNode(id || focused?.dataset.id || null);
}

function branchLabelCandidates(center, size, bounds) {
  const directions = [[0, 1], [1, 0], [-1, 0], [0, -1], [1, 1], [-1, 1], [1, -1], [-1, -1], [2, 1], [-2, 1], [2, -1], [-2, -1], [1, 2], [-1, 2], [1, -2], [-1, -2]];
  return [8, 20, 34].flatMap(gap => directions.map(([horizontal, vertical]) => {
    const length = Math.hypot(horizontal, vertical);
    const offsetX = center.radius * horizontal / length + gap * horizontal;
    const offsetY = center.radius * vertical / length + gap * vertical;
    const rect = {
      x: center.x + offsetX - (horizontal < 0 ? size.width : horizontal === 0 ? size.width / 2 : 0),
      y: center.y + offsetY - (vertical < 0 ? size.height : vertical === 0 ? size.height / 2 : 0),
      width: size.width,
      height: size.height,
    };
    return { ...rect, inside: rect.x >= bounds.left && rect.x + size.width <= bounds.right && rect.y >= bounds.top && rect.y + size.height <= bounds.bottom };
  }));
}

function branchRectsOverlap(first, second, gap = 5) {
  return first.x < second.x + second.width + gap && first.x + first.width + gap > second.x && first.y < second.y + second.height + gap && first.y + first.height + gap > second.y;
}

function branchUpdateBackground() {
  if (graphView !== 'knowledge') return;
  const viewport = el('viewport');
  const interval = 32;
  viewport.style.setProperty('--branch-grid-x', ((panX % interval) + interval) % interval + 'px');
  viewport.style.setProperty('--branch-grid-y', ((panY % interval) + interval) % interval + 'px');
}

function branchFitPositions(ids, local = false) {
  const values = ids.map(id => nodePositions.get(id)).filter(Boolean);
  if (!values.length) return;
  const left = Math.min(...values.map(position => position.x)) - 70;
  const top = Math.min(...values.map(position => position.y)) - 45;
  const right = Math.max(...values.map(position => position.x + position.width)) + 70;
  const bottom = Math.max(...values.map(position => position.y + position.height)) + 100;
  const viewport = el('viewport');
  const usableWidth = Math.max(80, viewport.clientWidth - 80), usableHeight = Math.max(80, viewport.clientHeight - 82 - (el('branch-dock')?.offsetHeight || 110) - 36);
  scale = Math.max(.01, Math.min(local ? 1.2 : .95, usableWidth / (right - left), usableHeight / (bottom - top)));
  panX = viewport.clientWidth / 2 - (left + right) / 2 * scale;
  panY = 82 + usableHeight / 2 - (top + bottom) / 2 * scale;
  transform();
}

function branchApplyFocus() {
  const active = knowledgeState.activeBranch;
  for (const [id, item] of nodeElements) item.classList.toggle('branch-inactive', graphView === 'knowledge' && !!active && knowledgeState.branchOf.get(id) !== active);
  for (const item of edgeElements) item.path.classList.toggle('branch-inactive', graphView === 'knowledge' && !!active && knowledgeState.branchOf.get(item.edge.from) !== active && knowledgeState.branchOf.get(item.edge.to) !== active);
  for (const canopy of el('edges').querySelectorAll('.branch-canopy')) canopy.classList.toggle('branch-inactive', !!active && canopy.dataset.branch !== active);
  for (const button of document.querySelectorAll('.branch-chip')) button.setAttribute('aria-pressed', String(button.dataset.branch === (active || '')));
  scheduleKnowledgeLabels();
}

function branchFocus(id) {
  clearSelection();
  knowledgeState.activeBranch = id;
  branchApplyFocus();
  fitGraph();
}

function branchUpdateStatus() {
  const status = el('branch-status');
  if (!status) return;
  const branch = knowledgeState.branches.find(item => item.id === knowledgeState.activeBranch);
  status.textContent = t(branch ? 'branchStatus' : 'graphStatus', { nodes: branch ? branch.count : nodePositions.size, labels: knowledgeState.labelCount });
}

function branchRenderNavigation() {
  const navigation = el('branch-navigation');
  if (!navigation) return;
  navigation.replaceChildren();
  const add = (label, id, color, count) => {
    const button = document.createElement('button');
    button.className = 'branch-chip';
    button.type = 'button';
    button.dataset.branch = id || '';
    button.setAttribute('aria-pressed', String(knowledgeState.activeBranch === id));
    button.title = label + ' · ' + (id ? t('branchNodeCount', { count }) : t('showEntireGraph'));
    button.style.setProperty('--branch-color', color);
    const name = document.createElement('span'); name.textContent = label;
    button.append(name);
    if (count) { const total = document.createElement('b'); total.textContent = count; button.append(total); }
    button.addEventListener('click', event => { event.stopPropagation(); branchFocus(id); });
    navigation.append(button);
  };
  add(t('allDirections'), null, '#52677f', nodePositions.size);
  for (const branch of knowledgeState.branches) add(branch.title, branch.id, branch.color, branch.count);
}

function branchUpdateDock() {
  const dock = el('branch-dock');
  if (dock) el('graph-shell').style.setProperty('--branch-dock-height', dock.offsetHeight + 'px');
}

function knowledgeNodeSize(node, root = false) { return root ? 150 : node.kind === 'exploration' ? 84 : 72; }
function knowledgeCreatedCompare(first,second){const firstTime=Date.parse(first.created_at),secondTime=Date.parse(second.created_at);return Number.isFinite(firstTime)&&Number.isFinite(secondTime)?firstTime-secondTime||graphNodeCompare(first,second):graphNodeCompare(first,second)}
function knowledgeGraphLayout(nodes, edges) {
  const positions = new Map();
  const skeleton = branchBuildSkeleton(nodes, edges);
  Object.assign(knowledgeState, skeleton);
  knowledgeState.branches = [];
  if (!skeleton.root) { stageWidth = 880; stageHeight = 600; return { positions, curvePlan: new Map() }; }
  const byNode = new Map(nodes.map(node => [node.id, node]));
  const crossDegree = new Map(nodes.map(node => [node.id, 0]));
  for (const edge of edges) {
    if (!byNode.has(edge.from) || !byNode.has(edge.to) || skeleton.parentEdges.get(edge.to) === edge) continue;
    for (const id of [edge.from, edge.to]) crossDegree.set(id, crossDegree.get(id) + 1);
  }
  const spacing = new Map(nodes.map(node => {
    const junction = Math.max(0, skeleton.children.get(node.id).length - 1), pressure = Math.max(0, crossDegree.get(node.id) - 2);
    return [node.id, { across: Math.min(48, pressure * 5 + junction * 9), along: Math.min(38, pressure * 4 + junction * 6) }];
  }));
  knowledgeState.spacing = spacing;
  const branchIDs = skeleton.children.get(skeleton.root);
  const preliminary = branchIDs.map(id => branchPackTree(id, skeleton.children, 122, 24));
  const weights = preliminary.map(tree => Math.sqrt(Math.max(...tree.right.map((right, index) => right - tree.left[index])) / 122));
  const sectors = branchAllocateSectors(weights);
  const gap = branchIDs.length > 2 ? (Math.PI * 2 - sectors.reduce((sum, sector) => sum + sector, 0)) / branchIDs.length : Math.PI * .12;
  const largest = weights.indexOf(Math.max(...weights));
  let cursor = 0;
  const angles = sectors.map(sector => { const center = cursor + sector / 2; cursor += sector + gap; return center; });
  const rotation = -.12 - (angles[largest] || 0);
  const rootNode = byNode.get(skeleton.root), rootSize = knowledgeNodeSize(rootNode, true);
  positions.set(skeleton.root, { x: -rootSize / 2, y: -rootSize / 2, width: rootSize, height: rootSize, level: 0, root: true });
  for (let index = 0; index < branchIDs.length; index++) {
    const id = branchIDs[index], angle = angles[index] + rotation;
    const cosine = Math.cos(angle), sine = Math.sin(angle);
    const crossSize = Math.abs(sine) * 180 + Math.abs(cosine) * 126;
    const advance = Math.abs(cosine) * 180 + Math.abs(sine) * 126 + 46;
    const tree = branchPackTree(id, skeleton.children, crossSize, 24, spacing);
    let offset = 195;
    if (branchIDs.length > 1) {
      const tangent = Math.tan(Math.min(sectors[index], Math.PI * .92) / 2);
      for (const point of tree.points) offset = Math.max(offset, (Math.abs(point.cross) + 110) / tangent - point.depth * advance - point.extra + 110);
    }
    const branch = { id, title: byNode.get(id).title, color: knowledgeState.palette[index % knowledgeState.palette.length], count: tree.points.length, angle, sector: sectors[index], index, ids: tree.points.map(point => point.id) };
    knowledgeState.branches.push(branch);
    for (const point of tree.points) {
      const node = byNode.get(point.id), size = knowledgeNodeSize(node);
      const along = offset + point.depth * advance + point.extra;
      const centerX = along * cosine - point.cross * sine, centerY = along * sine + point.cross * cosine;
      positions.set(node.id, { x: centerX - size / 2, y: centerY - size / 2, width: size, height: size, level: skeleton.depth.get(node.id), root: false, branch: id });
    }
  }
  const boxes = [...positions.values()];
  const left = Math.min(...boxes.map(position => position.x - 94));
  const top = Math.min(...boxes.map(position => position.y - 36));
  const right = Math.max(...boxes.map(position => position.x + position.width + 94));
  const bottom = Math.max(...boxes.map(position => position.y + position.height + 96));
  stageWidth = Math.max(640, right - left + 120);
  stageHeight = Math.max(520, bottom - top + 120);
  for (const position of positions.values()) { position.x += 60 - left; position.y += 60 - top; }
  return { positions, curvePlan: new Map() };
}

function knowledgeHexagon(position,inset=0){const radius=position.width/2-inset;return Array.from({length:6},(_,index)=>({x:position.width/2+Math.cos(index*Math.PI/3)*radius,y:position.height/2+Math.sin(index*Math.PI/3)*radius}))}
function knowledgeNodeShape(node,position,inset,className){return node.kind==='cognition'?svgElement('circle',{class:className,cx:position.width/2,cy:position.height/2,r:position.width/2-inset}):svgElement('polygon',{class:className,points:knowledgeHexagon(position,inset).map(point=>point.x+','+point.y).join(' ')})}
function knowledgeNodeBoundary(position,toward,gap=0){
  const center={x:position.x+position.width/2,y:position.y+position.height/2},deltaX=toward.x-center.x,deltaY=toward.y-center.y,length=Math.hypot(deltaX,deltaY)||1,directionX=deltaX/length,directionY=deltaY/length;
  if(byId.get(position.id)?.kind!=='exploration'){const radius=position.width/2-5+gap;return{x:center.x+directionX*radius,y:center.y+directionY*radius}}
  const vertices=knowledgeHexagon(position,5);
  let distance=Infinity;for(let index=0;index<vertices.length;index++){const first=vertices[index],second=vertices[(index+1)%vertices.length],segmentX=second.x-first.x,segmentY=second.y-first.y,denominator=directionX*segmentY-directionY*segmentX;if(Math.abs(denominator)<.000001)continue;const offsetX=first.x-position.width/2,offsetY=first.y-position.height/2,ray=(offsetX*segmentY-offsetY*segmentX)/denominator,segment=(offsetX*directionY-offsetY*directionX)/denominator;if(ray>=0&&segment>=0&&segment<=1)distance=Math.min(distance,ray)}
  if(!Number.isFinite(distance))distance=position.width/2;return{x:center.x+directionX*(distance+gap),y:center.y+directionY*(distance+gap)};
}
function knowledgeSegmentDistance(point,start,end){const deltaX=end.x-start.x,deltaY=end.y-start.y,length=deltaX*deltaX+deltaY*deltaY,ratio=length?Math.max(0,Math.min(1,((point.x-start.x)*deltaX+(point.y-start.y)*deltaY)/length)):0;return Math.hypot(point.x-start.x-ratio*deltaX,point.y-start.y-ratio*deltaY)}
function knowledgeRoutingGrid(positions){
  const step=Math.max(18,Math.ceil(Math.max(stageWidth,stageHeight)/240)),width=Math.ceil(stageWidth/step)+1,height=Math.ceil(stageHeight/step)+1,owner=new Int32Array(width*height),obstacles=[...positions].map(([id,position],index)=>({id,index:index+1,x:position.x+position.width/2,y:position.y+position.height/2,radius:position.width/2,position}));
  for(const obstacle of obstacles){const radius=obstacle.radius+20;for(let row=Math.max(0,Math.floor((obstacle.y-radius)/step));row<=Math.min(height-1,Math.ceil((obstacle.y+radius)/step));row++)for(let column=Math.max(0,Math.floor((obstacle.x-radius)/step));column<=Math.min(width-1,Math.ceil((obstacle.x+radius)/step));column++)if(Math.hypot(column*step-obstacle.x,row*step-obstacle.y)<radius){const index=row*width+column;owner[index]=owner[index]===0?obstacle.index:-1}}
  return{step,width,height,owner,obstacles,byId:new Map(obstacles.map(obstacle=>[obstacle.id,obstacle])),traffic:new Float32Array(width*height),headings:new Uint16Array(width*height)};
}
function knowledgeHeapPush(heap,item){heap.push(item);let index=heap.length-1;while(index){const parent=Math.floor((index-1)/2);if(heap[parent].priority<=item.priority)break;heap[index]=heap[parent];index=parent}heap[index]=item}
function knowledgeHeapPop(heap){const first=heap[0],last=heap.pop();if(heap.length){let index=0;while(index*2+1<heap.length){let child=index*2+1;if(child+1<heap.length&&heap[child+1].priority<heap[child].priority)child++;if(heap[child].priority>=last.priority)break;heap[index]=heap[child];index=child}heap[index]=last}return first}
function knowledgeGridIndex(point,grid){const column=Math.max(0,Math.min(grid.width-1,Math.round(point.x/grid.step))),row=Math.max(0,Math.min(grid.height-1,Math.round(point.y/grid.step)));return row*grid.width+column}
function knowledgeGridPoint(index,grid){return{x:index%grid.width*grid.step,y:Math.floor(index/grid.width)*grid.step}}
function knowledgeTrafficCost(index,direction,grid){const mask=(1<<direction)|(1<<((direction+4)%8));return grid.traffic[index]*(grid.headings[index]&mask?14:7)}
function knowledgeRouteScore(start,end,grid,edge){
  for(const obstacle of grid.obstacles)if(obstacle.id!==edge.from&&obstacle.id!==edge.to&&knowledgeSegmentDistance(obstacle,start,end)<obstacle.radius+12)return Infinity;
  const length=Math.hypot(end.x-start.x,end.y-start.y),count=Math.max(1,Math.ceil(length/(grid.step*.65))),direction=(Math.round(Math.atan2(end.y-start.y,end.x-start.x)/(Math.PI/4))+8)%8;let cost=0;
  for(let index=1;index<=count;index++){const ratio=index/count,cell=knowledgeGridIndex({x:start.x+(end.x-start.x)*ratio,y:start.y+(end.y-start.y)*ratio},grid);cost+=1+knowledgeTrafficCost(cell,direction,grid)}
  return cost/count*length/grid.step;
}
function knowledgeGridPath(from,to,grid){
  const start=knowledgeGridIndex(from,grid),target=knowledgeGridIndex(to,grid),count=grid.owner.length,scores=new Float64Array(count).fill(Infinity),parents=new Int32Array(count).fill(-1),closed=new Uint8Array(count),heap=[],directions=[[1,0],[1,1],[0,1],[-1,1],[-1,0],[-1,-1],[0,-1],[1,-1]],allowed=index=>grid.owner[index]===0||grid.owner[index]===from.index||grid.owner[index]===to.index,heuristic=index=>Math.hypot(index%grid.width-target%grid.width,Math.floor(index/grid.width)-Math.floor(target/grid.width));
  scores[start]=0;knowledgeHeapPush(heap,{index:start,priority:heuristic(start)});let expanded=0;
  while(heap.length&&expanded<count){const current=knowledgeHeapPop(heap).index;if(closed[current])continue;closed[current]=1;expanded++;if(current===target){const path=[];for(let index=target;index!==-1;index=parents[index])path.push(knowledgeGridPoint(index,grid));path.reverse();return[from,...path.slice(1,-1),to]}
    const column=current%grid.width,row=Math.floor(current/grid.width),previous=parents[current];for(let direction=0;direction<directions.length;direction++){const [deltaX,deltaY]=directions[direction],nextColumn=column+deltaX,nextRow=row+deltaY;if(nextColumn<1||nextColumn>=grid.width-1||nextRow<1||nextRow>=grid.height-1)continue;const next=nextRow*grid.width+nextColumn;if(closed[next]||!allowed(next))continue;if(deltaX&&deltaY&&(!allowed(row*grid.width+nextColumn)||!allowed(nextRow*grid.width+column)))continue;const turning=previous>=0&&(column-previous%grid.width!==deltaX||row-Math.floor(previous/grid.width)!==deltaY),distance=deltaX&&deltaY?Math.SQRT2:1,score=scores[current]+distance*(1+knowledgeTrafficCost(next,direction,grid))+(turning?.28:0);if(score>=scores[next])continue;scores[next]=score;parents[next]=current;knowledgeHeapPush(heap,{index:next,priority:score+heuristic(next)})}
  }
  return null;
}
function knowledgeSimplifyPath(points,grid,edge){
  const costs=[0];for(let index=1;index<points.length;index++)costs.push(costs[index-1]+knowledgeRouteScore(points[index-1],points[index],grid,edge));const simplified=[points[0]];let current=0;
  while(current<points.length-1){let next=current+1,limit=points.length-1;for(let index=current+1;index<points.length;index++)if(points[index].anchor){limit=index;break}for(let candidate=limit;candidate>next;candidate--){const score=knowledgeRouteScore(points[current],points[candidate],grid,edge);if(Number.isFinite(score)&&score<=(costs[candidate]-costs[current])*1.08){next=candidate;break}}simplified.push(points[next]);current=next}
  return simplified;
}
function knowledgeRoundedPath(input, grid, edge) {
  const cross = knowledgeState.parentEdges.get(edge.to) !== edge;
  const pairKey = JSON.stringify([edge.from, edge.to].sort());
  const simple = cross && edge.from !== edge.to && (knowledgeState.routePairs?.get(pairKey) || 1) === 1;
  if (simple && input.length > 2) {
    const direct = branchDirectCurve(grid, edge, input);
    if (direct) return direct;
  }
  const round = point => ({ x: graphPathNumber(point.x), y: graphPathNumber(point.y) });
  const repaired = branchRepairRoute(input, grid, edge);
  const compact = simple ? branchCompactRoute(repaired.points, grid, edge) : repaired.points;
  const points = compact.map(round).filter((point, index, values) => !index || Math.hypot(point.x - values[index - 1].x, point.y - values[index - 1].y) > .01);
  const format = point => point.x + ' ' + point.y;
  if (points.length < 3) return { path: 'M ' + points.map(format).join(' L '), samples: points, curves: [], smoothKind: 'straight', smoothingPasses: 0, pathRepairs: repaired.repairs };
  const handles = points.map((point, index) => {
    const previous = points[Math.max(0, index - 1)], next = points[Math.min(points.length - 1, index + 1)];
    const incoming = Math.hypot(point.x - previous.x, point.y - previous.y), outgoing = Math.hypot(next.x - point.x, next.y - point.y);
    const direction = { x: next.x - previous.x, y: next.y - previous.y };
    const length = Math.hypot(direction.x, direction.y) || 1;
    const reach = (index === 0 ? outgoing : index === points.length - 1 ? incoming : Math.min(incoming, outgoing)) * .34;
    return { x: direction.x / length * reach, y: direction.y / length * reach };
  });
  const obstacles = grid.obstacles.filter(obstacle => obstacle.id !== edge.from && obstacle.id !== edge.to).map(obstacle => {
    let originalClearance = Infinity;
    for (let index = 1; index < points.length; index++) originalClearance = Math.min(originalClearance, knowledgeSegmentDistance(obstacle, points[index - 1], points[index]));
    return { ...obstacle, clearance: Math.max(obstacle.radius + 3, Math.min(obstacle.radius + 12, originalClearance - .5)) };
  });
  const makeCurves = () => points.slice(1).map((end, index) => {
    const start = points[index], firstHandle = round(handles[index]), secondHandle = round(handles[index + 1]);
    const curve = { start, first: round({ x: start.x + firstHandle.x, y: start.y + firstHandle.y }), second: round({ x: end.x - secondHandle.x, y: end.y - secondHandle.y }), end };
    return { ...curve, samples: branchSampleCubic(curve) };
  });
  const blocked = curve => {
    const left = Math.min(curve.start.x, curve.first.x, curve.second.x, curve.end.x);
    const right = Math.max(curve.start.x, curve.first.x, curve.second.x, curve.end.x);
    const top = Math.min(curve.start.y, curve.first.y, curve.second.y, curve.end.y);
    const bottom = Math.max(curve.start.y, curve.first.y, curve.second.y, curve.end.y);
    return obstacles.some(obstacle => {
      if (obstacle.x + obstacle.clearance < left || obstacle.x - obstacle.clearance > right || obstacle.y + obstacle.clearance < top || obstacle.y - obstacle.clearance > bottom) return false;
      for (let index = 1; index < curve.samples.length; index++) {
        if (knowledgeSegmentDistance(obstacle, curve.samples[index - 1], curve.samples[index]) < obstacle.clearance) return true;
      }
      return false;
    });
  };
  let curves = [], smoothingPasses = 0;
  for (let pass = 0; pass < 18; pass++) {
    curves = makeCurves();
    const conflicts = curves.map((curve, index) => blocked(curve) ? index : -1).filter(index => index >= 0);
    if (!conflicts.length) break;
    const adjusted = new Set(conflicts.flatMap(index => [index, index + 1]));
    for (const index of adjusted) { handles[index].x *= .55; handles[index].y *= .55; }
    smoothingPasses++;
  }
  curves = makeCurves();
  const path = 'M ' + format(points[0]) + curves.map(curve => ' C ' + format(curve.first) + ' ' + format(curve.second) + ' ' + format(curve.end)).join('');
  return { path, samples: [points[0], ...curves.flatMap(curve => curve.samples.slice(1))], curves, smoothKind: 'cubic', smoothingPasses, pathRepairs: repaired.repairs };
}
function knowledgeReserveRoute(route,grid){const occupied=new Map();for(let index=1;index<route.samples.length;index++){const start=route.samples[index-1],end=route.samples[index],count=Math.max(1,Math.ceil(Math.hypot(end.x-start.x,end.y-start.y)/(grid.step*.4))),direction=(Math.round(Math.atan2(end.y-start.y,end.x-start.x)/(Math.PI/4))+8)%8;for(let step=0;step<=count;step++){const ratio=step/count,cell=knowledgeGridIndex({x:start.x+(end.x-start.x)*ratio,y:start.y+(end.y-start.y)*ratio},grid);occupied.set(cell,(occupied.get(cell)||0)|(1<<direction))}}for(const [index,mask]of occupied){grid.traffic[index]++;grid.headings[index]|=mask}}
function knowledgeGraphRoutes(edges,positions){
knowledgeState.routePairs = new Map();
for (const edge of edges) {
    const key = JSON.stringify([edge.from, edge.to].sort());
    knowledgeState.routePairs.set(key, (knowledgeState.routePairs.get(key) || 0) + 1);
  }

  const grid=knowledgeRoutingGrid(positions),valid=edges.filter(edge=>positions.has(edge.from)&&positions.has(edge.to)),routes=[],pairs=new Map(),pairKey=edge=>JSON.stringify([edge.from,edge.to].sort()),length=edge=>{const from=grid.byId.get(edge.from),to=grid.byId.get(edge.to);return Math.hypot(to.x-from.x,to.y-from.y)};for(const edge of valid){const key=pairKey(edge);if(!pairs.has(key))pairs.set(key,{count:0,index:0});pairs.get(key).count++}valid.sort((first,second)=>length(first)-length(second)||first.from.localeCompare(second.from)||first.to.localeCompare(second.to)||first.kind.localeCompare(second.kind));
  for(const edge of valid){const from=grid.byId.get(edge.from),to=grid.byId.get(edge.to);let points;
    if(edge.from===edge.to){const radius=(from.radius+70+routes.filter(route=>route.edge.from===edge.from&&route.edge.to===edge.to).length*28)*.8;let best=Infinity;for(let direction=0;direction<16;direction++){const angle=direction*Math.PI/8,candidate=[[0,0],[radius,-radius],[radius*1.4,0],[radius,radius],[0,0]].map(([deltaX,deltaY])=>({x:from.x+deltaX*Math.cos(angle)-deltaY*Math.sin(angle),y:from.y+deltaX*Math.sin(angle)+deltaY*Math.cos(angle)}));let score=0;for(let index=1;index<candidate.length;index++)score+=knowledgeRouteScore(candidate[index-1],candidate[index],grid,edge);if(!points||score<best){points=candidate;best=score}}}
    else{const pair=pairs.get(pairKey(edge));if(pair.count>1){const distance=length(edge),offset=(pair.index++-(pair.count-1)/2)*64*(edge.from<edge.to?1:-1),desired={x:(from.x+to.x)/2-(to.y-from.y)/distance*offset,y:(from.y+to.y)/2+(to.x-from.x)/distance*offset};let waypoint=null,best=Infinity;const cell=knowledgeGridIndex(desired,grid),column=cell%grid.width,row=Math.floor(cell/grid.width);for(let deltaY=-5;deltaY<=5;deltaY++)for(let deltaX=-5;deltaX<=5;deltaX++){const nextColumn=column+deltaX,nextRow=row+deltaY;if(nextColumn<1||nextColumn>=grid.width-1||nextRow<1||nextRow>=grid.height-1)continue;const index=nextRow*grid.width+nextColumn;if(grid.owner[index])continue;const point=knowledgeGridPoint(index,grid),score=Math.hypot(point.x-desired.x,point.y-desired.y);if(score<best){best=score;waypoint={...point,index:0,anchor:true}}}if(waypoint){const first=knowledgeGridPath(from,waypoint,grid),second=knowledgeGridPath(waypoint,to,grid);if(first&&second)points=[...first.slice(0,-1),waypoint,...second.slice(1)]}}
      if(!points){const direct=knowledgeRouteScore(from,to,grid,edge),distance=length(edge)/grid.step;points=direct<=distance*1.05?[from,to]:knowledgeGridPath(from,to,grid)}if(!points)points=[from,{x:from.x,y:24},{x:to.x,y:24},to];}
    if(edge.from!==edge.to)points=knowledgeSimplifyPath(points,grid,edge);while(points.length>2&&Math.hypot(points[1].x-from.x,points[1].y-from.y)<from.radius+8)points.splice(1,1);while(points.length>2&&Math.hypot(points.at(-2).x-to.x,points.at(-2).y-to.y)<to.radius+8)points.splice(points.length-2,1);
    const source={...from.position,id:from.id},target={...to.position,id:to.id};points[0]=knowledgeNodeBoundary(source,points[1]);points[points.length-1]=knowledgeNodeBoundary(target,points.at(-2),graphArrowGap);const route={edge,plan:{kind:'knowledge'},...knowledgeRoundedPath(points,grid,edge)};routes.push(route);knowledgeReserveRoute(route,grid);
  }
  knowledgeState.routes = routes;
return routes;
}
function updateGraphViewControl(){const knowledge=graphView==='knowledge',label=t(knowledge?'switchToPlane':'switchToKnowledge'),button=el('graph-view-toggle');button.title=label;button.setAttribute('aria-label',label);button.setAttribute('aria-pressed',String(knowledge));el('graph-shell').dataset.graphView=graphView}
function toggleGraphView(){graphViewStates.set(graphView,{scale,panX,panY});graphView=graphView==='plane'?'knowledge':'plane';renderGraph({fit:false,preserveSelection:true});const previous=graphViewStates.get(graphView);if(previous){scale=previous.scale;panX=previous.panX;panY=previous.panY;transform()}else fitGraph()}
function graphGutterBow(span){return Math.min(420,Math.max(96,Math.abs(span)*.18))}
function alignGraphRank(value,kind){const parity=kind==='exploration'?1:0;return value%2===parity?value:value+1}
function planGraphCurves(edges,rank){const plans=new Map(),lanes={above:[],below:[]};let alternate=0;const longEdges=edges.filter(edge=>Math.abs(rank.get(edge.to)-rank.get(edge.from))>1).sort((a,b)=>Math.min(rank.get(a.from),rank.get(a.to))-Math.min(rank.get(b.from),rank.get(b.to))||Math.max(rank.get(b.from),rank.get(b.to))-Math.max(rank.get(a.from),rank.get(a.to))||a.kind.localeCompare(b.kind)||a.from.localeCompare(b.from)||a.to.localeCompare(b.to));for(const edge of longEdges){const start=Math.min(rank.get(edge.from),rank.get(edge.to)),end=Math.max(rank.get(edge.from),rank.get(edge.to)),option=side=>{const values=lanes[side],reusable=values.findIndex(last=>last<start),lane=reusable<0?values.length:reusable;return{side,lane,adds:lane===values.length?1:0}};const above=option('above'),below=option('below');let selected;if(above.adds!==below.adds)selected=above.adds<below.adds?above:below;else if(lanes.above.length!==lanes.below.length)selected=lanes.above.length<lanes.below.length?above:below;else selected=alternate++%2===0?above:below;lanes[selected.side][selected.lane]=end;plans.set(edge,{kind:'gutter',side:selected.side,lane:selected.lane,span:end-start})}return{plans,above:lanes.above.length,below:lanes.below.length}}
function planSameColumnLanes(edges,positions,plans){const groups=new Map();for(const edge of edges){const from=positions.get(edge.from),to=positions.get(edge.to);if(!from||!to||from.level!==to.level||byId.get(edge.from)?.kind!=='cognition'||byId.get(edge.to)?.kind!=='cognition')continue;const start=Math.min(from.y+from.height/2,to.y+to.height/2),end=Math.max(from.y+from.height/2,to.y+to.height/2);if(!groups.has(from.level))groups.set(from.level,[]);groups.get(from.level).push({edge,start,end})}for(const entries of groups.values()){const lanes=[];entries.sort((a,b)=>a.start-b.start||a.end-b.end||a.edge.from.localeCompare(b.edge.from)||a.edge.to.localeCompare(b.edge.to));for(const entry of entries){let lane=lanes.findIndex(end=>end+16<entry.start);if(lane<0)lane=lanes.length;lanes[lane]=entry.end;plans.set(entry.edge,{kind:'same-column',lane})}}}
function graphColumnShift(level,contentHeight,columnHeight){const shifts=[0,-58,46,-24,68,-44,26,-66,38],limit=Math.max(0,(contentHeight-columnHeight)/2-52),wanted=shifts[((level%shifts.length)+shifts.length)%shifts.length];return Math.max(-limit,Math.min(limit,wanted))}
function graphLayout(nodes,edges){
  const byNodeID=new Map(nodes.map(node=>[node.id,node])),ids=new Set(byNodeID.keys()),validEdges=edges.filter(edge=>ids.has(edge.from)&&ids.has(edge.to));const incoming=new Map(nodes.map(node=>[node.id,[]])),outgoing=new Map(nodes.map(node=>[node.id,[]])),outgoingEdges=new Map(nodes.map(node=>[node.id,[]])),indegree=new Map(nodes.map(node=>[node.id,0]));
  for(const edge of validEdges){outgoing.get(edge.from).push(edge.to);incoming.get(edge.to).push(edge.from);outgoingEdges.get(edge.from).push(edge);indegree.set(edge.to,indegree.get(edge.to)+1)}
  const rank=new Map(nodes.map(node=>[node.id,node.kind==='exploration'?1:0]));const queue=nodes.filter(node=>indegree.get(node.id)===0).sort(graphNodeCompare);let processed=0;
  while(queue.length){const node=queue.shift();processed++;for(const edge of outgoingEdges.get(node.id)){const target=byNodeID.get(edge.to),candidate=edge.kind==='extends'&&node.kind==='cognition'&&target.kind==='cognition'?rank.get(node.id):alignGraphRank(rank.get(node.id)+1,target.kind);if(candidate>rank.get(target.id))rank.set(target.id,candidate);indegree.set(target.id,indegree.get(target.id)-1);if(indegree.get(target.id)===0){queue.push(target);queue.sort(graphNodeCompare)}}}
  if(processed<nodes.length){for(const node of nodes.filter(item=>indegree.get(item.id)>0))rank.set(node.id,alignGraphRank(rank.get(node.id),node.kind))}
  const columns=new Map();for(const node of nodes){const level=rank.get(node.id);if(!columns.has(level))columns.set(level,[]);columns.get(level).push(node)}const levels=[...columns.keys()].sort((a,b)=>a-b);for(const level of levels)columns.get(level).sort(graphNodeCompare);
  const width=250,columnGap=graphColumnGap,rowGap=72,padding=52,horizontalPadding=96,heights=new Map(nodes.map(node=>[node.id,graphNodeHeight(node)]));const columnHeights=new Map(levels.map(level=>[level,columns.get(level).reduce((total,node)=>total+heights.get(node.id),0)+Math.max(0,columns.get(level).length-1)*rowGap])),contentHeight=Math.max(540,padding*2+Math.max(0,...columnHeights.values())),curves=planGraphCurves(validEdges,rank),curveGap=36,curvePadding=36+Math.max(0,...[...curves.plans.values()].map(plan=>graphGutterBow(plan.span*(width+columnGap)))),topGutter=curves.above?curvePadding+curves.above*curveGap:0,bottomGutter=curves.below?curvePadding+curves.below*curveGap:0;stageHeight=topGutter+contentHeight+bottomGutter;const columnStart=level=>topGutter+(contentHeight-columnHeights.get(level))/2+graphColumnShift(level,contentHeight,columnHeights.get(level));
  const centers=()=>{const result=new Map();for(const level of levels){let y=columnStart(level);for(const node of columns.get(level)){const height=heights.get(node.id);result.set(node.id,{level,y:y+height/2});y+=height+rowGap}}return result};
  const reorder=(level,neighbors,direction)=>{const current=centers(),items=columns.get(level),before=items.map(node=>node.id).join('|');const score=node=>{const values=neighbors.get(node.id).map(id=>current.get(id)).filter(value=>value&&(direction==='left'?value.level<level:value.level>level));return values.length?values.reduce((sum,value)=>sum+value.y,0)/values.length:null};const scores=new Map(items.map(node=>[node.id,score(node)]));items.sort((a,b)=>{const x=scores.get(a.id),y=scores.get(b.id);if(x!==null&&y!==null&&x!==y)return x-y;if(x!==null&&y===null)return -1;if(x===null&&y!==null)return 1;return graphNodeCompare(a,b)});return before!==items.map(node=>node.id).join('|')};
  for(let pass=0;pass<8;pass++){let changed=false;for(const level of levels.slice(1))changed=reorder(level,incoming,'left')||changed;for(const level of levels.slice(0,-1).reverse())changed=reorder(level,outgoing,'right')||changed;if(!changed)break}
  stageWidth=Math.max(880,horizontalPadding*2+(Math.max(0,...levels)+1)*width+Math.max(0,...levels)*columnGap);const positions=new Map();for(const level of levels){let y=columnStart(level);for(const node of columns.get(level)){const height=heights.get(node.id);positions.set(node.id,{x:horizontalPadding+level*(width+columnGap),y,width,height,level});y+=height+rowGap}}
  planSameColumnLanes(validEdges,positions,curves.plans);const maxSameLane=Math.max(-1,...[...curves.plans.values()].filter(plan=>plan.kind==='same-column').map(plan=>plan.lane)),requiredHorizontalPadding=72+Math.max(0,maxSameLane)*18;if(requiredHorizontalPadding>horizontalPadding){const shift=requiredHorizontalPadding-horizontalPadding;for(const position of positions.values())position.x+=shift;stageWidth+=shift*2}for(const curve of curves.plans.values())if(curve.kind==='gutter')curve.y=curve.side==='above'?topGutter-(curve.lane+1)*curveGap:topGutter+contentHeight+(curve.lane+1)*curveGap;
  return{positions,incoming,outgoing,columns,curvePlan:curves.plans};
}
function svgElement(name,attributes={}){const node=document.createElementNS('http://www.w3.org/2000/svg',name);for(const [key,value] of Object.entries(attributes))node.setAttribute(key,String(value));return node}
function appendMetalGradient(defs,id,stops){const gradient=svgElement('linearGradient',{id,x1:'0%',y1:'0%',x2:'100%',y2:'100%'});for(const [offset,color] of stops)gradient.append(svgElement('stop',{offset,'stop-color':color}));defs.append(gradient)}
function graphDefinitions(){const defs=svgElement('defs'),marker=svgElement('marker',{id:'arrow-relation',viewBox:'0 0 10 10',refX:'10',refY:'5',markerWidth:'6',markerHeight:'6',orient:'auto'});marker.append(svgElement('path',{d:'M 0 0 L 10 5 L 0 10 z',fill:'#a5aeaa'}));defs.append(marker);appendMetalGradient(defs,'cognition-bronze',[['0%','#e5d2bf'],['18%','#ead9c8'],['38%','#f6ede3'],['55%','#e6d3c0'],['76%','#f0e3d5'],['100%','#dcc2a8']]);appendMetalGradient(defs,'cognition-silver',[['0%','#e1e4e6'],['18%','#e8eaec'],['38%','#fafafa'],['55%','#e4e7e9'],['76%','#f0f1f2'],['100%','#d9dde0']]);appendMetalGradient(defs,'cognition-gold',[['0%','#eadca8'],['18%','#efe4bb'],['38%','#faf4dc'],['55%','#ecdfad'],['76%','#f5ebcb'],['100%','#dfcc89']]);return defs}
const graphIconPaths={
  'task-cognition':['M12 3l1.4 4.1 4.1 1.4-4.1 1.4L12 14l-1.4-4.1-4.1-1.4 4.1-1.4L12 3Z','M19 15l.7 2.3L22 18l-2.3.7L19 21l-.7-2.3L16 18l2.3-.7L19 15Z'],
  regular:['M9 18h6','M10 22h4','M15.1 14c.2-.7.7-1.2 1.2-1.7A6 6 0 1 0 7.7 12.3c.5.5 1 1 1.2 1.7h6.2Z'],
  'result-cognition':['M5 21V4','M5 4h10l-1.5 4L15 12H5'],
  todo:['M12 3a9 9 0 1 1-9 9','M12 7v5l3 2'],
  running:['M21 12a9 9 0 0 1-9 9','M3 12a9 9 0 0 1 9-9'],
  finished:['M22 11.1V12a10 10 0 1 1-5.9-9.1','m9 11 3 3L22 4']
};
function createGraphIcon(variant){const icon=svgElement('svg',{class:'node-icon',viewBox:'0 0 24 24','aria-hidden':'true'}),glyph=svgElement('g',{class:'node-icon-glyph'});for(const value of graphIconPaths[variant]||[])glyph.append(svgElement('path',{d:value}));icon.append(glyph);return icon}
function knowledgeGraphGuides(positions) {
  const guides = svgElement('g', { class: 'knowledge-guides branch-guides', 'aria-hidden': 'true' });
  for (const branch of knowledgeState.branches) {
    const points = branch.ids.flatMap(id => {
      const position = positions.get(id), centerX = position.x + position.width / 2, centerY = position.y + position.height / 2;
      return [{ x: centerX - 90, y: centerY - 55 }, { x: centerX + 90, y: centerY - 55 }, { x: centerX + 90, y: centerY + 100 }, { x: centerX - 90, y: centerY + 100 }];
    });
    const hull = branchHull(points);
    if (!hull.length) continue;
    const midpoint = (first, second) => ({ x: (first.x + second.x) / 2, y: (first.y + second.y) / 2 });
    const start = midpoint(hull.at(-1), hull[0]);
    let path = 'M ' + start.x + ' ' + start.y;
    for (let index = 0; index < hull.length; index++) { const end = midpoint(hull[index], hull[(index + 1) % hull.length]); path += ' Q ' + hull[index].x + ' ' + hull[index].y + ' ' + end.x + ' ' + end.y; }
    guides.append(svgElement('path', { class: 'branch-canopy', d: path + ' Z', fill: branch.color, stroke: branch.color, 'data-branch': branch.id }));
  }
  return guides;
}
function knowledgeGraphDefinitions(defs){const glow=svgElement('radialGradient',{id:'knowledge-center-glow'});glow.append(svgElement('stop',{offset:'0%','stop-color':'#bad4ff','stop-opacity':'.4'}),svgElement('stop',{offset:'100%','stop-color':'#e8f1ff','stop-opacity':'0'}));defs.append(glow);for(const [theme,{color}]of Object.entries(knowledgeThemes)){const marker=svgElement('marker',{id:'knowledge-arrow-'+theme,viewBox:'0 0 10 10',refX:'10',refY:'5',markerWidth:'5',markerHeight:'5',orient:'auto'});marker.append(svgElement('path',{d:'M 1 1 L 9 5 L 1 9 z',fill:color}));defs.append(marker)}}
function appendKnowledgeLabel(item,node,position){
  const label=svgElement('g',{class:'knowledge-label',transform:'translate('+position.width/2+' '+position.height/2+')'}),frame=svgElement('foreignObject',{class:'knowledge-label-frame',x:-94,y:position.height/2*scale+8,width:188,height:112});
  const wrap=document.createElementNS('http://www.w3.org/1999/xhtml','div');wrap.setAttribute('class','knowledge-label-wrap');
  const copy=document.createElementNS('http://www.w3.org/1999/xhtml','div');copy.setAttribute('class','knowledge-label-copy');const text=document.createElementNS('http://www.w3.org/1999/xhtml','div');text.setAttribute('class','knowledge-label-text');text.textContent=node.title||t('untitledNode');copy.append(text);wrap.append(copy);frame.append(wrap);label.append(svgElement('line',{class:'knowledge-label-link','aria-hidden':'true'}),frame);item.append(label);
}

function knowledgeLabelPlacement(center,size,obstacles,occupied,bounds,preferred){
  const directions=[[1,0],[-1,0],[0,1],[0,-1],[1,1],[-1,1],[1,-1],[-1,-1],[2,1],[-2,1],[2,-1],[-2,-1],[1,2],[-1,2],[1,-2],[-1,-2]],length=Math.hypot(preferred.x,preferred.y)||1;
  let best=null;
  for(const gap of [10,26,44])for(const [horizontal,vertical]of directions){
    const directionLength=Math.hypot(horizontal,vertical),offsetX=center.radius*horizontal/directionLength+gap*horizontal,offsetY=center.radius*vertical/directionLength+gap*vertical;
    const candidateX=horizontal>0?center.x+offsetX:horizontal<0?center.x+offsetX-size.width:center.x-size.width/2,candidateY=vertical>0?center.y+offsetY:vertical<0?center.y+offsetY-size.height:center.y-size.height/2;
    const left=Math.max(bounds.left,Math.min(candidateX,bounds.right-size.width)),top=Math.max(bounds.top,Math.min(candidateY,bounds.bottom-size.height));
    const rect={x:left,y:top,width:size.width,height:size.height},right=left+size.width,bottom=top+size.height;
    let score=gap*.4+(1-(horizontal*preferred.x+vertical*preferred.y)/(Math.hypot(horizontal,vertical)*length))*12;
    score+=(Math.max(0,bounds.left-left)+Math.max(0,right-bounds.right)+Math.max(0,bounds.top-top)+Math.max(0,bottom-bounds.bottom))*1000;
    for(const obstacle of obstacles){const nearestX=Math.max(left,Math.min(obstacle.x,right)),nearestY=Math.max(top,Math.min(obstacle.y,bottom)),clearance=Math.hypot(nearestX-obstacle.x,nearestY-obstacle.y);if(clearance<obstacle.radius+5)score+=100000+(obstacle.radius+5-clearance)**2*30}
    for(const other of occupied){const width=Math.min(right+4,other.x+other.width+4)-Math.max(left-4,other.x-4),height=Math.min(bottom+4,other.y+other.height+4)-Math.max(top-4,other.y-4);if(width>0&&height>0)score+=10000+width*height*20}
    if(!best||score<best.score)best={...rect,score};
  }
  return best;
}
function layoutKnowledgeLabels() {
  if (graphView !== 'knowledge') return;
  const viewport = el('viewport'), legend = viewport.querySelector('.knowledge-legend');
  const bounds = { left: 16, right: viewport.clientWidth - 16, top: (legend?.offsetTop || 14) + (legend?.offsetHeight || 44) + 12, bottom: viewport.clientHeight - (el('branch-dock')?.offsetHeight || 110) - 28 };
  const obstacles = [...nodePositions].map(([id, position]) => ({ id, x: (position.x + position.width / 2) * scale + panX, y: (position.y + position.height / 2) * scale + panY, radius: position.width / 2 * scale }));
  const byObstacle = new Map(obstacles.map(obstacle => [obstacle.id, obstacle]));
  const occupied = [], entries = [];
  const overview = scale < .26;
  const important = id => id === selectedNodeId ? 100000 : id === knowledgeState.hoveredId || nodeElements.get(id).matches(':focus') ? 90000
    : nodeElements.get(id).classList.contains('search-match') ? 80000 : id === knowledgeState.root ? 70000
    : knowledgeState.parent.get(id) === knowledgeState.root ? 60000 : nodeElements.get(id).classList.contains('related') || nodeElements.get(id).classList.contains('preview-related') ? 50000
    : ((knowledgeState.children.get(id)?.length || 0) > 1 ? 20000 : byId.get(id).kind === 'exploration' ? 10000 : 0) + Math.min(5000, (knowledgeState.subtreeSize.get(id) || 0) * 20);
  const sorted = [...nodeElements].sort((first, second) => important(second[0]) - important(first[0]) || knowledgeCreatedCompare(byId.get(first[0]), byId.get(second[0])));
  if (!knowledgeLabelMeasure) knowledgeLabelMeasure = document.createElement('canvas').getContext('2d');
  for (const [id, item] of sorted) {
    const label = item.querySelector('.knowledge-label');
    if (!label) continue;
    const center = byObstacle.get(id);
    if (center.x < -center.radius || center.x > viewport.clientWidth + center.radius || center.y < -center.radius || center.y > viewport.clientHeight + center.radius
      || knowledgeState.activeBranch && knowledgeState.branchOf.get(id) !== knowledgeState.activeBranch && id !== selectedNodeId) {
      label.setAttribute('visibility', 'hidden');
      continue;
    }
    const expanded = id === selectedNodeId || id === knowledgeState.hoveredId || item.matches(':focus');
    const copy = label.querySelector('.knowledge-label-copy'), frame = label.querySelector('.knowledge-label-frame');
    const style = getComputedStyle(copy);
    knowledgeLabelMeasure.font = style.fontWeight + ' ' + style.fontSize + ' ' + style.fontFamily;
    const textWidth = knowledgeLabelMeasure.measureText(copy.textContent).width;
    const landmark = id === knowledgeState.root || knowledgeState.parent.get(id) === knowledgeState.root || (knowledgeState.children.get(id)?.length || 0) > 1;
    const compact = overview && !expanded && !landmark;
    const maximum = Math.min(expanded ? 310 : id === knowledgeState.root ? 210 : compact ? 118 : overview ? 138 : 172, viewport.clientWidth - 40);
    const width = Math.ceil(Math.min(maximum, Math.max(62, textWidth + 18)));
    copy.style.width = width + 'px';
    copy.style.setProperty('--knowledge-label-lines', expanded ? '6' : compact ? '1' : '2');
    frame.setAttribute('width', String(width));
    entries.push({ id, item, label, frame, copy, center, expanded });
  }
  const measured = entries.map(entry => ({ ...entry, size: entry.copy.getBoundingClientRect() }));
  for (const entry of measured) {
    const { id, label, frame, center, size, expanded } = entry;
    let best = null;
    const candidates = branchLabelCandidates(center, size, bounds);
    const anchor = knowledgeState.hoveredId && (id === knowledgeState.hoveredId ? knowledgeState.hoverAnchor : knowledgeState.labelPlacements.get(id));
    if (anchor) {
      const rect = { x: Math.max(bounds.left, Math.min(anchor.x, bounds.right - size.width)), y: Math.max(bounds.top, Math.min(anchor.y, bounds.bottom - size.height)), width: size.width, height: size.height };
      candidates.unshift({ ...rect, inside: rect.x >= bounds.left && rect.y >= bounds.top && rect.x + rect.width <= bounds.right && rect.y + rect.height <= bounds.bottom });
    }
    for (const [index, rect] of candidates.entries()) {
      if (!rect.inside) continue;
      const nodeCollision = obstacles.some(obstacle => {
        const nearestX = Math.max(rect.x, Math.min(obstacle.x, rect.x + rect.width));
        const nearestY = Math.max(rect.y, Math.min(obstacle.y, rect.y + rect.height));
        return Math.hypot(nearestX - obstacle.x, nearestY - obstacle.y) < obstacle.radius + 3;
      });
      const overlap = occupied.some(other => branchRectsOverlap(rect, other));
      const score = index + (nodeCollision ? 1000 : 0) + (overlap ? 10000 : 0);
      if (!best || score < best.score) best = { ...rect, score };
    }
    if ((!best || best.score >= 1000) && (expanded || id === knowledgeState.root)) {
      const arranged = knowledgeLabelPlacement(center, size, obstacles, occupied, bounds, { x: -1, y: -.35 });
      if (arranged && arranged.score < 1000) best = arranged;
    }
    if ((!best || best.score >= 1000) && (expanded || id === knowledgeState.root)) {
      const preferred = best || { x: center.x - size.width / 2, y: center.y + center.radius + 8 };
      const rect = { x: Math.max(bounds.left, Math.min(preferred.x, bounds.right - size.width)), y: Math.max(bounds.top, Math.min(preferred.y, bounds.bottom - size.height)), width: size.width, height: size.height };
      best = { ...rect, score: 0 };
      for (let index = occupied.length - 1; index >= 0; index--) {
        if (branchRectsOverlap(rect, occupied[index])) { occupied[index].label.setAttribute('visibility', 'hidden'); occupied.splice(index, 1); }
      }
    }
    if (!best || best.score >= 1000) { label.setAttribute('visibility', 'hidden'); continue; }
    label.setAttribute('visibility', 'visible');
    frame.setAttribute('height', String(Math.ceil(size.height)));
    frame.setAttribute('x', String(best.x - center.x));
    frame.setAttribute('y', String(best.y - center.y));
    const link = label.querySelector('.knowledge-label-link');
    const deltaX = Math.max(best.x, Math.min(center.x, best.x + best.width)) - center.x;
    const deltaY = Math.max(best.y, Math.min(center.y, best.y + best.height)) - center.y;
    const distance = Math.hypot(deltaX, deltaY) || 1;
    link.setAttribute('x1', String(deltaX / distance * (center.radius + 2) / scale));
    link.setAttribute('y1', String(deltaY / distance * (center.radius + 2) / scale));
    link.setAttribute('x2', String(deltaX / scale));
    link.setAttribute('y2', String(deltaY / scale));
    occupied.push({ ...best, label, id });
  }
  knowledgeState.labelCount = occupied.length;
  knowledgeState.labelPlacements = new Map(occupied.map(rect => [rect.id, { x: rect.x, y: rect.y, width: rect.width, height: rect.height }]));
  knowledgeState.labelStats = { internal: occupied.filter(entry => knowledgeState.children.get(entry.id)?.length).length, overview };
  branchUpdateStatus();
}
function scheduleKnowledgeLabels(){
  if(knowledgeLabelFrame!==null)cancelAnimationFrame(knowledgeLabelFrame);
  knowledgeLabelFrame=requestAnimationFrame(()=>{knowledgeLabelFrame=null;layoutKnowledgeLabels()});
}
function previewKnowledgeNode(id){
  if(graphView!=='knowledge'||selectedNodeId)id=null;
  const related=new Set(id?[id]:[]);for(const item of edgeElements){if(item.edge.from===id)related.add(item.edge.to);if(item.edge.to===id)related.add(item.edge.from)}
  for(const [nodeId,item]of nodeElements){item.classList.toggle('previewed',nodeId===id);item.classList.toggle('preview-related',!!id&&related.has(nodeId)&&nodeId!==id);item.classList.toggle('preview-unrelated',!!id&&!related.has(nodeId))}
  for(const item of edgeElements){const connected=!!id&&(item.edge.from===id||item.edge.to===id);item.path.classList.toggle('preview-related',connected);item.path.classList.toggle('preview-unrelated',!!id&&!connected)}
  scheduleKnowledgeLabels();
}
function addNode(node,position,nodeLayer){const variant=node.kind==='cognition'?cognitionVariant(node):node.status;const item=svgElement('g',{class:'node '+node.kind+' '+variant,transform:'translate('+position.x+' '+position.y+')',tabindex:'0',role:'button','aria-label':node.title});item.dataset.id=node.id;if(graphView==='knowledge'&&position.root)item.classList.add('knowledge-root');const tooltip=svgElement('title');tooltip.textContent=node.title;item.append(tooltip);let shape,ring=null;if(graphView==='knowledge'){shape=knowledgeNodeShape(node,position,14,'node-shape');ring=knowledgeNodeShape(node,position,5,'knowledge-ring');if(node.kind==='cognition')ring.setAttribute('style','stroke:url(#'+knowledgeThemes[knowledgeNodeTheme(node)].metal+')');if(position.root)item.append(svgElement('circle',{class:'knowledge-root-halo',cx:position.width/2,cy:position.height/2,r:position.width/2+12}),svgElement('circle',{class:'knowledge-root-orbit',cx:position.width/2,cy:position.height/2,r:position.width/2+24}))}else if(node.kind==='cognition')shape=svgElement('rect',{class:'node-shape',width:position.width,height:position.height,rx:7});else{shape=svgElement('path',{class:'node-shape',d:'M 0 0 H '+(position.width-36)+' L '+position.width+' '+(position.height/2)+' L '+(position.width-36)+' '+position.height+' H 0 Z'});ring=svgElement('path',{class:'node-status-ring',d:'M 7 7 H '+(position.width-40)+' L '+(position.width-9)+' '+(position.height/2)+' L '+(position.width-40)+' '+(position.height-7)+' H 7 Z'})}const foreign=svgElement('foreignObject',{width:position.width,height:position.height});const copy=document.createElementNS('http://www.w3.org/1999/xhtml','div');copy.setAttribute('class','node-copy');const heading=document.createElementNS('http://www.w3.org/1999/xhtml','div');heading.setAttribute('class','node-heading');const kind=document.createElementNS('http://www.w3.org/1999/xhtml','div');kind.setAttribute('class','node-kind');kind.textContent=node.kind==='cognition'?cognitionLabel(node):graphView==='knowledge'?(statusNames[node.status]||node.status):t('exploration')+' · '+explorationStateLabel(node);heading.append(createGraphIcon(variant),kind);const title=document.createElementNS('http://www.w3.org/1999/xhtml','div');title.setAttribute('class','node-title');title.textContent=node.title;copy.append(heading,title);foreign.append(copy);const input=svgElement('circle',{class:'node-port in',cx:0,cy:position.height/2,r:4}),output=svgElement('circle',{class:'node-port out',cx:position.width,cy:position.height/2,r:4});const outline=shape.cloneNode(false);outline.setAttribute('class','node-selection-outline');item.append(outline,shape);if(ring)item.append(ring);item.append(foreign,input,output);if(graphView==='knowledge'){appendKnowledgeLabel(item,node,position);item.addEventListener('focus',()=>previewKnowledgeNode(node.id));item.addEventListener('blur',()=>previewKnowledgeNode(null))}const activate=()=>focusNode(node.id);item.addEventListener('click',event=>{event.stopPropagation();activate()});item.addEventListener('keydown',event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();activate()}});nodeLayer.append(item);nodeElements.set(node.id,item);nodePortElements.set(node.id,[input,output])
if(graphView==='knowledge'){item.dataset.branch=knowledgeState.branchOf.get(node.id)||'root';item.dataset.depth=String(knowledgeState.depth.get(node.id)||0);item.classList.toggle('branch-entry',knowledgeState.parent.get(node.id)===knowledgeState.root)}
}
function nodePortX(position,side){return side==='out'?position.x+position.width:position.x-graphArrowGap}
function graphRoutes(edges,positions,curvePlan){const routes=[];for(const edge of edges){const from=positions.get(edge.from),to=positions.get(edge.to);if(!from||!to)continue;routes.push({edge,from,to,sameColumn:from.level===to.level,plan:curvePlan.get(edge)||{kind:'direct'}})}return routes}
function graphPathNumber(value){return Math.round(value*10)/10}
function graphCurvePath(start,end,entryX,exitX,crossY,bow){const direction=exitX>=entryX?1:-1,handle=Math.min(72,Math.max(24,Math.abs(exitX-entryX)*.18)),exitHandle=Math.max(36,Math.abs(entryX-start.x)*.7),entryHandle=Math.max(36,Math.abs(end.x-exitX)*.7),point=value=>graphPathNumber(value.x)+' '+graphPathNumber(value.y);return'M '+point(start)+' C '+point({x:start.x+exitHandle,y:start.y})+', '+point({x:entryX-direction*handle,y:crossY-bow})+', '+point({x:entryX,y:crossY})+' C '+point({x:entryX+direction*handle,y:crossY+bow})+', '+point({x:exitX-direction*handle,y:crossY+bow})+', '+point({x:exitX,y:crossY})+' C '+point({x:exitX+direction*handle,y:crossY-bow})+', '+point({x:end.x-entryHandle,y:end.y})+', '+point(end)}

function graphRoutePath(route){const {from,to}=route,start={x:nodePortX(from,'out'),y:from.y+from.height/2},end={x:nodePortX(to,'in'),y:to.y+to.height/2},lane=route.plan.lane||0;
  if(route.plan.kind==='gutter'){const shoulder=Math.min(graphColumnGap-40,graphColumnGap*.55+lane*8),bow=graphGutterBow(end.x-start.x)*(route.plan.side==='above'?-1:1);return graphCurvePath(start,end,start.x+shoulder,end.x-shoulder,route.plan.y,bow)}
  if(route.sameColumn||end.x<=start.x){const clearance=52+lane*18,targetBelow=end.y>=start.y,crossY=targetBelow?to.y-32:to.y+to.height+32;return graphCurvePath(start,end,Math.max(from.x+from.width,to.x+to.width)+clearance,Math.min(from.x,to.x)-clearance,crossY,targetBelow?-12:12)}
  const bend=Math.max(48,(end.x-start.x)*.3);return'M '+graphPathNumber(start.x)+' '+graphPathNumber(start.y)+' C '+graphPathNumber(start.x+bend)+' '+graphPathNumber(start.y)+', '+graphPathNumber(end.x-bend)+' '+graphPathNumber(end.y)+', '+graphPathNumber(end.x)+' '+graphPathNumber(end.y)
}
function drawEdges(edges,positions,curvePlan){const svg=el('edges');svg.replaceChildren();svg.setAttribute('width',String(stageWidth));svg.setAttribute('height',String(stageHeight));svg.setAttribute('viewBox','0 0 '+stageWidth+' '+stageHeight);const definitions=graphDefinitions();if(graphView==='knowledge')knowledgeGraphDefinitions(definitions);svg.append(definitions);if(graphView==='knowledge')svg.append(knowledgeGraphGuides(positions));const edgeLayer=svgElement('g',{class:'edge-layer'}),nodeLayer=svgElement('g',{class:'node-layer'});svg.append(edgeLayer,nodeLayer);edgeElements=[];
  const routes=graphView==='knowledge'?knowledgeGraphRoutes(edges,positions):graphRoutes(edges,positions,curvePlan);for(const route of routes){const edge=route.edge,description=route.path||graphRoutePath(route),theme=graphView==='knowledge'?knowledgeEdgeTheme(edge):null,marker=theme?'knowledge-arrow-'+theme:'arrow-relation',path=svgElement('path',{class:'edge '+edge.kind+(route.plan.kind==='gutter'?' long-edge '+route.plan.side+'-curve':''),d:description,'marker-end':'url(#'+marker+')'});if(theme)path.setAttribute('style','--edge-color:'+knowledgeThemes[theme].color);edgeLayer.append(path);edgeElements.push({edge,path})}
  if (graphView === 'knowledge') {
    for (const item of edgeElements) {
      const backbone = knowledgeState.parentEdges.get(item.edge.to) === item.edge;
      item.path.classList.add(backbone ? 'backbone' : 'cross-edge');
    }
  }
return nodeLayer
}
function updateSearchStatus(){const querying=el('search').value.trim()!=='';el('search-status').textContent=!querying?'':!searchMatches.length?t('noGraphMatch'):searchIndex>=0?t('searchPosition',{index:searchIndex+1,total:searchMatches.length}):t('searchCount',{count:searchMatches.length});el('search-previous').disabled=el('search-next').disabled=!querying||!searchMatches.length}
function navigateSearch(direction){if(!searchMatches.length)return;const index=searchIndex<0?(direction>0?0:searchMatches.length-1):(searchIndex+direction+searchMatches.length)%searchMatches.length;focusNode(searchMatches[index],{reveal:true})}
function applySearch(){const querying=el('search').value.trim()!=='';searchMatches=[];for(const [id,item] of nodeElements){const matched=graphMatches(byId.get(id));if(querying&&matched)searchMatches.push(id);item.classList.toggle('search-match',querying&&matched);item.classList.toggle('search-dimmed',querying&&!matched);for(const port of nodePortElements.get(id)||[])port.classList.toggle('search-dimmed',querying&&!matched)}searchMatches.sort((first,second)=>nodePositions.get(first).x-nodePositions.get(second).x||nodePositions.get(first).y-nodePositions.get(second).y||first.localeCompare(second));searchIndex=searchMatches.indexOf(selectedNodeId);updateSearchStatus()}
function renderGraph({fit=true,preserveSelection=false}={}){
knowledgeState.hoveredId=null;knowledgeState.hoverAnchor=null;knowledgeState.labelPlacements=new Map();
const selection=preserveSelection?selectedNodeId:null;selectedNodeId=null;el('detail').classList.remove('visible');el('detail-body').className='detail-body detail-empty';el('detail-body').textContent=t('selectNode');visible=[...byId.values()];const edges=data.edges;const layout=graphView==='knowledge'?knowledgeGraphLayout(visible,edges):graphLayout(visible,edges);nodeElements=new Map();nodePortElements=new Map();nodePositions=layout.positions;el('stage').style.width=stageWidth+'px';el('stage').style.height=stageHeight+'px';const nodeLayer=drawEdges(edges,layout.positions,layout.curvePlan);for(const node of visible)addNode(node,layout.positions.get(node.id),nodeLayer);el('graph-empty').hidden=visible.length!==0;updateGraphViewControl();applySearch();if(selection&&nodeElements.has(selection))focusNode(selection);if(fit)requestAnimationFrame(fitGraph)
if(graphView==='knowledge'){branchRenderNavigation();branchApplyFocus()}branchUpdateDock();
}
function neighborsFor(id,direction){return data.edges.filter(edge=>direction==='in'?edge.to===id:edge.from===id).map(edge=>direction==='in'?edge.from:edge.to)}
function relationField(fields,label,ids){if(!ids.length)return;const dt=document.createElement('dt');dt.textContent=label;const dd=document.createElement('dd');dd.className='relation-list';for(const id of new Set(ids)){const button=document.createElement('button');button.type='button';button.className='relation-link';const node=byId.get(id);button.textContent=node?.title||id;if(node?.title){const identifier=document.createElement('small');identifier.textContent=id;button.append(identifier)}button.title=t('locate',{id});button.addEventListener('click',()=>focusNode(id,{reveal:true,center:true}));dd.append(button)}fields.append(dt,dd)}
function identifierField(fields,node){const dt=document.createElement('dt');dt.textContent=node.kind==='exploration'?t('explorationId'):t('cognitionId');const dd=document.createElement('dd');dd.className='relation-list';const value=document.createElement('span');value.className='relation-token';value.textContent=node.id;dd.append(value);fields.append(dt,dd)}
function showDetail(node){const body=el('detail-body');body.className='detail-body';body.replaceChildren();const context=document.createElement('div');context.className='detail-context';const kind=document.createElement('span');kind.className='related-kind';kind.textContent=node.kind==='exploration'?t('exploration'):cognitionLabel(node);context.append(kind);if(node.kind==='exploration')context.append(explorationBadge(node));const title=document.createElement('div');title.className='detail-title';title.textContent=node.title||t('untitledNode');const content=document.createElement('div');content.className='detail-content';content.append(renderMarkdown(node.id,node.content));const reason=node.kind==='exploration'?explorationReason(node):null;if(reason)content.append(reason);const fields=document.createElement('dl');fields.className='fields';identifierField(fields,node);relationField(fields,t('upstream'),neighborsFor(node.id,'in'));relationField(fields,t('downstream'),neighborsFor(node.id,'out'));body.append(context,title,createDetailActions(node),content,fields);const detail=el('detail');detail.classList.add('visible');detail.scrollTop=0}
function detailAction(label,activate){const button=document.createElement('button');button.type='button';button.className='detail-action';button.textContent=label;button.addEventListener('click',event=>{event.stopPropagation();activate()});return button}
function updateSelectionScope(section=el('detail-body')){const mode=section.querySelector('[data-selection-mode]'),note=section.querySelector('.detail-scope-note');if(mode){mode.setAttribute('aria-pressed',String(traceSelection));mode.title=t(traceSelection?'neighborsHint':'fullChainHint')}if(note)note.textContent=t(traceSelection?'scopeFull':'scopeDirect')}
function createDetailActions(node) {
  const section = document.createElement('section');
  section.className = 'detail-actions';
  const primary = document.createElement('div');
  primary.className = 'detail-action-row';
  const mode = detailAction(t('fullChain'), () => {
    traceSelection = !traceSelection;
    applySelection(node.id);
    updateSelectionScope();
    fitGraph();
  });
  mode.setAttribute('data-selection-mode', '');
  primary.append(detailAction(t('focusSelection'), fitSelection), mode);
  section.append(primary);
  if (graphView === 'knowledge') {
    const row = document.createElement('div');
    row.className = 'detail-action-row branch-detail-actions';
    const relations = detailAction(t(knowledgeState.allRelations ? 'readBackbone' : 'emphasizeCrossLinks'), () => {
      knowledgeState.allRelations = !knowledgeState.allRelations;
      el('graph-shell').dataset.allRelations = String(knowledgeState.allRelations);
      relations.setAttribute('aria-pressed', String(knowledgeState.allRelations));
      relations.textContent = t(knowledgeState.allRelations ? 'readBackbone' : 'emphasizeCrossLinks');
    });
    relations.id = 'branch-relations';
    relations.setAttribute('aria-pressed', String(knowledgeState.allRelations));
    const origin = detailAction(t('locateInitialCognition'), () => focusNode(knowledgeState.root, { reveal: true, center: true }));
    origin.id = 'branch-origin';
    row.append(relations, origin);
    section.append(row);
  }
  const exploration = owningExploration(node);
  if (exploration) {
    const navigation = document.createElement('div');
    navigation.className = 'detail-action-row';
    navigation.append(detailAction(t('viewExploration'), () => revealExplorationForGraphNode(node, { scroll: true })));
    const outputs = outputCognitions(exploration.id);
    if (outputs.length) navigation.append(detailAction(t('viewCognitionOutputs'), () => {
      revealExplorationForGraphNode(node);
      openExplorationOutputs(exploration);
    }));
    section.append(navigation);
  }
  const note = document.createElement('p');
  note.className = 'detail-scope-note';
  note.setAttribute('role', 'status');
  section.append(note);
  updateSelectionScope(section);
  return section;
}
function traverse(start,adjacency){const visited=new Set(),queue=[...(adjacency.get(start)||[])];while(queue.length){const id=queue.shift();if(id===start||visited.has(id))continue;visited.add(id);queue.push(...(adjacency.get(id)||[]))}return visited}
function applySelection(id){const shown=new Set(nodeElements.keys());const incoming=new Map([...shown].map(value=>[value,[]])),outgoing=new Map([...shown].map(value=>[value,[]]));for(const item of edgeElements){if(shown.has(item.edge.from)&&shown.has(item.edge.to)){outgoing.get(item.edge.from).push(item.edge.to);incoming.get(item.edge.to).push(item.edge.from)}}const ancestors=traceSelection?traverse(id,incoming):new Set(incoming.get(id)||[]),descendants=traceSelection?traverse(id,outgoing):new Set(outgoing.get(id)||[]);for(const [nodeId,node] of nodeElements){const related=nodeId===id||ancestors.has(nodeId)||descendants.has(nodeId);node.classList.toggle('selected',nodeId===id);node.classList.toggle('related',related&&nodeId!==id);node.classList.toggle('unrelated',!related);for(const port of nodePortElements.get(nodeId)||[])port.classList.toggle('unrelated',!related)}for(const item of edgeElements){const related=!traceSelection?(item.edge.from===id||item.edge.to===id):(ancestors.has(item.edge.from)&&(ancestors.has(item.edge.to)||item.edge.to===id))||((item.edge.from===id||descendants.has(item.edge.from))&&descendants.has(item.edge.to));item.path.classList.toggle('related',related);item.path.classList.toggle('unrelated',!related)}const selected=nodeElements.get(id);if(selected)selected.parentNode.append(selected)}
function centerNode(id){const position=nodePositions.get(id);if(!position)return;const viewport=el('viewport');const x=position.x+position.width/2,y=position.y+position.height/2;panX=viewport.clientWidth/2-x*scale;panY=viewport.clientHeight/2-y*scale;transform()}
function focusNode(id,{reveal=false,center=false}={}){
if(graphView==='knowledge'&&knowledgeState.activeBranch&&knowledgeState.branchOf.get(id)!==knowledgeState.activeBranch){knowledgeState.activeBranch=null;branchApplyFocus()}
const node=byId.get(id);if(!node||!nodeElements.has(id))return;closeExplorationPanel();selectedNodeId=id;previewKnowledgeNode(null);applySelection(id);showDetail(node);scheduleKnowledgeLabels();searchIndex=searchMatches.indexOf(id);updateSearchStatus();if(reveal){const viewport=el('viewport'),position=nodePositions.get(id);scale=Math.max(scale,Math.min(1,Math.max(.01,(viewport.clientWidth-70)/position.width),Math.max(.01,(viewport.clientHeight-70)/position.height)))}if(center||reveal)centerNode(id)}
function clearSelection(){selectedNodeId=null;previewKnowledgeNode(null);searchIndex=-1;updateSearchStatus();for(const node of nodeElements.values())node.classList.remove('selected','related','unrelated');for(const ports of nodePortElements.values())for(const port of ports)port.classList.remove('unrelated');for(const item of edgeElements)item.path.classList.remove('related','unrelated');el('detail').classList.remove('visible');el('detail-body').className='detail-body detail-empty';el('detail-body').textContent=t('selectNode')}
function transform(){
branchClearHover();
el('graph-shell').style.setProperty('--graph-edge-width',(1/scale)+'px');el('graph-shell').style.setProperty('--knowledge-label-scale',String(1/scale));el('graph-shell').dataset.knowledgeDensity=scale<.32?'overview':'detail';el('stage').style.transform='translate('+panX+'px,'+panY+'px) scale('+scale+')';el('zoom-value').value=String(Math.round(scale*100));scheduleKnowledgeLabels()
branchUpdateBackground();
}
function setZoom(next,cx,cy){const old=scale;scale=Math.min(2.5,Math.max(.01,next));const ratio=scale/old;panX=cx-(cx-panX)*ratio;panY=cy-(cy-panY)*ratio;transform()}
function commitZoom(){const value=Number.parseInt(el('zoom-value').value,10);const percent=Number.isFinite(value)?Math.min(250,Math.max(1,value)):Math.round(scale*100);setZoom(percent/100,el('viewport').clientWidth/2,el('viewport').clientHeight/2)}
function fitBounds(bounds,padding={horizontal:35,vertical:35}){const viewport=el('viewport');scale=Math.min(1,Math.max(.01,Math.min((viewport.clientWidth-padding.horizontal*2)/Math.max(1,bounds.width),(viewport.clientHeight-padding.vertical*2)/Math.max(1,bounds.height))));panX=viewport.clientWidth/2-(bounds.x+bounds.width/2)*scale;panY=viewport.clientHeight/2-(bounds.y+bounds.height/2)*scale;transform()}
function fitSelection(){if(!selectedNodeId)return;traceSelection=false;applySelection(selectedNodeId);updateSelectionScope();const positions=[...nodeElements].filter(([,node])=>node.classList.contains('selected')||node.classList.contains('related')).map(([id])=>nodePositions.get(id));const left=Math.min(...positions.map(position=>position.x)),top=Math.min(...positions.map(position=>position.y)),right=Math.max(...positions.map(position=>position.x+position.width)),bottom=Math.max(...positions.map(position=>position.y+position.height));fitBounds({x:left,y:top,width:right-left,height:bottom-top})}
function fitGraph(){
if(graphView==='knowledge'){const branch=knowledgeState.branches.find(item=>item.id===knowledgeState.activeBranch);branchFitPositions(branch?branch.ids:[...nodePositions.keys()],!!branch);return}
const viewport=el('viewport'),legend=viewport.querySelector('.plane-legend');fitBounds({x:0,y:0,width:stageWidth,height:stageHeight},{horizontal:35,vertical:Math.max(35,(legend?.offsetHeight||0)+(legend?.offsetTop||0)+12)})}
function outputCognitions(explorationID){const ids=data.edges.filter(edge=>edge.kind==='output'&&edge.from===explorationID).map(edge=>edge.to);return ids.map(id=>byId.get(id)).filter(node=>node?.kind==='cognition')}
function closeExplorationPanel(){if(!openRelatedRow)return;openRelatedRow.querySelector('.exploration-related').hidden=true}
function closeExplorationRelated(){if(!openRelatedRow)return;openRelatedRow.classList.remove('active');openRelatedRow.querySelector('.exploration-toggle').setAttribute('aria-expanded','false');openRelatedRow.querySelector('.exploration-details').hidden=true;openRelatedRow.querySelector('.exploration-related').hidden=true;openRelatedRow=null}
function owningExploration(node){if(node.kind==='exploration')return node;const origin=byId.get(node.origin_ref);if(origin?.kind==='exploration')return origin;const output=data.edges.find(edge=>edge.kind==='output'&&edge.to===node.id&&byId.get(edge.from)?.kind==='exploration');return output?byId.get(output.from):null}
function openExplorationArticle(article,item,{syncGraph=false,scroll=false}={}){if(!article)return;closeExplorationRelated();const toggle=article.querySelector('.exploration-toggle'),details=article.querySelector('.exploration-details');renderExplorationDetails(item,details);article.classList.add('active');toggle.setAttribute('aria-expanded','true');details.hidden=false;openRelatedRow=article;if(syncGraph)focusNode(item.id);if(scroll)requestAnimationFrame(()=>article.scrollIntoView({behavior:'smooth',block:'center'}))}
function revealExplorationForGraphNode(node,{scroll=false}={}){const item=owningExploration(node);if(!item)return;if(explorationTaskFilter&&explorationTaskFilter!==item.task_id){explorationTaskFilter='';el('exploration-task-filter').value=''}if(explorationFilter&&explorationFilter!==item.status){explorationFilter='';document.querySelectorAll('.filter').forEach(button=>button.setAttribute('aria-pressed',String(button.dataset.status==='')))}renderExplorations();openExplorationArticle(explorationElements.get(item.id),item,{scroll})}
function openExplorationOutputs(item){const article=explorationElements.get(item.id);if(!article)return;if(openRelatedRow!==article)openExplorationArticle(article,item);const panel=article.querySelector('.exploration-related');renderExplorationRelated(item,panel);panel.hidden=false}
function renderExplorationRelated(item,panel){panel.replaceChildren();const head=document.createElement('div');head.className='related-head';const heading=document.createElement('h3');const outputs=outputCognitions(item.id);heading.textContent=t('outputCognitions')+' · '+outputs.length;const close=document.createElement('button');close.type='button';close.className='related-close';close.title=t('closeOutputs');close.setAttribute('aria-label',t('closeOutputs'));close.textContent='×';close.addEventListener('click',event=>{event.stopPropagation();closeExplorationPanel()});head.append(heading,close);const stack=document.createElement('div');stack.className='related-stack';for(const cognition of outputs){const output=document.createElement('details');output.className='related-item';const summary=document.createElement('summary');summary.dataset.expandLabel=t('showContent');summary.dataset.collapseLabel=t('hideContent');const kind=document.createElement('div');kind.className='related-kind';kind.textContent=cognitionLabel(cognition);const title=document.createElement('div');title.className='related-title';title.textContent=cognition.title||t('untitledNode');summary.append(kind,title);const content=renderMarkdown(cognition.id,cognition.content);const meta=document.createElement('dl');meta.className='related-meta';for(const [label,value] of [[t('createdAt'),formatDate(cognition.created_at)],[t('cognitionId'),cognition.id]]){const row=document.createElement('div');row.className='related-meta-row';const term=document.createElement('dt');term.textContent=label+':';const detail=document.createElement('dd');detail.textContent=text(value);row.append(term,detail);meta.append(row)}const locate=document.createElement('button');locate.type='button';locate.className='related-locate';locate.textContent=t('locateInGraph');locate.addEventListener('click',()=>{closeExplorationRelated();el('graph-module').scrollIntoView({behavior:'smooth',block:'start'});focusNode(cognition.id,{reveal:true,center:true})});output.append(summary,content,meta,locate);stack.append(output)}panel.append(head,stack)}
function initExplorationTaskFilter(){const select=el('exploration-task-filter');for(const task of [...data.tasks].sort((a,b)=>String(a.created_at).localeCompare(String(b.created_at)))){const option=document.createElement('option');option.value=task.id;option.textContent=truncate(task.purpose,20);option.title=task.purpose;select.append(option)}}
function explorationAttribute(parent,label,value,{wide=false,tokens=false}={}){const field=document.createElement('div');field.className='exploration-attribute'+(wide?' wide':'');const term=document.createElement('dt');term.textContent=label;const detail=document.createElement('dd');if(tokens){detail.className='exploration-source-list';const values=Array.isArray(value)?value.filter(Boolean):value?[value]:[];if(values.length){for(const entry of values){const token=document.createElement('span');token.className='exploration-source';token.textContent=entry;detail.append(token)}}else{detail.classList.add('empty');detail.textContent='-'}}else detail.textContent=text(value);field.append(term,detail);parent.append(field)}
function renderExplorationDetails(item,details){details.replaceChildren();const content=document.createElement('section');content.className='exploration-details-content';const contentTitle=document.createElement('h4');contentTitle.textContent=t('content');content.append(contentTitle,renderMarkdown(item.id,item.content));const reason=explorationReason(item);if(reason)content.append(reason);const properties=document.createElement('section');properties.className='exploration-details-fields';const propertiesTitle=document.createElement('h4');propertiesTitle.textContent=t('properties');const attributes=document.createElement('dl');attributes.className='exploration-attributes';const outputs=outputCognitions(item.id).map(cognition=>cognition.id);explorationAttribute(attributes,t('explorationId'),item.id,{wide:true});explorationAttribute(attributes,t('status'),statusNames[item.status]||item.status);explorationAttribute(attributes,t('createdAt'),formatDateSeconds(item.created_at));if(item.status!=='todo'){explorationAttribute(attributes,t('executorId'),item.executor_id);explorationAttribute(attributes,t('assignedAt'),formatDateSeconds(item.assigned_at))}if(item.status==='finished'){explorationAttribute(attributes,t('outcome'),outcomeName(item.outcome));explorationAttribute(attributes,t('finishedAt'),formatDateSeconds(item.finished_at))}if(item.status!=='todo'){explorationAttribute(attributes,t('sessionFile'),item.output_ref,{wide:true});if(item.retry_of)explorationAttribute(attributes,t('retryFrom'),item.retry_of,{wide:true})}explorationAttribute(attributes,t('taskId'),item.task_id,{wide:true});explorationAttribute(attributes,t('sourceCognitions'),item.source_cognitions,{wide:true,tokens:true});if(item.status!=='todo')explorationAttribute(attributes,t('outputCognitions'),outputs,{wide:true,tokens:true});properties.append(propertiesTitle,attributes);if(outputs.length){const button=detailAction(t('viewOutputs',{count:outputs.length}),()=>openExplorationOutputs(item));const actions=document.createElement('div');actions.className='exploration-output-actions';actions.append(button);content.append(actions)}details.append(content,properties)}
function renderExplorations(){closeExplorationRelated();const list=el('exploration-list');list.replaceChildren();explorationElements=new Map();const items=data.explorations.filter(item=>(!explorationTaskFilter||item.task_id===explorationTaskFilter)&&(!explorationFilter||item.status===explorationFilter)).sort((a,b)=>String(a.created_at).localeCompare(String(b.created_at))||a.id.localeCompare(b.id));el('exploration-count').textContent=t('explorationCount',{count:items.length});el('exploration-empty').hidden=items.length!==0;items.forEach((item,index)=>{const article=document.createElement('article');article.className='exploration-item '+item.status;if(item.status==='finished'&&item.outcome)article.dataset.outcome=item.outcome;article.dataset.id=item.id;const toggle=document.createElement('button');toggle.type='button';toggle.className='exploration-toggle';toggle.setAttribute('aria-expanded','false');const number=document.createElement('span');number.className='exploration-index';number.textContent=String(index+1).padStart(2,'0');const identity=document.createElement('div');identity.className='exploration-identity';const title=document.createElement('div');title.className='exploration-title';title.textContent=item.title;const id=document.createElement('span');id.className='exploration-sub';const outputs=outputCognitions(item.id);id.textContent=t('taskOutputs',{count:outputs.length});id.title=item.id;identity.append(title,id);const time=document.createElement('time');time.className='exploration-time';time.dateTime=item.finished_at||item.assigned_at||item.created_at;time.textContent=item.status==='finished'?t('completedAt',{time:formatDate(item.finished_at),duration:formatElapsed(item.assigned_at,item.finished_at)}):item.status==='running'?t('startedAt',{time:formatDate(item.assigned_at),duration:formatElapsed(item.assigned_at,data.generated_at)}):t('createdAtTime',{time:formatDate(item.created_at)});const details=document.createElement('div');details.className='exploration-details';details.hidden=true;const panel=document.createElement('aside');panel.className='exploration-related';panel.hidden=true;panel.setAttribute('aria-label',t('outputsAria',{title:item.title}));toggle.addEventListener('click',event=>{event.stopPropagation();const shouldOpen=openRelatedRow!==article;closeExplorationRelated();if(shouldOpen)openExplorationArticle(article,item,{syncGraph:true})});panel.addEventListener('click',event=>event.stopPropagation());details.addEventListener('click',event=>event.stopPropagation());const state=document.createElement('span');state.className='exploration-state';state.append(explorationBadge(item));toggle.append(number,identity,time,state);article.append(toggle,details,panel);list.append(article);explorationElements.set(item.id,article)})}

function initKnowledgeMotion() {
  const svg = el('edges'), shell = el('graph-shell'), viewport = el('viewport');
  const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)');
  let frame = 0, previousTime = 0, edgeLayer = null, layer = null, tracks = [];
  const intersectsViewport = () => {
    const bounds = shell.getBoundingClientRect();
    return bounds.bottom >= 0 && bounds.top <= window.innerHeight;
  };
  let inViewport = intersectsViewport();
  const highlightColor = color => 'rgb(' + color.match(/[a-f\d]{2}/gi).map(channel => {
    const value = parseInt(channel, 16);
    return Math.round(value + (255 - value) * .48);
  }).join(',') + ')';
  const makeTrack = (item, route, index) => {
    let length = 0;
    const points = route.samples.map((point, position) => {
      if (position) length += Math.hypot(point.x - route.samples[position - 1].x, point.y - route.samples[position - 1].y);
      return { x: point.x, y: point.y, distance: length };
    });
    return { item, points, length, index, distance: length * (index * .61803398875 % 1), particles: [],
      theme: knowledgeEdgeTheme(item.edge),
      bounds: { left: Math.min(...points.map(point => point.x)), right: Math.max(...points.map(point => point.x)), top: Math.min(...points.map(point => point.y)), bottom: Math.max(...points.map(point => point.y)) } };
  };
  const sample = (track, distance) => {
    const points = track.points;
    const position = Math.max(0, Math.min(track.length, distance));
    let low = 0, high = points.length - 1;
    while (low + 1 < high) {
      const middle = (low + high) >>> 1;
      if (points[middle].distance < position) low = middle; else high = middle;
    }
    const start = points[low], end = points[high];
    const fraction = (position - start.distance) / (end.distance - start.distance || 1);
    return { x: start.x + (end.x - start.x) * fraction, y: start.y + (end.y - start.y) * fraction };
  };
  function rebuild() {
    const next = svg.querySelector('.edge-layer');
    if (edgeLayer === next && (graphView === 'knowledge' ? layer?.isConnected : !layer)) return;
    edgeLayer = next; layer?.remove(); layer = null; tracks = []; previousTime = 0;
    if (graphView !== 'knowledge' || !next) return;
    const definitions = svg.querySelector('defs');
    for (const [theme, { color }] of Object.entries(knowledgeThemes)) {
      const id = 'knowledge-motion-glow-' + theme;
      if (definitions.querySelector('#' + id)) continue;
      const gradient = svgElement('radialGradient', { id });
      gradient.append(svgElement('stop', { offset: '0%', 'stop-color': color, 'stop-opacity': '.56' }),
        svgElement('stop', { offset: '38%', 'stop-color': color, 'stop-opacity': '.24' }),
        svgElement('stop', { offset: '100%', 'stop-color': color, 'stop-opacity': '0' }));
      definitions.append(gradient);
    }
    layer = svgElement('g', { class: 'knowledge-particles', 'aria-hidden': 'true', 'pointer-events': 'none' });
    next.after(layer);
    const byEdge = new Map(knowledgeState.routes.map(route => [route.edge, route]));
    tracks = edgeElements.map((item, index) => {
      const route = byEdge.get(item.edge);
      if (!route?.samples?.length) return null;
      const track = makeTrack(item, route, index);
      return track.length > 0 ? track : null;
    }).filter(Boolean);
  }
  const particle = track => {
    const group = svgElement('g', { class: 'knowledge-particle', visibility: 'hidden', 'data-edge': track.index });
    const color = knowledgeThemes[track.theme].color;
    const glow = svgElement('circle', { class: 'knowledge-particle-glow', fill: 'url(#knowledge-motion-glow-' + track.theme + ')' });
    const core = svgElement('circle', { class: 'knowledge-particle-core', fill: color });
    const highlight = svgElement('circle', { class: 'knowledge-particle-highlight', fill: highlightColor(color) });
    group.append(glow, core, highlight); layer.append(group);
    return { group, glow, core, highlight, sizeKey: null };
  };
  const visible = track => {
    const classes = track.item.path.classList;
    if (classes.contains('unrelated') || classes.contains('preview-unrelated') || classes.contains('branch-inactive')) return false;
    if (!classes.contains('backbone') && !knowledgeState.allRelations && !classes.contains('related') && !classes.contains('preview-related')) return false;
    const bounds = track.bounds;
    return bounds.right * scale + panX >= -16 && bounds.left * scale + panX <= viewport.clientWidth + 16
      && bounds.bottom * scale + panY >= -16 && bounds.top * scale + panY <= viewport.clientHeight + 16;
  };
  const paint = (track, bead, distance, emphasized) => {
    const point = sample(track, distance);
    if (point.x * scale + panX < -16 || point.x * scale + panX > viewport.clientWidth + 16
      || point.y * scale + panY < -16 || point.y * scale + panY > viewport.clientHeight + 16) {
      bead.group.setAttribute('visibility', 'hidden'); return;
    }
    const sizeKey = scale + ':' + emphasized;
    if (bead.sizeKey !== sizeKey) {
      const radius = emphasized ? 1.95 : 1.6;
      bead.core.setAttribute('r', String(radius / scale));
      bead.glow.setAttribute('r', String((emphasized ? 5.5 : 4.6) / scale));
      bead.highlight.setAttribute('r', String(radius * .46 / scale));
      bead.highlight.setAttribute('cx', String(-radius * .16 / scale));
      bead.highlight.setAttribute('cy', String(-radius * .16 / scale));
      bead.sizeKey = sizeKey;
    }
    bead.group.setAttribute('visibility', 'visible');
    bead.group.setAttribute('transform', 'translate(' + point.x.toFixed(3) + ' ' + point.y.toFixed(3) + ')');
    bead.group.setAttribute('opacity', String(emphasized ? .98 : track.item.path.classList.contains('backbone') ? .76 : .5));
  };
  function render(delta = 0) {
    if (!layer || reducedMotion?.matches) return false;
    let active = false;
    for (const track of tracks) {
      const show = visible(track);
      const count = Math.min(4, Math.max(1, Math.floor(track.length * scale / 160)));
      if (show) {
        active = true;
        while (track.particles.length < count) track.particles.push(particle(track));
      }
      track.distance = (track.distance + delta * 26 / scale) % track.length;
      const classes = track.item.path.classList;
      const emphasized = classes.contains('related') || classes.contains('preview-related');
      for (const [index, bead] of track.particles.entries()) {
        if (!show || index >= count) { bead.group.setAttribute('visibility', 'hidden'); continue; }
        paint(track, bead, (track.distance + index * track.length / count) % track.length, emphasized);
      }
    }
    return active;
  }
  const canRun = () => graphView === 'knowledge' && !document.hidden && inViewport && !reducedMotion?.matches;
  function refresh() {
    rebuild();
    const active = render();
    if (canRun() && active) {
      if (!frame) frame = requestAnimationFrame(tick);
    } else {
      if (frame) cancelAnimationFrame(frame);
      frame = 0; previousTime = 0;
    }
  }
  function tick(timestamp) {
    frame = 0;
    if (!canRun()) { previousTime = 0; return; }
    const delta = previousTime ? Math.min(.05, (timestamp - previousTime) / 1000) : 0;
    previousTime = timestamp;
    if (render(delta)) frame = requestAnimationFrame(tick);
    else previousTime = 0;
  }
  const geometryObserver = new MutationObserver(refresh);
  geometryObserver.observe(svg, { childList: true });
  const relationObserver = new MutationObserver(refresh);
  relationObserver.observe(svg, { attributes: true, subtree: true, attributeFilter: ['class'] });
  const modeObserver = new MutationObserver(refresh);
  modeObserver.observe(shell, { attributes: true, attributeFilter: ['data-graph-view', 'data-all-relations'] });
  const cameraObserver = new MutationObserver(refresh);
  cameraObserver.observe(el('stage'), { attributes: true, attributeFilter: ['style'] });
  if (typeof IntersectionObserver !== 'undefined') {
    const viewObserver = new IntersectionObserver(entries => { inViewport = entries[0].isIntersecting; refresh(); });
    viewObserver.observe(shell);
  } else {
    const updateVisibility = () => { inViewport = intersectsViewport(); refresh(); };
    window.addEventListener('scroll', updateVisibility, { passive: true });
    window.addEventListener('resize', updateVisibility);
  }
  document.addEventListener('visibilitychange', refresh);
  if (reducedMotion?.addEventListener) reducedMotion.addEventListener('change', refresh);
  else reducedMotion?.addListener?.(refresh);
  refresh();
}

function initKnowledgeGraph() {
  const viewport = el('viewport');
  viewport.addEventListener('pointermove', branchPointerMove);
  viewport.addEventListener('pointerleave', branchClearHover);
  viewport.addEventListener('pointercancel', branchClearHover);
  const dock = document.createElement('div'); dock.id = 'branch-dock'; dock.className = 'branch-dock';
  dock.addEventListener('pointerdown', event => event.stopPropagation());
  dock.addEventListener('click', event => event.stopPropagation());
  const tools = document.createElement('div'); tools.className = 'branch-dock-tools';
  tools.append(viewport.querySelector('.graph-controls'));
  const navigation = document.createElement('div'); navigation.id = 'branch-navigation'; navigation.className = 'branch-navigation';
  navigation.addEventListener('pointerdown', event => event.stopPropagation());
  const footer = document.createElement('div'); footer.className = 'branch-footer';
  const status = document.createElement('span'); status.id = 'branch-status';
  const hint = document.createElement('span'); hint.textContent = t('graphNavigationHint');
  footer.append(status, hint); dock.append(tools, navigation, footer); viewport.append(dock);
  el('graph-shell').dataset.allRelations = 'false';
  branchRenderNavigation();
  branchUpdateDock();
  if (typeof ResizeObserver !== 'undefined') {
    const dockObserver = new ResizeObserver(() => { branchUpdateDock(); scheduleKnowledgeLabels(); });
    dockObserver.observe(dock);
  }
  requestAnimationFrame(fitGraph);
}

el('runtime').classList.toggle('active',data.runtime_occupied);el('runtime').querySelector('span').textContent=data.runtime_occupied?t('runtimeActive'):t('runtimeIdle');el('generated').textContent=t('generatedAt',{time:formatDate(data.generated_at)})+' · '+data.display_timezone;
initExplorationTaskFilter();renderTasks();renderMetrics();renderGraph();renderExplorations();
function setSearchOpen(open){el('graph-search').classList.toggle('open',open);el('search-toggle').setAttribute('aria-expanded',String(open));if(open)requestAnimationFrame(()=>el('search').focus())}
el('search').addEventListener('input',applySearch);el('search').addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();navigateSearch(event.shiftKey?-1:1)}if(event.key==='Escape'){event.preventDefault();setSearchOpen(false);el('search-toggle').focus()}});el('search-previous').addEventListener('click',()=>navigateSearch(-1));el('search-next').addEventListener('click',()=>navigateSearch(1));el('search-toggle').addEventListener('click',event=>{event.stopPropagation();setSearchOpen(!el('graph-search').classList.contains('open'))});el('search-close').addEventListener('click',event=>{event.stopPropagation();setSearchOpen(false)});el('graph-search').addEventListener('pointerdown',event=>event.stopPropagation());el('exploration-task-filter').addEventListener('change',event=>{explorationTaskFilter=event.target.value;renderExplorations()});document.querySelectorAll('.filter').forEach(button=>button.addEventListener('click',()=>{explorationFilter=button.dataset.status;document.querySelectorAll('.filter').forEach(item=>item.setAttribute('aria-pressed',String(item===button)));renderExplorations()}));document.addEventListener('click',event=>{if(openRelatedRow&&!openRelatedRow.contains(event.target))closeExplorationPanel()});document.addEventListener('keydown',event=>{if(event.key==='Escape'){closeExplorationPanel();setSearchOpen(false)}});
el('zoom-value').addEventListener('change',commitZoom);el('zoom-value').addEventListener('focus',event=>event.target.select());el('zoom-value').addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();commitZoom();event.target.blur()}});el('zoom-in').addEventListener('click',()=>setZoom(scale*1.2,el('viewport').clientWidth/2,el('viewport').clientHeight/2));el('zoom-out').addEventListener('click',()=>setZoom(scale/1.2,el('viewport').clientWidth/2,el('viewport').clientHeight/2));el('fit').addEventListener('click',fitGraph);el('graph-view-toggle').addEventListener('click',toggleGraphView);el('fullscreen').addEventListener('click',async()=>{if(document.fullscreenElement===el('graph-shell'))await document.exitFullscreen();else await el('graph-shell').requestFullscreen()});document.addEventListener('fullscreenchange',()=>{const active=document.fullscreenElement===el('graph-shell'),label=t(active?'exitFullscreen':'fullscreen');el('fullscreen').title=label;el('fullscreen').setAttribute('aria-label',label);requestAnimationFrame(fitGraph)});el('detail-close').addEventListener('click',clearSelection);
el('viewport').addEventListener('click',event=>{if(!event.target.closest('.node,.graph-controls,.graph-search'))clearSelection()});el('viewport').addEventListener('wheel',event=>{event.preventDefault();const box=el('viewport').getBoundingClientRect();setZoom(scale*(event.deltaY<0?1.12:1/1.12),event.clientX-box.left,event.clientY-box.top)},{passive:false});el('viewport').addEventListener('pointerdown',event=>{if(event.target.closest('.node,.graph-controls,.graph-search'))return;drag={x:event.clientX,y:event.clientY,px:panX,py:panY,id:event.pointerId};el('viewport').classList.add('dragging');el('viewport').setPointerCapture(event.pointerId)});el('viewport').addEventListener('pointermove',event=>{if(!drag)return;panX=drag.px+event.clientX-drag.x;panY=drag.py+event.clientY-drag.y;transform()});const endDrag=()=>{drag=null;el('viewport').classList.remove('dragging')};el('viewport').addEventListener('pointerup',endDrag);el('viewport').addEventListener('pointercancel',endDrag);window.addEventListener('resize',()=>requestAnimationFrame(fitGraph));

initKnowledgeGraph();
initKnowledgeMotion();
</script>
</body>
</html>`
	var rendered strings.Builder
	tmpl, err := template.New("visual").Parse(page)
	if err != nil {
		return nil, err
	}
	htmlLang := "en"
	if language == "zh" {
		htmlLang = "zh-CN"
	}
	view := struct {
		Data     template.JS
		HTMLLang string
	}{Data: template.JS(data), HTMLLang: htmlLang}
	if err := tmpl.Execute(&rendered, view); err != nil {
		return nil, err
	}
	return []byte(rendered.String()), nil
}
