package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSemanticStickyNavigationScrollBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("Semantic scrolling geometry browser regression")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	type browserResult struct {
		Failures []string `json:"failures"`
		Evidence []string `json:"evidence"`
	}
	done := make(chan browserResult, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/semantic-scroll", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, semanticScrollBrowserHTML)
	})
	mux.HandleFunc("/semantic-scroll/done", func(w http.ResponseWriter, r *http.Request) {
		var result browserResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case done <- result:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	profile := t.TempDir()
	// Native layout/ResizeObserver frames must finish before accepting geometry;
	// a virtual-time dump can terminate while requestAnimationFrame is pending.
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--window-size=1200,900", "--user-data-dir="+profile, server.URL+"/semantic-scroll")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stop := epath161OwnedChromeCleanup(t, command, profile)
	t.Cleanup(stop)
	select {
	case result := <-done:
		stop()
		for _, line := range result.Evidence {
			t.Log(line)
		}
		if len(result.Failures) != 0 {
			t.Fatalf("Semantic scroll regression: %s", strings.Join(result.Failures, "\n"))
		}
	case <-ctx.Done():
		stop()
		t.Fatal("native-time Semantic scroll browser did not report before its deadline")
	}
}

const semanticScrollBrowserHTML = `<!doctype html><html data-theme="dark"><head>
<link rel="stylesheet" href="/src/styles.css">
<style>body{display:block;overflow:hidden;padding:30px}#fixture{width:940px}#semanticEditor{height:600px;flex:none}</style>
</head><body><main id="fixture"><div id="semanticEditor" class="semantic-editor"></div></main>
<script type="module">
const failures=[],evidence=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const frame=()=>new Promise(resolve=>requestAnimationFrame(resolve));
const settle=async()=>{await frame();await frame();await frame();};
try{
 const {state}=await import('/src/js/state.js'),{renderInputViews}=await import('/src/js/views/input-views.js');
 const sections=['compatibility','project','simulation','site','building','schedules','constructions','materials','zones','spaces','surfaces','equipment','outputs','miscellaneous'];
 const lines=[{text:'semantic_energyplus_model:',indent:0,role:'syntax'}];
 sections.forEach((section,index)=>{
  lines.push({text:'  '+section+':',key:section,indent:1,role:'syntax'});
  lines.push({text:'    branch_'+index+':',key:'branch_'+index,indent:2,role:'syntax'});
  for(let row=0;row<30;row++)lines.push({text:'      value_'+row+': '+row,key:'value_'+row,value:row,indent:3,role:'field'});
 });
 state.activeInputView='semantic';state.documentText='Version,25.1;';state.reportAnalyzedText=state.documentText;
 state.semanticProjection={lines,sourceNameConflicts:[]};
 renderInputViews();await settle();
 const editor=document.getElementById('semanticEditor'),fixture=document.getElementById('fixture');
 const navigation=()=>editor.querySelector('.semantic-navigation-header');
 const path=()=>editor.querySelector('.semantic-sticky-path');
 const index=()=>editor.querySelector('.semantic-section-index');
 const find=text=>[...editor.querySelectorAll('.semantic-line')].find(line=>line.dataset.semanticText.trim()===text);
 if(!navigation())throw Error('Semantic navigation has no combined header');
 const rows=()=>new Set([...index().querySelectorAll('button')].map(button=>Math.round(button.getBoundingClientRect().top))).size;
 const geometry=label=>{
  const header=navigation(),h=header.getBoundingClientRect(),e=editor.getBoundingClientRect(),i=index().getBoundingClientRect(),p=path().getBoundingClientRect();
  check(Math.abs(h.top-(e.top+editor.clientTop))<1,label+': header is not flush with the scroll viewport');
  check(p.top>=i.bottom-0.5,label+': breadcrumb overlaps the wrapped section buttons');
  check(p.bottom<=h.bottom+0.5,label+': breadcrumb escapes the fixed header');
  for(const y of [e.top+editor.clientTop+1,(i.bottom+p.top)/2,h.bottom-1]){
   const hit=document.elementFromPoint(e.left+e.width/2,y);
   check(Boolean(hit&&header.contains(hit)),label+': YAML is exposed through the header at y='+y);
  }
  const background=getComputedStyle(header).backgroundColor;
  check(background!=='transparent'&&!/rgba\([^)]*,\s*0(?:\.\d+)?\)$/.test(background),label+': header background is translucent: '+background);
  check(Math.abs(window.scrollY)<1&&Math.abs(document.documentElement.scrollTop)<1,label+': scrolling moved the application document');
  return h.height;
 };
 const position=async(section,row)=>{
  const start=find('branch_'+section+':');
  const target=start.parentElement.querySelector('[data-semantic-line="'+(Number(start.dataset.semanticLine)+row+1)+'"]');
  editor.scrollTop+=target.getBoundingClientRect().top-editor.getBoundingClientRect().top-navigation().getBoundingClientRect().height-3;
  await settle();
  check(path().textContent.includes(sections[section])&&path().textContent.includes('branch_'+section),'breadcrumb does not identify the first visible branch '+section+': '+path().textContent);
 };
 await position(7,12);
 const wideHeight=geometry('wide/down'),wideRows=rows();
 check(wideRows>=2,'fixture failed to wrap the section index at wide width');
 await position(2,15);geometry('wide/up');
 fixture.style.width='410px';await settle();
 const narrowHeight=geometry('narrow/resize'),narrowRows=rows();
 check(narrowRows>wideRows&&narrowHeight>wideHeight,'resizing did not reflow and expand the section header');
 await position(9,14);geometry('narrow/down');
 await position(3,8);geometry('narrow/up');
 const jump=async(section,label)=>{
  editor.querySelector('[data-semantic-section-id="'+section+'"]').click();await settle();
  const title=find(section+':').getBoundingClientRect(),header=navigation().getBoundingClientRect();
  check(title.top>=header.bottom-1&&title.top<=header.bottom+12,label+': section title is hidden by or separated from the complete header: '+JSON.stringify({title:title.top,header:header.bottom}));
  geometry(label);
 };
 await jump('outputs','narrow/section-jump');
 fixture.style.width='940px';await settle();geometry('wide/resize-back');
 await jump('schedules','wide/section-jump');
 renderInputViews();await settle();await position(5,10);geometry('rerender/up');
 evidence.push('Wrapped index rows '+wideRows+' -> '+narrowRows+'; header heights '+wideHeight+' -> '+narrowHeight+'px; native scroll up/down, resize, section jumps, breadcrumb and rerender passed');
}catch(error){failures.push(error.stack||String(error));}
await fetch('/semantic-scroll/done',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({failures,evidence})});
</script></body></html>`
