package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVisualCognitionMetaIsFormattedTextInNodeDetails(t *testing.T) {
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
const source=html.slice(html.indexOf('function cognitionMetadata('),html.indexOf('function detailAction('));
class Element {
 constructor(tag){this.tag=tag;this.children=[];this.classList={add(){}}}
 append(...children){this.children.push(...children)}
 replaceChildren(...children){this.children=children}
 set innerHTML(value){throw new Error('metadata must not be interpreted as HTML')}
}
const document={createElement:tag=>new Element(tag)},elements=new Map();
const el=id=>{if(!elements.has(id))elements.set(id,new Element('div'));return elements.get(id)};
const t=value=>value,cognitionLabel=()=>'',explorationBadge=()=>new Element('span');
const renderMarkdown=()=>new Element('div'),explorationReason=()=>null,createDetailActions=()=>new Element('div');
const identifierField=()=>{},relationField=()=>{},neighborsFor=()=>[];
eval(source);
const flatten=node=>[node,...node.children.flatMap(flatten)];
const meta={type:'theorem',scope:'geometry',note:'<img src=x onerror=alert(1)>',unicode:'几何'};
for(const origin of ['task','user','exploration']){
 showDetail({id:'cog_test',kind:'cognition',origin,content:'Evidence',meta});
 const nodes=flatten(el('detail-body')),blocks=nodes.filter(item=>item.tag==='code');
 assert.equal(blocks.length,1);
 assert.equal(blocks[0].className,'language-json');
 assert.equal(blocks[0].textContent,JSON.stringify(meta,null,2));
 assert.equal(nodes.some(item=>item.tag==='img'),false);
 assert.equal(nodes.filter(item=>item.tag==='pre').length,1);
}
for(const item of [{kind:'cognition'}, {kind:'cognition',meta:{}}, {kind:'exploration',meta}]){
 showDetail({id:'empty',content:'Content',...item});
 assert.equal(flatten(el('detail-body')).filter(node=>node.tag==='pre').length,0);
}
`
	command := exec.Command(node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual metadata rendering failed: %v\n%s", err, output)
	}
}
