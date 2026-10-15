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

func TestSimulationJSONResponseDiagnosticsBrowser(t *testing.T) {
	_ = readTestFile(t, "frontend/src/js/simulation-progress.js")
	if testing.Short() {
		t.Skip("simulation response browser regression")
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
	mux.HandleFunc("/response-test", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, simulationJSONResponseHTML)
	})
	mux.HandleFunc("/response-done", func(w http.ResponseWriter, r *http.Request) {
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
	command := exec.Command(chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--user-data-dir="+profile, server.URL+"/response-test")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stop := epath161OwnedChromeCleanup(t, command, profile)
	t.Cleanup(stop)
	select {
	case result := <-done:
		stop()
		for _, item := range result.Evidence {
			t.Log(item)
		}
		if len(result.Failures) != 0 {
			t.Fatal(strings.Join(result.Failures, "\n"))
		}
	case <-ctx.Done():
		stop()
		t.Fatal("simulation JSON response browser did not report within its deadline")
	}
}

// Exercise the production reader with bounded browser streams. The empty
// decoder reproduces native WebView2's oversized-string outcome without a
// routine test allocating the original 595 MB response.
const simulationJSONResponseHTML = `<!doctype html><html><head><meta charset="utf-8"></head><body><script type="module">
const failures=[],evidence=[],check=(condition,message)=>{if(!condition)failures.push(message);};
try{
 const {readSimulationJSONResponse:read}=await import('/src/js/simulation-progress.js');
 const {setLanguage,t}=await import('/src/js/i18n.js');setLanguage('en');
 const encoder=new TextEncoder();
 const response=(text,headers={},split=false)=>{
  const bytes=encoder.encode(text),body=new ReadableStream({start(controller){
   if(split)for(let n=0;n<bytes.length;n++)controller.enqueue(bytes.slice(n,n+1));else if(bytes.length)controller.enqueue(bytes);
   controller.close();
  }});
  return new Response(body,{headers:{'Content-Type':'application/json',...headers}});
 };
 const text='{"name":"보일러🔥","values":[0,-0,1.25,-3],"items":null}',size=encoder.encode(text).length,progress=[];
 const utf8=response(text,{'Content-Length':String(size)},true),value=await read(utf8,item=>progress.push(item));
 check(value.name==='보일러🔥'&&value.items===null&&Object.is(value.values[1],-0)&&value.values[2]===1.25&&value.values[3]===-3,'byte-split UTF-8, null, negative zero or precision changed');
 check(progress.at(-1).phase==='decode_results'&&progress.at(-1).workCompleted===size&&progress[0].workTotal===size&&!utf8.body.locked,'actual byte progress or stream lock cleanup changed');
 const compressedProgress=[],compressed=response(text,{'Content-Encoding':'gzip','Content-Length':'1'});
 check((await read(compressed,item=>compressedProgress.push(item))).name===value.name&&compressedProgress[0].workTotal===0,'compressed Content-Length was treated as decoded byte count');
 check((await read(response(text))).name===value.name,'missing Content-Length or absent progress callback rejected valid JSON');
 check(await read(response('null',{'Content-Length':'4'}))===null,'valid null response became an empty-body error');
 check((await read(response(text,{'Content-Encoding':'IDENTITY','Content-Length':String(size)}))).name===value.name,'identity encoding was not normalized');
 const fallback={headers:new Headers({'Content-Length':String(size)}),body:null,arrayBuffer:async()=>encoder.encode(text).buffer};
 check((await read(fallback)).name===value.name,'non-streaming response fallback changed values');
 const reject=async(item,code,label)=>{
  let error;try{await read(item);}catch(cause){error=cause;}
  check(error?.code==='simulation_result_transfer_'+code,label+' did not receive specific transfer error');
  check(error?.message.includes('result')||error?.message.includes('Result'),label+' was not identified as result loading');
  check(!error?.message.includes('Unexpected end of JSON')&&!item.body?.locked,label+' leaked a parser message or reader lock');
  return error;
 };
 await reject(response('',{'Content-Length':'0'}),'empty','empty response');
 const short=await reject(response(text,{'Content-Length':String(size+8)}),'incomplete','truncated response');
 check(short.message.includes(size+' B')&&short.message.includes((size+8)+' B'),'truncated response omitted received/expected counts');
 const malformed=await reject(response('{"results":'),'invalidJSON','malformed JSON');
 check(malformed.cause instanceof SyntaxError,'malformed JSON lost its diagnostic cause');
 const streamCause=new TypeError('connection interrupted'),firstChunk=encoder.encode('{"results":'),fallbackCause=new TypeError('body unavailable');
 let pulled=false;
 const interrupted=new Response(new ReadableStream({pull(controller){
  if(!pulled){pulled=true;controller.enqueue(firstChunk);}else controller.error(streamCause);
 }}),{headers:{'Content-Length':String(size)}});
 const streamError=await reject(interrupted,'interrupted','rejected response stream');
 check(streamError.cause===streamCause&&streamError.message.includes(firstChunk.byteLength+' B'),'interrupted stream lost its cause or received byte count');
 const fallbackError=await reject({headers:new Headers(),body:null,arrayBuffer:async()=>{throw fallbackCause;}},'interrupted','rejected non-streaming response');
 check(fallbackError.cause===fallbackCause&&fallbackError.message.includes('0 B'),'interrupted fallback lost its cause or received byte count');
 const OriginalDecoder=globalThis.TextDecoder;
 try{
  globalThis.TextDecoder=class {decode(){return '';}};
  const emptyDecode=await reject(response(text,{'Content-Length':String(size)}),'decode','complete bytes decoded empty');
  check(emptyDecode.message.includes('browser')&&emptyDecode.message.includes(size+' B'),'oversized decode omitted browser text-limit/byte evidence');
 }finally{globalThis.TextDecoder=OriginalDecoder;}
 check((await read(response(text))).name===value.name,'decoder was not restored after native failure simulation');
 setLanguage('ko');
 let localized;try{await read(response(''));}catch(error){localized=error;}
 check(localized?.message===t('simulation.resultTransfer.empty')&&localized?.message.includes('저장된 실행 결과'),'Korean transfer diagnostic was not localized');
 setLanguage('en');
 evidence.push('Production reader: byte-split UTF-8; exact byte progress; compressed/unknown lengths; null; no-progress and non-stream fallback; empty/incomplete/interrupted/malformed responses; native empty decode; EN/KO diagnostics; reader/decoder cleanup');
}catch(error){failures.push(error.stack||String(error));}
await fetch('/response-done',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({failures,evidence})});
</script></body></html>`
