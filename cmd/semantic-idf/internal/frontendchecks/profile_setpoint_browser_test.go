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

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Exercise the analyzer's nullable temperature series through the same controls
// and charts used for the other Profile dimensions.
func TestProfileSetpointDimensionsFromAnalyzedIDFBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser Profile setpoint verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	doc, err := idf.Parse(profileSetpointBrowserIDF)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(idf.AnalyzeProfile(doc))
	if err != nil {
		t.Fatal(err)
	}
	markup, _, ok := strings.Cut(profileLayoutSelectionHarnessHTML, `<script type="module">`)
	if !ok {
		t.Fatal("Profile layout fixture has no module boundary")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/profile-setpoint.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/profile-setpoint", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, markup+profileSetpointBrowserAssertions)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check", "--window-size=1440,1200",
		"--virtual-time-budget=25000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/profile-setpoint",
	).CombinedOutput()
	if err != nil || ctx.Err() != nil {
		t.Fatalf("Profile setpoint browser: %v, %v\n%s", err, ctx.Err(), output)
	}
	if !strings.Contains(string(output), `data-profile-setpoint-status="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("Profile setpoint browser: %s", document[start:start+end])
			}
		}
		t.Fatalf("Profile setpoint browser did not pass:\n%s", output)
	}
}

const profileSetpointBrowserIDF = `
Version,24.1;
Zone,Zone A,0,0,0,0,1,1,3,300,100;
Zone,Zone B,0,0,0,0,1,1,3,300,100;
ZoneList,Both zones,Zone A,Zone B;
ScheduleTypeLimits,Control type,0,4,Discrete;
ScheduleTypeLimits,Temperature,,,Continuous,Temperature;
Schedule:Compact,Control mode,Control type,Through:12/31,For:AllDays,
  Until:06:00,0,Until:12:00,1,Until:18:00,2,Until:24:00,4;
Schedule:Compact,Heating temperature,Temperature,Through:12/31,For:AllDays,
  Until:12:00,0,Until:24:00,20;
Schedule:Compact,Cooling temperature,Temperature,Through:12/31,For:AllDays,
  Until:18:00,-5,Until:24:00,26;
Schedule:Constant,Always on,,1;
ThermostatSetpoint:SingleHeating,Heating only,Heating temperature;
ThermostatSetpoint:SingleCooling,Cooling only,Cooling temperature;
ThermostatSetpoint:DualSetpoint,Dual,Heating temperature,Cooling temperature;
ZoneControl:Thermostat,Shared thermostat,Both zones,Control mode,
  ThermostatSetpoint:SingleHeating,Heating only,
  ThermostatSetpoint:SingleCooling,Cooling only,
  ThermostatSetpoint:DualSetpoint,Dual;
Lights,Shared lights,Both zones,Always on,Watts/Area,,5;
`

const profileSetpointBrowserAssertions = `<script type="module">
const assert = (condition, message, detail = {}) => { if (!condition) throw new Error(message + ": " + JSON.stringify(detail)); };
const until = async (predicate, message) => { for (let i=0;i<200;i++) { if (predicate()) return; await new Promise(resolve=>setTimeout(resolve,10)); } throw new Error(message); };
try {
  localStorage.clear();
  const saves = [];
  window.go = { main: { App: { SaveSettings: async settings => { saves.push(structuredClone(settings)); return {settings}; } } } };
  const [{state}, profileViews, i18n, settingsClient] = await Promise.all([
    import("/src/js/state.js"), import("/src/js/views/profile-views.js"),
    import("/src/js/i18n.js"), import("/src/js/settings-client.js"),
  ]);
  const profile = await (await fetch("/profile-setpoint.json")).json();
  const before = JSON.stringify(profile);
  const dimensions = ["cooling_setpoint", "heating_setpoint"];
  const zoneSeries = dimension => profile.graphDataset.series.find(series=>series.scopeType==="zone" && series.zoneName==="Zone A" && series.dimension===dimension);
  const heat = zoneSeries("heating_setpoint"), cool = zoneSeries("cooling_setpoint");
  assert(heat && cool, "analyzed thermostat supplies both native dimensions");
  assert(heat.annualValueProfile[0]===null && heat.annualValueProfile[6]===0 && heat.annualValueProfile[12]===null && heat.annualValueProfile[18]===20,
    "inactive heating is absent while real zero remains known", heat.annualValueProfile.slice(0,24));
  assert(cool.annualValueProfile[0]===null && cool.annualValueProfile[6]===null && cool.annualValueProfile[12]===-5 && cool.annualValueProfile[18]===26,
    "cooling preserves negative degrees and control-type intervals", cool.annualValueProfile.slice(0,24));
  for (const series of [heat,cool]) for (const field of ["dayValueProfile","weekValueProfile","monthValueProfile","annualValueProfile","durationValueProfile"]) {
    assert(Array.isArray(series[field]) && series[field].some(value=>value!==null), "all five native views provide temperatures", {dimension:series.dimension,field});
  }
  const legacy = settingsClient.mergeSettings({profile:{enabledDimensions:["lighting"]}});
  assert(legacy.profile.enabledDimensions.join() === "lighting" && dimensions.every(dimension=>legacy.profile.displayMetrics[dimension]==="temperature" && legacy.profile.groupingMetrics[dimension]==="temperature"), "old settings gain metric choices without changing checked dimensions");
  state.report = {profile};
  state.analysisKey = state.reportAnalysisKey = "profile-setpoint-native";
  state.activeResultTab = "profile";
  state.profileSettings = structuredClone(legacy.profile);
  state.profileViewCache = new Map();
  state.activeProfileView = "profile";
  state.profileSelectedDimensions = [];
  state.profileSelectedGroupIds = [];
  state.profileSelectedZoneNames = [];
  i18n.setLanguage("en");
  profileViews.initializeProfileControls();
  profileViews.renderProfile(profile);
  const checkbox = dimension => document.querySelector('#profileSettings input[data-profile-dimension="'+dimension+'"]');
  const row = checkbox("lighting").closest(".profile-toggle-row");
  for (const dimension of dimensions) {
    const control = checkbox(dimension);
    assert(control?.type === "checkbox" && control.closest(".profile-toggle-row")===row && !control.checked,
      "temperature is a peer checkbox preserving existing selection", {dimension});
  }
  const pane = document.getElementById("profilePane"), controls = document.getElementById("profileSettings");
  for (const width of [1100,360]) {
    pane.style.width=width+"px";
    const bounds=controls.getBoundingClientRect();
    assert(controls.scrollWidth<=controls.clientWidth+1 && [...controls.querySelectorAll(".profile-check")].every(label=>{
      const box=label.getBoundingClientRect(); return box.left>=bounds.left-1 && box.right<=bounds.right+1;
    }), "all eight peer checkboxes wrap within wide and narrow containers", {width,scroll:controls.scrollWidth,client:controls.clientWidth});
  }
  pane.style.width="1100px";
  for (const dimension of dimensions) checkbox(dimension).click();
  await until(()=>saves.length>=2 && dimensions.every(dimension=>saves.at(-1).profile.enabledDimensions.includes(dimension)), "both checked temperatures persist through Settings API");
  state.profileSelectedDimensions = dimensions;
  profileViews.renderProfile(profile);
  assert(profile.groups.some(group=>group.zoneNames.length===2), "shared thermostat definition groups both zones");
  const panel = dimension => document.querySelector('#profileGraph section[data-profile-dimension="'+dimension+'"]');
  const picture = dimension => {
    const target=panel(dimension), canvas=target?.querySelector("canvas");
    return canvas ? canvas.toDataURL() : target?.querySelector(".profile-overlay-paths path")?.getAttribute("d");
  };
  const ticks = dimension => [...panel(dimension).querySelectorAll(".profile-y-axis-ticks .profile-axis-tick")].map(tick=>Number(tick.textContent.replaceAll(",","")));
  const snapshots = {};
  for (const view of ["day","week","month","year","duration"]) {
    state.profileSettings.scaleMode = "auto";
    document.querySelector('[data-profile-time-view="'+view+'"]').click();
    for (const dimension of dimensions) {
      const target=panel(dimension);
      assert(target && /°C/.test(target.textContent), "temperature axis retains degrees in every view", {view,dimension,text:target?.textContent});
      snapshots[view+dimension] = picture(dimension);
      assert(snapshots[view+dimension], "selected temperature is drawn", {view,dimension});
      if (view!=="year") assert(!/NaN|Infinity/.test(snapshots[view+dimension]), "temperature path has finite coordinates", {view,dimension});
    }
    for (const mode of ["shared","design_peak","multiplier_0_1","percentile"]) {
      state.profileSettings.scaleMode=mode;
      profileViews.renderProfile(profile);
      assert(dimensions.every(dimension=>picture(dimension)===snapshots[view+dimension]), "temperature values ignore multiplier scaling", {view,mode});
    }
  }
  state.profileSettings.timeView="day";
  state.profileSettings.scaleMode="auto";
  profileViews.renderProfile(profile);
  assert(Math.min(...ticks("cooling_setpoint"))<=-5 && Math.max(...ticks("cooling_setpoint"))>=26 && Math.max(...ticks("cooling_setpoint"))<40,
    "cooling axis shows negative actual temperature and group mean, not summed temperatures", ticks("cooling_setpoint"));
  assert(Math.max(...ticks("heating_setpoint"))>=20 && Math.max(...ticks("heating_setpoint"))<35, "heating axis shows actual group temperatures", ticks("heating_setpoint"));
  for (const dimension of dimensions) {
    const path=picture(dimension), firstX=Number(/^M([\d.]+)/.exec(path.trim())?.[1]);
    assert(firstX>0 && (path.match(/M/g)||[]).length>=3, "inactive periods leave real gaps between daily temperature traces", {dimension,firstX,path});
  }
  state.activeProfileView="zone";
  state.activeProfileZoneName="Zone A";
  state.profileSelectedZoneNames=["Zone A"];
  profileViews.renderProfile(profile);
  assert(dimensions.every(dimension=>picture(dimension)===snapshots["day"+dimension]), "group temperature equals each identical zone, not their sum");
  state.profileSettings.timeView="year";
  profileViews.renderProfile(profile);
  const pixel = (dimension,hour) => [...panel(dimension).querySelector("canvas").getContext("2d").getImageData(0,hour,1,1).data];
  assert(pixel("heating_setpoint",0)[3]===0 && pixel("heating_setpoint",6)[3]===255 && pixel("heating_setpoint",18)[3]===255,
    "heatmap distinguishes missing data from 0°C", {absent:pixel("heating_setpoint",0),zero:pixel("heating_setpoint",6)});
  assert(pixel("cooling_setpoint",12)[3]===255 && pixel("cooling_setpoint",12).join()!==pixel("cooling_setpoint",18).join(), "negative temperature paints with a distinct valid color");
  for (const [language,cooling,heating] of [["en","Cooling setpoint","Heating setpoint"],["ko","냉방 설정온도","난방 설정온도"],["ja","Cooling setpoint","Heating setpoint"]]) {
    i18n.setLanguage(language);
    profileViews.renderProfile(profile);
    assert(checkbox("cooling_setpoint").parentElement.textContent.trim()===cooling && checkbox("heating_setpoint").parentElement.textContent.trim()===heating,
      "temperature labels follow Korean/English and per-key English fallback", {language});
  }
  state.activeProfileView="profile";
  profileViews.renderProfile(profile);
  document.getElementById("profileApplyButton").click();
  assert(!document.getElementById("profileApplyDialog").classList.contains("hidden") && document.querySelector('[data-profile-apply-dimension="lighting"]') && dimensions.every(dimension=>!document.querySelector('[data-profile-apply-dimension="'+dimension+'"]')), "Apply does not offer thermostat edits as load-object copying");
  assert(JSON.stringify(profile)===before, "charting, selection and Apply preserve native report values");
  document.body.dataset.profileSetpointStatus="passed";
  document.getElementById("result").textContent=JSON.stringify({nativeSetpointCheckboxes:true,settingsPersist:true,fiveRawTemperatureViews:true,meanNotSum:true,nullVersusZero:true,negativeDegrees:true,localized:true,applyBoundary:true,immutable:true});
} catch(error) {
  document.body.dataset.profileSetpointStatus="failed";
  document.getElementById("result").textContent=error.stack || String(error);
}
</script></body></html>`
