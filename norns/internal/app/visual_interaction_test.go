package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVisualFullChainFitsAllNodes(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required to exercise generated Visual JavaScript")
	}
	html, err := visualHTML([]byte(`{}`), "en")
	if err != nil {
		t.Fatal(err)
	}
	const script = `
const assert=require('node:assert/strict'),html=require('node:fs').readFileSync(0,'utf8');
const extract=(start,end)=>html.slice(html.indexOf('function '+start+'('),html.indexOf('function '+end+'('));
const check=new Function('assert',String.raw` + "`" + `
let traceSelection=false,scale=2.5,panX=500,panY=-400,selectedNodeId='selected',graphView='plane';
const stageWidth=2000,stageHeight=1600,viewport={clientWidth:900,clientHeight:680,querySelector:()=>null},selections=[];
const document={createElement:()=>({children:[],append(...children){this.children.push(...children)},setAttribute(){}})};
const t=key=>key,el=()=>viewport,detailAction=(label,activate)=>({label,activate,setAttribute(){}}),owningExploration=()=>null;
function applySelection(id){selections.push(id)}
function updateSelectionScope(){}
function fitSelection(){}
function transform(){}
` + "`" + `+extract('createDetailActions','traverse')+extract('fitBounds','fitSelection')+extract('fitGraph','outputCognitions')+String.raw` + "`" + `
const actions=createDetailActions({id:selectedNodeId}),button=actions.children[0].children[1];
assert.equal(button.label,'fullChain');
for(const expectedTrace of [true,false]){
 scale=2.5;panX=500;panY=-400;button.activate();
 assert.equal(traceSelection,expectedTrace);
 assert.equal(scale,.38125);assert.equal(panX,68.75);assert.equal(panY,35);
 assert.equal(selectedNodeId,'selected');
}
assert.deepEqual(selections,['selected','selected']);
viewport.querySelector=()=>({offsetHeight:64,offsetTop:14});fitGraph();
assert.ok(panY>=90&&stageHeight*scale+panY<=viewport.clientHeight-90,'fit all must leave space for the planar legend');
` + "`" + `);
check(assert);
`
	command := exec.Command(node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual full-chain action failed: %v\n%s", err, output)
	}
}

func TestVisualDetailsScrollWithinGraph(t *testing.T) {
	html, err := visualHTML([]byte(`{}`), "en")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"grid-template-rows:minmax(0,1fr);height:680px;min-height:0",
		".graph-main{position:relative;min-width:0;min-height:0}",
		".detail{position:relative;min-width:0;min-height:0;max-width:100%",
		"overflow:auto;overscroll-behavior:contain;scrollbar-gutter:stable",
		".graph-body{grid-template-columns:1fr;height:620px}",
		".graph-body{height:580px}",
		".graph-shell:fullscreen .viewport{height:100%;min-height:0}",
		"detail.classList.add('visible');detail.scrollTop=0",
	} {
		if !strings.Contains(string(html), required) {
			t.Errorf("Visual omitted bounded-detail behavior %q", required)
		}
	}
}
