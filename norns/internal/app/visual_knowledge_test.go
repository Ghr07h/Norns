package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVisualKnowledgeGraphGeometry(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required to exercise generated Visual JavaScript")
	}
	html, err := visualHTML([]byte(`{}`), "en")
	if err != nil {
		t.Fatal(err)
	}
	const script = `
const assert=require('node:assert/strict'),vm=require('node:vm'),html=require('node:fs').readFileSync(0,'utf8');
const script=html.slice(html.indexOf('<script>')+8,html.lastIndexOf('</script>'));
new vm.Script(script);
const algorithms=script.slice(script.indexOf('const knowledgeState'),script.indexOf('function updateGraphViewControl'));
const checks=new Function('assert', 'let stageWidth=0,stageHeight=0;const graphArrowGap=8,byId=new Map();function graphNodeCompare(first,second){return String(first.created_at).localeCompare(String(second.created_at))||first.id.localeCompare(second.id)}function graphPathNumber(value){return Math.round(value*10)/10}\n'+algorithms+'\n'+String.raw` + "`" + `
const makeNode=(id,kind='cognition',origin='exploration')=>({id,kind,origin,created_at:id,title:id});
const center=position=>({x:position.x+position.width/2,y:position.y+position.height/2});
const distance=(first,second)=>Math.hypot(first.x-second.x,first.y-second.y);
assert.equal(knowledgeGraphLayout([],[]).positions.size,0);
assert.equal(stageWidth,880);
const root={...makeNode('root','cognition','task'),created_at:'0000'};
const nodes=[root,makeNode('explore','exploration'),makeNode('result'),makeNode('other','exploration'),makeNode('island'),makeNode('cycle','exploration')];
for(const node of nodes)byId.set(node.id,node);
const edges=[{from:'root',to:'explore',kind:'source'},{from:'explore',to:'result',kind:'output'},{from:'root',to:'other',kind:'source'},{from:'other',to:'result',kind:'output'},{from:'result',to:'cycle',kind:'source'},{from:'cycle',to:'result',kind:'output'},{from:'missing',to:'root',kind:'source'}];
const layout=knowledgeGraphLayout(nodes,edges),rootPosition=layout.positions.get(root.id);
assert.equal(layout.positions.size,nodes.length);
assert.equal(knowledgeState.root,root.id);
assert.equal(knowledgeState.parent.get('result'),'other');
assert.equal(knowledgeState.parent.get('cycle'),'result');
assert.equal(knowledgeState.parent.has('island'),false,'isolated nodes must not gain fabricated parents');
assert.equal(layout.positions.get('island').root,true);
assert.equal(rootPosition.root,true);
assert.ok(rootPosition.width>layout.positions.get('result').width);
for(const [id,position] of layout.positions){assert.ok(Number.isFinite(position.x)&&Number.isFinite(position.y));if(id!==root.id)assert.ok(distance(center(rootPosition),center(position))>(rootPosition.width+position.width)/2+20)}
const entries=[...layout.positions.values()];for(let first=0;first<entries.length;first++)for(let second=first+1;second<entries.length;second++)assert.ok(distance(center(entries[first]),center(entries[second]))>(entries[first].width+entries[second].width)/2+20,'nodes overlap');
assert.deepEqual(knowledgeGraphLayout([...nodes].reverse(),[...edges].reverse()).positions,layout.positions);
const circle={id:'root',x:0,y:0,width:184,height:184};
assert.deepEqual(knowledgeNodeBoundary(circle,{x:1000,y:92}),{x:179,y:92});
assert.deepEqual(knowledgeNodeBoundary(circle,{x:92,y:-1000}),{x:92,y:5});
const hexagon={id:'explore',x:0,y:0,width:144,height:144};
assert.equal(knowledgeHexagon(hexagon,5).length,6);
assert.ok(Math.abs(knowledgeNodeBoundary(hexagon,{x:72,y:-1000}).y-(72-67*Math.sin(Math.PI/3)))<.001);
const routes=knowledgeGraphRoutes([...edges,{from:'root',to:'explore',kind:'retry'},{from:'root',to:'root',kind:'retry'}],layout.positions);
assert.equal(routes.length,edges.length+1);
for(const route of routes)assert.ok(!/NaN|Infinity/.test(route.path),route.path);
assert.equal(new Set(routes.map(route=>route.path)).size,routes.length);
for(const route of routes)for(const [id,position]of layout.positions){if(id===route.edge.from||id===route.edge.to)continue;for(let index=1;index<route.samples.length;index++)assert.ok(knowledgeSegmentDistance(center(position),route.samples[index-1],route.samples[index])>position.width/2-5,'route intersects a node')}
assert.ok(routes.find(route=>route.edge.from===route.edge.to).samples.length>3,'self-loop must have visible geometry');
const obstacles=new Map([['root',{x:0,y:0,width:132,height:132}],['result',{x:600,y:0,width:132,height:132}],['other',{x:300,y:0,width:144,height:144}]]);
const avoided=knowledgeGraphRoutes([{from:'root',to:'result',kind:'source'}],obstacles)[0];
const obstacle=center(obstacles.get('other'));
assert.ok(avoided.samples.every(point=>distance(point,obstacle)>86),'route crosses an unrelated node');
const emptyGrid=knowledgeRoutingGrid(new Map()),smoothEdge={from:'smooth-from',to:'smooth-to',kind:'source'};
const wideTurn=knowledgeRoundedPath([{x:100,y:500},{x:500,y:500},{x:500,y:100}],emptyGrid,smoothEdge);
assert.ok(wideTurn.samples.every(point=>distance(point,{x:500,y:500})>45),'long bends must not retain tiny fixed-radius elbows');
assert.deepEqual(wideTurn.samples[0],{x:100,y:500});assert.deepEqual(wideTurn.samples.at(-1),{x:500,y:100});
const shortBend=[{x:100,y:200},{x:300,y:200},{x:318,y:182},{x:600,y:400}];
assert.equal(knowledgeRoundedPath(shortBend,emptyGrid,smoothEdge).curves.length,0,'unobstructed zigzags should collapse to a direct segment');
assert.equal(shortBend.length,4,'smoothing must not mutate the routed points');
const anchored=knowledgeRoundedPath(shortBend.map((point,index)=>({...point,anchor:index===1})),emptyGrid,smoothEdge);
assert.ok(anchored.curves.some(curve=>curve.end.x===300&&curve.end.y===200),'parallel-edge lane anchors must survive smoothing');
for(let index=1;index<anchored.curves.length;index++){
 const previous=anchored.curves[index-1],next=anchored.curves[index];
 assert.ok(Math.hypot(previous.end.x-previous.second.x-next.first.x+next.start.x,previous.end.y-previous.second.y-next.first.y+next.start.y)<.3,'cubic joins must be tangent-continuous');
}
const closeObstacle={x:280,y:280,width:40,height:40},closeGrid=knowledgeRoutingGrid(new Map([['smooth-obstacle',closeObstacle]]));
const tightTurn=knowledgeRoundedPath([{x:100,y:500},{x:500,y:500},{x:500,y:100}],closeGrid,smoothEdge);
assert.match(tightTurn.path,/ C /,'obstacle avoidance must retain a smooth turn');
assert.notEqual(tightTurn.path,wideTurn.path,'rounding must adapt to nearby obstacles');
for(let index=1;index<tightTurn.samples.length;index++)assert.ok(knowledgeSegmentDistance(center(closeObstacle),tightTurn.samples[index-1],tightTurn.samples[index])>=closeObstacle.width/2+12,'smoothed curve must preserve obstacle clearance');
const alternate={...makeNode('later-task','cognition','task'),created_at:'9999'};
assert.equal([...knowledgeGraphLayout([...nodes,alternate],edges).positions].find(([,position])=>position.root)[0],root.id);
assert.equal(knowledgeGraphLayout([makeNode('fallback','exploration')],[]).positions.get('fallback').root,true);
stageWidth=1280;stageHeight=1000;
const wall=new Map([['left',{x:40,y:400,width:132,height:132}],['right',{x:1020,y:400,width:132,height:132}],...Array.from({length:4},(_,index)=>['wall-'+index,{x:500,y:140+index*170,width:144,height:144}])]);
for(const id of wall.keys())byId.set(id,makeNode(id));
const detours=knowledgeGraphRoutes([{from:'left',to:'right',kind:'source'},{from:'left',to:'right',kind:'retry'},{from:'right',to:'left',kind:'output'}],wall);
assert.equal(new Set(detours.map(route=>route.path)).size,3);
for(const route of detours)for(const [id,position]of wall){if(id===route.edge.from||id===route.edge.to)continue;for(let index=1;index<route.samples.length;index++)assert.ok(knowledgeSegmentDistance(center(position),route.samples[index-1],route.samples[index])>position.width/2-5,'detour intersects obstacle wall')}
for(const count of [1,12,80]){const fan=[root,...Array.from({length:count},(_,index)=>makeNode('fan-'+index,'exploration'))],links=fan.slice(1).map(node=>({from:root.id,to:node.id,kind:'source'}));const result=knowledgeGraphLayout(fan,links);assert.equal(result.positions.get(root.id).root,true);const values=[...result.positions.values()];for(let first=0;first<values.length;first++)for(let second=first+1;second<values.length;second++)assert.ok(distance(center(values[first]),center(values[second]))>(values[first].width+values[second].width)/2+20,'fan overlaps')}
const chronological=[root,...Array.from({length:45},(_,index)=>({...makeNode('time-'+(45-index),index%2?'cognition':'exploration'),created_at:new Date(Date.UTC(2026,10,5,0,index)).toISOString()}))];
const timeLinks=chronological.slice(1).map((node,index)=>({from:index%3?root.id:chronological[Math.max(0,index-1)].id,to:node.id,kind:index%2?'output':'source'}));
const timeLayout=knowledgeGraphLayout(chronological,timeLinks);
assert.deepEqual(knowledgeGraphLayout([...chronological].reverse(),[...timeLinks].reverse()).positions,timeLayout.positions,'branch layout must remain deterministic');
const timePositions=[...timeLayout.positions.values()];
for(let first=0;first<timePositions.length;first++)for(let second=first+1;second<timePositions.length;second++)assert.ok(distance(center(timePositions[first]),center(timePositions[second]))>(timePositions[first].width+timePositions[second].width)/2+20,'branch nodes overlap');
const early={...makeNode('early','cognition','task'),created_at:'2026-11-05T10:00:00+08:00'},late={...makeNode('late','cognition','task'),created_at:'2026-11-05T03:00:00Z'};
assert.equal(knowledgeGraphLayout([late,early],[]).positions.get('early').root,true,'root selection must respect timezone offsets');
const retryNodes=[root,makeNode('first','exploration'),makeNode('retry','exploration'),makeNode('output')];
const retryEdges=[{from:root.id,to:'first',kind:'source'},{from:root.id,to:'retry',kind:'source'},{from:'first',to:'retry',kind:'retry'},{from:'retry',to:'output',kind:'output'}];
const skeleton=branchBuildSkeleton(retryNodes,retryEdges);
assert.equal(skeleton.parent.get('retry'),'root');assert.equal(skeleton.parent.get('output'),'retry');
const disconnected=Array.from({length:20},(_,index)=>makeNode('independent-'+index,'cognition','task'));
const forest=knowledgeGraphLayout(disconnected,[]);
assert.equal(knowledgeState.roots.length,disconnected.length);
assert.equal(knowledgeState.parent.size,0,'multiple inputs must remain unrelated');
for(const position of forest.positions.values())assert.equal(position.root,true);
const forestPositions=[...forest.positions.values()];
for(let first=0;first<forestPositions.length;first++)for(let second=first+1;second<forestPositions.length;second++)assert.ok(distance(center(forestPositions[first]),center(forestPositions[second]))>(forestPositions[first].width+forestPositions[second].width)/2+20,'independent inputs overlap');
console.log('Knowledge graph geometry checks passed');
` + "`" + `);
checks(assert);
`
	command := exec.Command(node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual knowledge graph geometry failed: %v\n%s", err, output)
	}
}

func TestVisualKnowledgeGraphReadableLabelsAndPreview(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required to exercise generated Visual JavaScript")
	}
	html, err := visualHTML([]byte(`{}`), "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`transform:scale(var(--knowledge-label-scale,1))`,
		`let graphView='plane'`,
		`function initKnowledgeGraph()`,
		`viewport.addEventListener('pointermove', branchPointerMove)`,
		`function initKnowledgeMotion()`,
		`class: 'knowledge-particle-core'`,
		`readBackbone:'回到主干阅读'`, `readBackbone:'Read backbone'`,
		`locateInitialCognition:'定位初始认知'`, `locateInitialCognition:'Locate initial cognition'`,
		`class: 'knowledge-particle-glow'`,
		`.knowledge-particles{pointer-events:none}`,
		`knowledgeHelp:'悬停高亮关联 · 点击查看详情'`,
		`class="legend-exploration legend-todo"`, `stroke="url(#cognition-bronze)"`,
		`dataset.knowledgeDensity=scale<.32?'overview':'detail'`,
		`frame.setAttribute('height', String(Math.ceil(size.height)))`,
		`expanded ? '6' : compact ? '1' : '2'`,
		`knowledgeState.labelPlacements`,
	} {
		if !strings.Contains(string(html), required) {
			t.Errorf("Visual omitted knowledge graph readability behavior %q", required)
		}
	}
	for _, obsolete := range []string{
		`window.nornsPrototype`, `branch-prototype`, `LAYOUT STUDY`, `knowledge-flow`,
		`window.motionPreview`, `motion-preview-header`,
		`.node:not(.selected):not(.previewed):not(:hover):not(:focus) .knowledge-label{display:none}`,
	} {
		if strings.Contains(string(html), obsolete) {
			t.Errorf("Visual retained obsolete prototype or label behavior %q", obsolete)
		}
	}
	const script = `
const assert=require('node:assert/strict'),html=require('node:fs').readFileSync(0,'utf8');
const extract=(start,end)=>html.slice(html.indexOf('function '+start+'('),html.indexOf('function '+end+'('));
const checks=new Function('assert',
extract('appendKnowledgeLabel','knowledgeLabelPlacement')+
extract('knowledgeLabelPlacement','layoutKnowledgeLabels')+
extract('branchLabelCandidates','branchUpdateBackground')+
extract('previewKnowledgeNode','addNode')+
extract('transform','setZoom')+String.raw` + "`" + `
let graphView='knowledge',selectedNodeId=null,scale=.5,panX=10,panY=20,scheduled=0;
const values=new Map(),shell={dataset:{},style:{setProperty:(name,value)=>values.set(name,value)}},stage={style:{}},zoom={};
const el=id=>id==='graph-shell'?shell:id==='stage'?stage:zoom;
const scheduleKnowledgeLabels=()=>scheduled++;
const branchClearHover=()=>{},branchUpdateBackground=()=>{};
const element=()=>({children:[],attributes:{},textContent:'',classList:{values:new Set(),toggle(name,active){if(active)this.values.add(name);else this.values.delete(name)}},append(...children){this.children.push(...children)},setAttribute(name,value){this.attributes[name]=value}});
const svgElement=(tag,attributes)=>Object.assign(element(),{tag,attributes});
const document={createElementNS:()=>element()},t=key=>key;
const nodeElements=new Map(['root','neighbor','island'].map(id=>[id,element()]));
const edgeElements=[{edge:{from:'root',to:'neighbor'},path:element()},{edge:{from:'island',to:'island'},path:element()}];
const has=(id,name)=>nodeElements.get(id).classList.values.has(name);
previewKnowledgeNode('root');
assert.ok(has('root','previewed'));assert.ok(has('neighbor','preview-related'));assert.ok(has('island','preview-unrelated'));
assert.ok(edgeElements[0].path.classList.values.has('preview-related'));
assert.ok(edgeElements[1].path.classList.values.has('preview-unrelated'));
selectedNodeId='neighbor';previewKnowledgeNode('root');
for(const item of nodeElements.values())assert.equal(item.classList.values.size,0,'hover must not override selection');
selectedNodeId=null;graphView='plane';previewKnowledgeNode('root');
for(const item of nodeElements.values())assert.equal(item.classList.values.size,0,'planar view must not gain knowledge preview classes');
graphView='knowledge';previewKnowledgeNode(null);
for(const item of edgeElements)assert.equal(item.path.classList.values.size,0,'preview classes must clear');
const title='<img src=x onerror="alert(1)">',item=element();
appendKnowledgeLabel(item,{title},{width:132,height:132});
const label=item.children[0],frame=label.children[1],copy=frame.children[0].children[0],text=copy.children[0];
assert.equal(label.attributes.transform,'translate(66 66)');
assert.equal(text.textContent,title);assert.equal(text.innerHTML,undefined,'titles must not be interpreted as HTML');
for(const [nextScale,inverse,density]of [[.5,'2','detail'],[2,'.5','detail'],[.2,'5','overview']]){
 scale=nextScale;transform();assert.equal(Number(values.get('--knowledge-label-scale')),Number(inverse));assert.equal(shell.dataset.knowledgeDensity,density);
 assert.equal(zoom.value,String(Math.round(scale*100)));
}

const center={x:220,y:240,radius:26},size={width:160,height:34},obstacles=[center,{x:350,y:240,radius:26}],bounds={left:12,right:480,top:70,bottom:500},occupied=[];
const candidates=branchLabelCandidates(center,size,bounds);
assert.equal(candidates.length,48,'label search must include near and distant alternatives');
assert.ok(candidates.some(candidate=>candidate.inside));
for(const candidate of candidates)assert.equal(candidate.inside,candidate.x>=bounds.left&&candidate.x+candidate.width<=bounds.right&&candidate.y>=bounds.top&&candidate.y+candidate.height<=bounds.bottom);
assert.equal(branchRectsOverlap({x:10,y:10,width:100,height:30},{x:110,y:10,width:100,height:30}),true,'title placement must reserve padding');
assert.equal(branchRectsOverlap({x:10,y:10,width:100,height:30},{x:116,y:10,width:100,height:30}),false);
for(let index=0;index<3;index++){
 const placement=knowledgeLabelPlacement(center,size,obstacles,occupied,bounds,{x:1,y:0});
 assert.ok(Number.isFinite(placement.score));
 assert.ok(placement.x>=bounds.left&&placement.y>=bounds.top&&placement.x+placement.width<=bounds.right&&placement.y+placement.height<=bounds.bottom,'label must stay in the canvas');
 for(const obstacle of obstacles){const nearestX=Math.max(placement.x,Math.min(obstacle.x,placement.x+placement.width)),nearestY=Math.max(placement.y,Math.min(obstacle.y,placement.y+placement.height));assert.ok(Math.hypot(nearestX-obstacle.x,nearestY-obstacle.y)>=obstacle.radius+5,'label overlaps a node')}
 for(const other of occupied)assert.ok(placement.x+placement.width<=other.x||other.x+other.width<=placement.x||placement.y+placement.height<=other.y||other.y+other.height<=placement.y,'labels overlap');
 occupied.push(placement);
}
assert.ok(scheduled>0);
for(const center of [{x:20,y:90,radius:30},{x:320,y:90,radius:30},{x:20,y:490,radius:30},{x:320,y:490,radius:30}]){
 const bounds={left:12,right:326,top:70,bottom:500},placement=knowledgeLabelPlacement(center,{width:188,height:51},[center],[],bounds,{x:-1,y:1});
 assert.ok(placement.x>=bounds.left&&placement.y>=bounds.top&&placement.x+placement.width<=bounds.right&&placement.y+placement.height<=bounds.bottom,'labels near mobile viewport edges must stay within bounds');
}
` + "`" + `);
checks(assert);
`
	command := exec.Command(node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual knowledge graph readability checks failed: %v\n%s", err, output)
	}
}

func TestVisualPlanarLegendAndFixedBackgrounds(t *testing.T) {
	html, err := visualHTML([]byte(`{}`), "zh")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`class="graph-legend plane-legend"`, `class="graph-legend knowledge-legend"`,
		`class="legend-task"`, `class="legend-result"`, `class="legend-regular"`,
		`class="legend-todo"`, `class="legend-running"`, `class="legend-finished"`,
		`legendTaskInit:'任务初始化认知'`, `legendTaskRelated:'任务直接关联认知'`, `legendCommon:'常规认知'`,
		`legendTodoExploration:'待分配探索'`, `legendRunningExploration:'进行中探索'`, `legendFinishedExploration:'已完成探索'`,
		`.graph-shell[data-graph-view="knowledge"] .plane-legend{display:none}`,
		`.graph-shell[data-graph-view="knowledge"] .knowledge-legend{display:flex}`,
		`background-image:linear-gradient(var(--canvas-grid) 1px,transparent 1px),linear-gradient(90deg,var(--canvas-grid) 1px,transparent 1px)`,
		`fill="url(#cognition-bronze)"`, `fill="url(#cognition-gold)"`, `fill="url(#cognition-silver)"`,
		`.legend-row-exploration i::after{content:"";position:absolute;inset:2px;background:#fff;clip-path:inherit}`,
		`filter:brightness(.88) saturate(1.3)`,
		`--canvas-bg:#f6f8fa`,
		`background-size:28px 28px;background-position:0 0`,
		`background-image:radial-gradient(#8ea5b1 1.1px,transparent 1.1px);background-size:32px 32px`,
	} {
		if !strings.Contains(string(html), required) {
			t.Errorf("Visual omitted graph legend or shared grid behavior %q", required)
		}
	}
	if strings.Contains(string(html), `knowledgeTimeline`) || strings.Contains(string(html), `legend-timeline`) {
		t.Error("Knowledge graph legend must not include a creation-time explanation")
	}
}

func TestVisualLegendLabelsStayScopedToLegends(t *testing.T) {
	html, err := visualHTML([]byte(`{}`), "zh")
	if err != nil {
		t.Fatal(err)
	}
	source := string(html)
	for _, view := range []string{"plane", "knowledge"} {
		start := strings.Index(source, `<div class="graph-legend `+view+`-legend"`)
		if start < 0 {
			t.Fatalf("Visual omitted %s legend", view)
		}
		remainder := source[start:]
		end := strings.Index(remainder, `</div></div>`)
		if end < 0 {
			t.Fatalf("Visual omitted %s legend rows", view)
		}
		legend := remainder[:end+len(`</div></div>`)]
		if strings.Count(legend, `class="legend-row`) != 2 {
			t.Errorf("%s legend must contain two type rows", view)
		}
		if strings.Count(legend, `class="legend-group-name"`) != 2 {
			t.Errorf("%s legend must name each row's type", view)
		}
		for _, key := range []string{"legendTaskInit", "legendTaskRelated", "legendCommon", "legendTodoExploration", "legendRunningExploration", "legendFinishedExploration"} {
			if strings.Count(legend, `data-i18n="`+key+`"`) != 1 {
				t.Errorf("%s legend must use dedicated label %q", view, key)
			}
		}
		for _, key := range []string{"todo", "running", "finished"} {
			if strings.Contains(legend, `data-i18n="`+key+`"`) {
				t.Errorf("%s legend must not reuse general status label %q", view, key)
			}
		}
	}
	for _, unchanged := range []string{
		`todo:'待分配',running:'进行中',finished:'已完成'`,
		`taskInitCognition:'任务初始认知'`, `taskRelatedCognition:'任务相关认知'`, `commonCognition:'常规认知'`,
		`.plane-legend,.knowledge-legend{flex-direction:column;align-items:flex-start;gap:8px}`,
	} {
		if !strings.Contains(source, unchanged) {
			t.Errorf("Visual changed non-legend wording or omitted row layout %q", unchanged)
		}
	}
	markup := source[:strings.Index(source, "<script>")]
	for _, key := range []string{"legendTodoExploration", "legendRunningExploration", "legendFinishedExploration"} {
		if strings.Count(markup, `data-i18n="`+key+`"`) != 2 {
			t.Errorf("Dedicated exploration label %q must only appear once in each legend", key)
		}
	}
}
