package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVisualKnowledgeThemesFollowDestination(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required to exercise generated Visual JavaScript")
	}
	html, err := visualHTML([]byte(`{}`), "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`.node.cognition .knowledge-ring{stroke-width:4;filter:brightness(.88) saturate(1.3)}`,
		`.node.cognition.knowledge-root .knowledge-ring{stroke-width:5}`,
		`ring.setAttribute('style','stroke:url(#'+knowledgeThemes[knowledgeNodeTheme(node)].metal+')')`,
		`.edge.retry{stroke-dasharray:`,
		`class="legend-exploration legend-running"`,
		`class="legend-exploration legend-finished"`,
	} {
		if !strings.Contains(string(html), required) {
			t.Errorf("Visual omitted destination theme behavior %q", required)
		}
	}
	for _, obsolete := range []string{`'knowledge-arrow-'+edge.kind`, `class="legend-source"`, `class="legend-output"`, `class="legend-extends"`, `class="legend-retry"`} {
		if strings.Contains(string(html), obsolete) {
			t.Errorf("Visual retained relation-kind coloring %q", obsolete)
		}
	}
	const script = `
const assert=require('node:assert/strict'),html=require('node:fs').readFileSync(0,'utf8');
const extract=(start,end)=>html.slice(html.indexOf(start),html.indexOf(end));
const source=extract('function cognitionVariant(','function cognitionLabel(')+
 extract('function appendMetalGradient(','const graphIconPaths=')+
 extract('function knowledgeGraphDefinitions(','function appendKnowledgeLabel(')+
 extract('function drawEdges(','function updateSearchStatus(');
const check=new Function('assert',source+String.raw` + "`" + `
class Element {
 constructor(tag,attributes={}){this.tag=tag;this.attributes={...attributes};this.children=[];this.classList={add(){}}}
 setAttribute(name,value){this.attributes[name]=String(value)}
 append(...children){this.children.push(...children)}
 replaceChildren(...children){this.children=children}
}
const svgElement=(tag,attributes)=>new Element(tag,attributes),svg=svgElement('svg'),el=()=>svg;
const stageWidth=1000,stageHeight=800,knowledgeState={parentEdges:new Map()},byId=new Map();
let graphView='knowledge',edgeElements=[];
const knowledgeGraphGuides=()=>svgElement('g');
const routes=edges=>edges.map(edge=>({edge,path:'M 0 0 L 100 100',plan:{kind:'direct'}}));
const knowledgeGraphRoutes=routes,graphRoutes=routes;
const cases=[
 ['task-cognition',{kind:'cognition',origin:'task',task_purpose_relevant:true},'#955d30','cognition-bronze'],
 ['result-cognition',{kind:'cognition',origin:'exploration',task_purpose_relevant:true},'#9a7414','cognition-gold'],
 ['regular',{kind:'cognition',origin:'exploration'},'#7b858c','cognition-silver'],
 ['todo',{kind:'exploration',status:'todo'},'#d4882f'],
 ['running',{kind:'exploration',status:'running'},'#477dbd'],
 ['finished',{kind:'exploration',status:'finished'},'#17684e']
];
const edges=[];
for(const [theme,node,color,metal] of cases){
 byId.set(theme,node);
 assert.equal(knowledgeNodeTheme(node),theme);assert.equal(knowledgeThemes[theme].color,color);
 assert.equal(knowledgeThemes[theme].metal,metal);
 for(const kind of ['source','output','extends','retry'])edges.push({from:'root',to:theme,kind});
}
assert.equal(knowledgeEdgeTheme({to:'missing'}),'regular');
assert.equal(knowledgeNodeTheme({kind:'exploration',status:'unknown'}),'regular');
drawEdges(edges,new Map());
const definitions=svg.children[0];
for(const [theme,,color,metal] of cases){
 const marker=definitions.children.find(child=>child.attributes.id==='knowledge-arrow-'+theme);
 assert.ok(marker);assert.equal(marker.children[0].attributes.fill,color);
 if(metal){
  const gradient=definitions.children.find(child=>child.attributes.id===metal);
  assert.equal(gradient.tag,'linearGradient');assert.ok(gradient.children.length>=6);
  assert.ok(new Set(gradient.children.map(stop=>stop.attributes['stop-color'])).size>=5);
 }
}
assert.equal(edgeElements.length,24);
for(const {edge,path} of edgeElements){
 const color=cases.find(([theme])=>theme===edge.to)[2];
 assert.equal(path.attributes.style,'--edge-color:'+color);
 assert.equal(path.attributes['marker-end'],'url(#knowledge-arrow-'+edge.to+')');
 assert.ok(path.attributes.class.includes(edge.kind),'relation kinds must remain available');
 assert.ok(!path.attributes.style.includes('url('),'cognition edge strokes must be flat metal colors');
}
graphView='plane';drawEdges(edges,new Map());
for(const {path} of edgeElements){assert.equal(path.attributes.style,undefined);assert.equal(path.attributes['marker-end'],'url(#arrow-relation)')}
assert.ok(!svg.children[0].children.some(child=>child.attributes.id.startsWith('knowledge-arrow-')));
console.log('Six destination themes, four relation kinds, metallic gradients and unchanged planar edges passed');
` + "`" + `);
check(assert);
`
	command := exec.Command(node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual destination theme checks failed: %v\n%s", err, output)
	}
}
