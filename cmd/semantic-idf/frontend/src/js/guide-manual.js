import { renderAppInfo } from "./app-info.js";
import { applyCachedAppSettings, loadAndApplyAppSettings } from "./settings-client.js";
import { getLanguage, getManualLanguage, translatePage } from "./i18n.js";
import { auxiliaryReturnDepth, setAuxiliaryReturnDepth } from "./auxiliary-navigation.js";

const labels = {
  en: {
    title: "Technical Reference Manual", search: "Search all chapters", clear: "Clear",
    skip: "Skip to chapter", chapter: "Technical reference chapter", pagination: "Previous and next chapters",
    shortcut: "Press / to search, Escape to clear.", results: "Search results", onThisPage: "On this page",
    loading: "Loading chapter…", searching: "Searching all chapters…", noResults: "No matching sections.",
    chapters: "Manual chapters", sections: "Chapter sections", previous: "Previous", next: "Next",
    unavailable: "This chapter could not be loaded. Try another chapter or reopen the Guide.",
    searchIncomplete: "Some chapters are unavailable; showing results from the chapters that loaded.",
    catalogLoading: "Loading metric catalog…", catalogUnavailable: "The metric catalog is unavailable.",
    catalogFallback: "Bundled metric reference (live catalog unavailable).", unit: "Unit", source: "Source", method: "Method",
    assumptions: "Assumptions", missingData: "Missing data", catalogTitle: "Metric catalog", uncategorized: "Other",
    count: (n) => `${n} matching section${n === 1 ? "" : "s"}`,
  },
  ko: {
    title: "기술 참고 매뉴얼", search: "전체 장 검색", clear: "지우기",
    skip: "본문으로 이동", chapter: "기술 참고 매뉴얼 본문", pagination: "이전 장과 다음 장",
    shortcut: "/ 키로 검색, Escape 키로 검색을 지웁니다.", results: "검색 결과", onThisPage: "이 장의 목차",
    loading: "본문을 불러오는 중…", searching: "전체 장을 검색하는 중…", noResults: "일치하는 절이 없습니다.",
    chapters: "매뉴얼 목차", sections: "이 장의 절", previous: "이전", next: "다음",
    unavailable: "본문을 불러올 수 없습니다. 다른 장을 선택하거나 매뉴얼을 다시 열어 주세요.",
    searchIncomplete: "일부 장을 불러올 수 없어, 사용 가능한 장의 검색 결과를 표시합니다.",
    catalogLoading: "지표 카탈로그를 불러오는 중…", catalogUnavailable: "지표 카탈로그를 불러올 수 없습니다.",
    catalogFallback: "내장 지표 참고 자료 (실시간 카탈로그 사용 불가).", unit: "단위", source: "원본", method: "계산 방법",
    assumptions: "가정", missingData: "누락 데이터", catalogTitle: "지표 카탈로그", uncategorized: "기타",
    count: (n) => `${n}개 절 일치`,
  },
};

export function escapeManualHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[character]);
}

const localizedMetricFields = ["name", "category", "source", "method", "assumptions", "missingData"];

function localizedMetricGuide(guide, entry) {
  const localized = { ...guide };
  for (const field of localizedMetricFields) {
    // The registry stays authoritative. A changed live definition must not
    // acquire an old translation, and IDs/units never enter this overlay.
    if (typeof entry?.translation?.[field] === "string" && entry.translation[field].trim() && guide[field] === entry.original?.[field]) localized[field] = entry.translation[field];
  }
  return localized;
}

function plainText(value) {
  return String(value).replace(/\[([^\]]+)\]\([^)]*\)/g, "$1").replace(/__([^_]+)__/g, "$1").replace(/(^|\s)_([^_]+)_(?=\s|$)/g, "$1$2").replace(/[`*~]/g, "").replace(/\s+/g, " ").trim();
}

export function manualSlug(value) {
  return plainText(value).normalize("NFKC").toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, "").replace(/\s+/g, "-").replace(/^-+|-+$/g, "") || "section";
}

function routeHash(chapter, section = "") {
  return `#${encodeURIComponent(chapter)}${section ? `/${encodeURIComponent(section)}` : ""}`;
}

function validHistoryDepth(value) {
  return Number.isInteger(value) && value >= 0 && value < window.history.length;
}

function safeLink(destination, chapter, chapterIds) {
  const value = destination.trim();
  if (/[\u0000-\u0020\u007f\\]/.test(value)) return null;
  if (value.startsWith("#")) {
    const section = value.slice(1);
    return section ? { href: routeHash(chapter, section), internal: true } : null;
  }
  const manual = /^(?:\.\/)?([a-z0-9-]+)\.(?:en|ko)\.md(?:#([^?]*))?$/.exec(value);
  if (manual && chapterIds.has(manual[1])) return { href: routeHash(manual[1], manual[2] || ""), internal: true };
  if (/^https?:\/\//i.test(value)) {
    try { const url = new URL(value); if (url.protocol === "https:" || url.protocol === "http:") return { href: url.href, external: true }; } catch { return null; }
  }
  if (/^mailto:[^<>"']+$/i.test(value)) return { href: value, external: true };
  if (/^\.\/(?:index|tools|settings|batch)\.html(?:#[a-z0-9-]+)?$/i.test(value)) return { href: value };
  return null;
}

// Emit only markup we create. Raw HTML and unsupported Markdown remain text.
function inlineMarkdown(value, context, depth = 0) {
  if (depth > 8) return escapeManualHTML(value);
  let result = "";
  for (let index = 0; index < value.length;) {
    if (value[index] === "\\" && index + 1 < value.length && /[\\`*_[\]()|]/.test(value[index + 1])) {
      result += escapeManualHTML(value[index + 1]); index += 2; continue;
    }
    if (value[index] === "`") {
      const delimiter = /^`+/.exec(value.slice(index))[0];
      const end = value.indexOf(delimiter, index + delimiter.length);
      if (end >= 0) { result += `<code>${escapeManualHTML(value.slice(index + delimiter.length, end))}</code>`; index = end + delimiter.length; continue; }
    }
    if (value[index] === "[") {
      const link = /^\[([^\]\n]+)\]\(([^)\n]+)\)/.exec(value.slice(index));
      if (link) {
        const target = safeLink(link[2], context.chapter, context.chapterIds);
        const label = inlineMarkdown(link[1], context, depth + 1);
        result += target ? `<a href="${escapeManualHTML(target.href)}"${target.internal ? " data-manual-route" : ""}${target.external ? ' target="_blank" rel="noopener noreferrer"' : ""}>${label}</a>` : label;
        index += link[0].length; continue;
      }
    }
    const delimiter = value.startsWith("**", index) ? "**" : value.startsWith("__", index) ? "__" : value.startsWith("~~", index) ? "~~" : value[index] === "*" ? "*" : value[index] === "_" && (index === 0 || !/[\p{L}\p{N}]/u.test(value[index - 1])) ? "_" : "";
    if (delimiter) {
      const end = value.indexOf(delimiter, index + delimiter.length);
      if (end > index + delimiter.length) {
        const tag = delimiter === "~~" ? "del" : delimiter.length === 2 ? "strong" : "em";
        result += `<${tag}>${inlineMarkdown(value.slice(index + delimiter.length, end), context, depth + 1)}</${tag}>`;
        index = end + delimiter.length; continue;
      }
    }
    result += escapeManualHTML(value[index]); index += 1;
  }
  return result;
}

function tableCells(line) {
  let text = line.trim();
  if (text.startsWith("|")) text = text.slice(1);
  if (text.endsWith("|") && !text.endsWith("\\|")) text = text.slice(0, -1);
  const cells = []; let cell = ""; let ticks = false;
  for (let i = 0; i < text.length; i += 1) {
    if (text[i] === "\\" && text[i + 1] === "|") { cell += "\\|"; i += 1; continue; }
    if (text[i] === "`") ticks = !ticks;
    if (text[i] === "|" && !ticks) { cells.push(cell.trim()); cell = ""; } else cell += text[i];
  }
  cells.push(cell.trim()); return cells;
}

function listMarker(line) {
  const match = /^(\s*)([-+*]|\d+[.)])\s+(.*)$/.exec(line);
  return match ? { indent: match[1].replaceAll("\t", "    ").length, ordered: /^\d/.test(match[2]), number: parseInt(match[2], 10), text: match[3] } : null;
}

export function renderManualMarkdown(markdown, { chapter = "getting-started", chapterIds = new Set([chapter]) } = {}) {
  const lines = String(markdown).replace(/^\uFEFF/, "").replace(/\r\n?/g, "\n").split("\n");
  const context = { chapter, chapterIds }; const headings = []; const records = []; const usedIds = new Set();
  let currentRecord = null; let index = 0;
  const record = (text) => {
    if (!currentRecord) { currentRecord = { section: "", title: "", text: "" }; records.push(currentRecord); }
    currentRecord.text += ` ${plainText(text)}`;
  };
  const isTable = (at) => at + 1 < lines.length && lines[at].includes("|") && tableCells(lines[at + 1]).every((cell) => /^:?-{3,}:?$/.test(cell)) && tableCells(lines[at + 1]).length > 1;
  const isBlock = (at) => !lines[at]?.trim() || /^\s{0,3}(?:#{1,4}\s|`{3,}|~{3,}|>\s?)/.test(lines[at]) || listMarker(lines[at]) || isTable(at) || /^\s*(?:---+|\*\*\*+)\s*$/.test(lines[at]);
  function renderList(indent) {
    const first = listMarker(lines[index]); const tag = first.ordered ? "ol" : "ul";
    let output = `<${tag}${first.ordered && first.number !== 1 ? ` start="${first.number}"` : ""}>`;
    while (index < lines.length) {
      const item = listMarker(lines[index]);
      if (!item || item.indent !== indent || item.ordered !== first.ordered) break;
      output += `<li>${inlineMarkdown(item.text, context)}`; record(item.text); index += 1;
      while (index < lines.length) {
        const child = listMarker(lines[index]);
        if (child && child.indent > indent) { output += renderList(child.indent); continue; }
        if (!child && lines[index].trim() && /^\s+/.test(lines[index]) && !isBlock(index)) { output += ` ${inlineMarkdown(lines[index].trim(), context)}`; record(lines[index]); index += 1; continue; }
        break;
      }
      output += "</li>";
      if (!lines[index]?.trim() && listMarker(lines[index + 1] || "")?.indent === indent) index += 1;
    }
    return output + `</${tag}>`;
  }
  let html = "";
  while (index < lines.length) {
    const line = lines[index];
    if (!line.trim()) { index += 1; continue; }
    const fence = /^\s{0,3}(`{3,}|~{3,})([^\s]*)\s*$/.exec(line);
    if (fence) {
      const code = []; index += 1;
      const close = new RegExp(`^\\s{0,3}${fence[1][0]}{${fence[1].length},}\\s*$`);
      while (index < lines.length && !close.test(lines[index])) code.push(lines[index++]);
      if (index < lines.length) index += 1;
      const language = fence[2].replace(/[^a-zA-Z0-9_-]/g, "");
      html += `<pre><code${language ? ` class="language-${language}"` : ""}>${escapeManualHTML(code.join("\n"))}</code></pre>`; record(code.join(" ")); continue;
    }
    const heading = /^\s{0,3}(#{1,4})\s+(.+?)\s*#*\s*$/.exec(line);
    if (heading) {
      const explicit = /\s*\{#([a-zA-Z][a-zA-Z0-9_-]*)\}\s*$/.exec(heading[2]);
      const title = explicit ? heading[2].slice(0, explicit.index).trim() : heading[2];
      const base = explicit ? explicit[1] : manualSlug(title); let id = base; let suffix = 1;
      while (usedIds.has(id)) id = `${base}-${suffix++}`;
      usedIds.add(id); const level = heading[1].length;
      headings.push({ id, level, title: plainText(title) });
      currentRecord = { section: id, title: plainText(title), text: plainText(title) }; records.push(currentRecord);
      html += `<h${level} id="${escapeManualHTML(id)}">${inlineMarkdown(title, context)}<a class="manual-heading-link" href="${routeHash(chapter, id)}" data-manual-route aria-label="${escapeManualHTML(plainText(title))}">#</a></h${level}>`;
      index += 1; continue;
    }
    if (isTable(index)) {
      const headers = tableCells(line); const alignment = tableCells(lines[index + 1]).map((cell) => cell.startsWith(":") && cell.endsWith(":") ? "center" : cell.endsWith(":") ? "right" : "left");
      const cellHTML = (text, column, tag) => `<${tag} style="text-align:${alignment[column] || "left"}">${inlineMarkdown(text, context)}</${tag}>`;
      html += `<div class="manual-table-scroll" tabindex="0" role="region" aria-label="${escapeManualHTML(headers.map(plainText).join(", "))}"><table><thead><tr>${headers.map((cell, column) => cellHTML(cell, column, "th")).join("")}</tr></thead><tbody>`;
      record(headers.join(" ")); index += 2;
      while (index < lines.length && lines[index].trim() && lines[index].includes("|")) {
        const cells = tableCells(lines[index++]); html += `<tr>${headers.map((_, column) => cellHTML(cells[column] || "", column, "td")).join("")}</tr>`; record(cells.join(" "));
      }
      html += "</tbody></table></div>"; continue;
    }
    const list = listMarker(line); if (list) { html += renderList(list.indent); continue; }
    if (/^\s*(?:---+|\*\*\*+)\s*$/.test(line)) { html += "<hr>"; index += 1; continue; }
    if (/^\s{0,3}>/.test(line)) {
      const quote = []; while (index < lines.length && /^\s{0,3}>/.test(lines[index])) quote.push(lines[index++].replace(/^\s{0,3}>\s?/, ""));
      html += `<blockquote><p>${inlineMarkdown(quote.join(" "), context)}</p></blockquote>`; record(quote.join(" ")); continue;
    }
    const paragraph = [line.trim()]; index += 1;
    while (index < lines.length && !isBlock(index)) paragraph.push(lines[index++].trim());
    html += `<p>${inlineMarkdown(paragraph.join(" "), context)}</p>`; record(paragraph.join(" "));
  }
  return { html, headings, records };
}

function validateManifest(manifest) {
  if (manifest?.version !== 1 || !Array.isArray(manifest.chapters) || !manifest.chapters.length) throw new Error("Invalid manual manifest");
  const ids = new Set();
  for (const chapter of manifest.chapters) {
    if (!/^[a-z][a-z0-9-]*$/.test(chapter.id) || ids.has(chapter.id)) throw new Error("Invalid manual chapter ID");
    ids.add(chapter.id);
    for (const language of ["en", "ko"]) {
      if (typeof chapter.title?.[language] !== "string" || chapter.file?.[language] !== `${chapter.id}.${language}.md`) throw new Error("Invalid manual chapter source");
    }
  }
  return manifest;
}

function highlightSnippet(text, query) {
  const terms = query.toLowerCase().trim().split(/\s+/).filter(Boolean); const lowered = text.toLowerCase();
  const first = Math.min(...terms.map((term) => lowered.indexOf(term)).filter((position) => position >= 0));
  const start = Number.isFinite(first) ? Math.max(0, first - 65) : 0;
  const snippet = text.slice(start, start + 240); const spans = [];
  for (const term of terms) {
    let at = snippet.toLowerCase().indexOf(term);
    while (at >= 0) { spans.push([at, at + term.length]); at = snippet.toLowerCase().indexOf(term, at + term.length); }
  }
  spans.sort((a, b) => a[0] - b[0]); const merged = [];
  for (const span of spans) { const previous = merged.at(-1); if (previous && span[0] <= previous[1]) previous[1] = Math.max(previous[1], span[1]); else merged.push(span); }
  let result = start ? "…" : ""; let at = 0;
  for (const span of merged) { result += escapeManualHTML(snippet.slice(at, span[0])) + `<mark>${escapeManualHTML(snippet.slice(span[0], span[1]))}</mark>`; at = span[1]; }
  return result + escapeManualHTML(snippet.slice(at)) + (start + 240 < text.length ? "…" : "");
}

export class GuideManual {
  constructor({ root = document, fetcher = (...args) => fetch(...args), sourceURL = "./manual/", language = getLanguage() } = {}) {
    this.root = root; this.fetcher = fetcher; this.sourceURL = sourceURL; this.language = getManualLanguage(language);
    this.chapterCache = new Map(); this.renderGeneration = 0; this.searchGeneration = 0; this.searchTimer = null; this.observer = null;
    this.languageGeneration = 0;
    this.content = root.querySelector("#manualContent"); this.status = root.querySelector("#manualStatus"); this.searchInput = root.querySelector("#manualSearch");
    this.onHashChange = () => { void this.openRoute(); }; this.onScroll = () => this.trackSection(); this.scrollScheduled = false;
    this.onLanguageChange = () => { void this.setLanguage(getLanguage()); };
  }
  get ui() { return labels[this.language]; }
  title(chapter) { return chapter.title[this.language] || chapter.title.en; }
  async initialize() {
    // Subscribe before the manifest fetch: desktop settings can arrive while
    // sources are loading, and the app language remains the only preference.
    window.addEventListener("idfAnalyzer:languageChanged", this.onLanguageChange);
    const response = await this.fetcher(`${this.sourceURL}manifest.json`); if (!response.ok) throw new Error("Manual manifest unavailable");
    this.manifest = validateManifest(await response.json()); this.chapterIds = new Set(this.manifest.chapters.map((chapter) => chapter.id));
    if (!validHistoryDepth(window.history.state?.manualDepth)) window.history.replaceState({ ...window.history.state, manualDepth: auxiliaryReturnDepth() - 1 }, "");
    this.root.addEventListener("click", (event) => {
      if (event.target.closest?.(".manual-skip-link")) { event.preventDefault(); this.content.scrollIntoView({ block: "start" }); this.content.focus({ preventScroll: true }); return; }
      const link = event.target.closest?.("a[data-manual-route]");
      if (!link || event.defaultPrevented || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
      event.preventDefault(); this.navigate(link.getAttribute("href"));
    });
    this.searchInput.addEventListener("input", () => { this.searchGeneration += 1; clearTimeout(this.searchTimer); this.searchTimer = setTimeout(() => { void this.search(this.searchInput.value); }, 160); });
    this.root.querySelector("#manualSearchClear").addEventListener("click", () => { this.clearSearch(); this.searchInput.focus(); });
    this.initializeDisclosures();
    this.root.addEventListener("keydown", (event) => {
      if (event.key === "/" && !event.ctrlKey && !event.altKey && !event.metaKey && !event.target.closest?.("input,textarea,select,[contenteditable]")) { event.preventDefault(); this.searchInput.focus(); }
      if (event.key === "Escape" && this.searchInput.value) { event.preventDefault(); this.clearSearch(); this.searchInput.focus(); }
    });
    window.addEventListener("hashchange", this.onHashChange);
    window.addEventListener("scroll", this.onScroll, { passive: true });
    this.localize(); await this.openRoute(); return this;
  }
  initializeDisclosures() {
    this.compactQuery = window.matchMedia("(max-width: 780px)");
    this.compactDisclosureState = new Map();
    const disclosures = [...this.root.querySelectorAll(".manual-disclosure")];
    disclosures.forEach((disclosure) => {
      this.compactDisclosureState.set(disclosure.id, false);
      // Record user intent before native activation. A deferred toggle event
      // can otherwise arrive after resizing and overwrite the mobile choice.
      disclosure.querySelector("summary").addEventListener("click", () => { if (this.compactQuery.matches) this.compactDisclosureState.set(disclosure.id, !disclosure.open); });
    });
    const apply = () => disclosures.forEach((disclosure) => { disclosure.open = this.compactQuery.matches ? this.compactDisclosureState.get(disclosure.id) : true; });
    this.compactQuery.addEventListener("change", apply); window.addEventListener("resize", apply); apply();
  }
  localize() {
    this.root.querySelectorAll("[data-manual-label]").forEach((element) => { element.textContent = this.ui[element.dataset.manualLabel] || element.textContent; element.lang = this.language; });
    this.root.querySelectorAll("[data-manual-aria]").forEach((element) => { element.setAttribute("aria-label", this.ui[element.dataset.manualAria]); });
    this.root.querySelector(".manual-layout").lang = this.language;
    this.root.querySelector("#manualChapters").setAttribute("aria-label", this.ui.chapters);
    this.root.querySelector("#manualSections").setAttribute("aria-label", this.ui.sections);
    this.searchInput.placeholder = this.ui.search; this.searchInput.setAttribute("aria-label", this.ui.search);
    this.root.querySelector("#manualChapters").innerHTML = this.manifest.chapters.map((chapter, index) => `<a href="${routeHash(chapter.id)}" data-manual-route><span class="manual-chapter-number">${String(index + 1).padStart(2, "0")}</span>${escapeManualHTML(this.title(chapter))}</a>`).join("");
    if (this.root === document) document.title = `SemanticIDF — ${this.ui.title}`;
  }
  async setLanguage(language) {
    const next = getManualLanguage(language); if (next === this.language) return;
    this.language = next; this.searchGeneration += 1; const generation = ++this.languageGeneration;
    clearTimeout(this.searchTimer);
    if (!this.manifest) {
      if (this.status.dataset.error) this.status.textContent = this.ui.unavailable;
      return;
    }
    this.localize();
    this.root.querySelector("#manualResultsList").replaceChildren();
    if (this.searchInput.value.trim()) this.root.querySelector("#manualSearchStatus").textContent = this.ui.searching;
    await this.openRoute();
    if (generation === this.languageGeneration && this.searchInput.value.trim()) await this.search(this.searchInput.value);
  }
  loadChapter(chapter, language = this.language) {
    const key = `${chapter.id}:${language}`;
    if (!this.chapterCache.has(key)) {
      const promise = this.fetcher(`${this.sourceURL}${chapter.file[language]}`).then(async (response) => {
        if (!response.ok) throw new Error(`Manual chapter unavailable: ${chapter.id}`);
        return renderManualMarkdown(await response.text(), { chapter: chapter.id, chapterIds: this.chapterIds });
      }).catch((error) => { this.chapterCache.delete(key); throw error; });
      this.chapterCache.set(key, promise);
    }
    return this.chapterCache.get(key);
  }
  readRoute() {
    const parts = window.location.hash.slice(1).split("/"); let chapter = ""; let section = "";
    try { chapter = decodeURIComponent(parts[0] || ""); section = decodeURIComponent(parts.slice(1).join("/")); } catch { /* Invalid escaped hashes use the first chapter. */ }
    if (chapter === "metric-catalog") return { chapter: "metrics", section: "metric-catalog" };
    const aliases = { start: { chapter: "getting-started" }, files: { chapter: "input" }, "input-views": { chapter: "input" }, cli: { chapter: "automation" }, "hvac-inspection": { chapter: "simulation", section: "hvac-frames" }, "batch-metrics": { chapter: "tools", section: "batch-metrics" }, "tools-diagnose": { chapter: "tools", section: "diagnose" }, settings: { chapter: "tools", section: "appearance" }, updates: { chapter: "reference" } };
    if (!this.chapterIds.has(chapter)) {
      const alias = aliases[chapter]; if (alias && this.chapterIds.has(alias.chapter)) return { chapter: alias.chapter, section: alias.section || section };
      chapter = this.manifest.chapters[0].id;
    }
    return { chapter, section };
  }
  navigate(hash) {
    this.clearSearch();
    if (window.location.hash === hash) void this.openRoute({ focus: true });
    else {
      const depth = validHistoryDepth(window.history.state?.manualDepth) ? window.history.state.manualDepth : 0;
      window.history.pushState({ ...window.history.state, manualDepth: depth + 1 }, "", hash);
      void this.openRoute({ focus: true });
    }
  }
  async openRoute({ focus = false } = {}) {
    const route = this.readRoute(); const chapter = this.manifest.chapters.find((item) => item.id === route.chapter); const generation = ++this.renderGeneration;
    this.status.textContent = this.ui.loading; delete this.status.dataset.error;
    const returnLink = this.root.querySelector("a[data-app-return]");
    const returnDepth = (validHistoryDepth(window.history.state?.manualDepth) ? window.history.state.manualDepth : 0) + 1;
    if (returnLink) returnLink.dataset.appReturnDepth = String(returnDepth);
    setAuxiliaryReturnDepth(returnDepth);
    try {
      const rendered = await this.loadChapter(chapter); if (generation !== this.renderGeneration) return;
      this.currentChapter = chapter; this.headings = rendered.headings; this.content.innerHTML = rendered.html; this.content.lang = this.language;
      this.status.textContent = ""; this.markChapter(chapter.id); this.renderSections(chapter, rendered.headings); this.renderPagination(chapter);
      if (chapter.id === "metrics") this.mountMetricCatalog(generation);
      this.observeHeadings();
      const target = route.section ? this.content.querySelector(`#${CSS.escape(route.section)}`) : null;
      if (target) { target.scrollIntoView({ block: "start" }); this.markSection(route.section); }
      else { this.content.scrollIntoView({ block: "start" }); this.markSection(rendered.headings[0]?.id || ""); }
      if (focus || this.focusOnNavigation) { this.content.focus({ preventScroll: true }); this.focusOnNavigation = false; }
    } catch {
      if (generation !== this.renderGeneration) return;
      this.content.replaceChildren(); this.status.textContent = this.ui.unavailable; this.status.dataset.error = "true";
      this.root.querySelector("#manualSections").replaceChildren(); this.root.querySelector("#manualPagination").replaceChildren(); this.markChapter(chapter.id);
    }
  }
  markChapter(id) {
    this.root.querySelectorAll("#manualChapters a").forEach((link) => { if (link.getAttribute("href") === routeHash(id)) link.setAttribute("aria-current", "page"); else link.removeAttribute("aria-current"); });
  }
  renderSections(chapter, headings) {
    this.root.querySelector("#manualSections").innerHTML = headings.filter((item) => item.level > 1).map((item) => `<a href="${routeHash(chapter.id, item.id)}" data-manual-route data-section="${escapeManualHTML(item.id)}" data-level="${item.level}">${escapeManualHTML(item.title)}</a>`).join("");
  }
  markSection(id) {
    this.root.querySelectorAll("#manualSections a").forEach((link) => { if (link.dataset.section === id) link.setAttribute("aria-current", "location"); else link.removeAttribute("aria-current"); });
  }
  renderPagination(chapter) {
    const index = this.manifest.chapters.indexOf(chapter); const previous = this.manifest.chapters[index - 1]; const next = this.manifest.chapters[index + 1];
    this.root.querySelector("#manualPagination").innerHTML = `${previous ? `<a href="${routeHash(previous.id)}" data-manual-route>← ${this.ui.previous}: ${escapeManualHTML(this.title(previous))}</a>` : ""}${next ? `<a class="manual-next" href="${routeHash(next.id)}" data-manual-route>${this.ui.next}: ${escapeManualHTML(this.title(next))} →</a>` : ""}`;
  }
  observeHeadings() {
    this.observer?.disconnect();
    if (typeof IntersectionObserver !== "function") return;
    this.observer = new IntersectionObserver(() => this.trackSection(), { rootMargin: "-20px 0px -65% 0px" });
    this.chapterHeadingElements().forEach((heading) => this.observer.observe(heading));
  }
  chapterHeadingElements() {
    return (this.headings || []).filter((heading) => heading.level > 1).map((heading) => this.content.querySelector(`#${CSS.escape(heading.id)}`)).filter(Boolean);
  }
  trackSection() {
    if (this.scrollScheduled) return; this.scrollScheduled = true;
    requestAnimationFrame(() => {
      this.scrollScheduled = false; const headings = this.chapterHeadingElements(); if (!headings.length) return;
      let active = headings[0]; for (const heading of headings) { if (heading.getBoundingClientRect().top <= 100) active = heading; else break; }
      this.markSection(active.id);
    });
  }
  clearSearch() {
    clearTimeout(this.searchTimer); this.searchGeneration += 1; this.searchInput.value = "";
    this.root.querySelector("#manualSearchResults").hidden = true; this.root.querySelector("#manualResultsList").replaceChildren();
  }
  async search(query) {
    const value = query.trim(); if (!value) { this.clearSearch(); return []; }
    const generation = ++this.searchGeneration; const language = this.language; const terms = value.toLowerCase().split(/\s+/);
    this.root.querySelector("#manualSearchResults").hidden = false; this.root.querySelector("#manualSearchStatus").textContent = this.ui.searching;
    const chapters = await Promise.allSettled(this.manifest.chapters.map(async (chapter) => ({ chapter, rendered: await this.loadChapter(chapter, language) })));
    let catalog = null; let localizedGuides = [];
    try { catalog = await this.loadMetricCatalog(); localizedGuides = await this.metricGuidesForLanguage(catalog.guides, language); } catch { /* Manual search remains available without the live catalog. */ }
    if (generation !== this.searchGeneration || language !== this.language) return [];
    const found = [];
    for (const item of chapters) {
      if (item.status !== "fulfilled") continue;
      const { chapter, rendered } = item.value;
      const records = [...rendered.records];
      if (chapter.id === "metrics" && catalog) records.push(...catalog.guides.map((guide, index) => ({ section: `metric-${manualSlug(guide.id)}`, title: `${localizedGuides[index]?.name || guide.name} (${guide.id})`, text: [...Object.values(guide), ...Object.values(localizedGuides[index] || {})].join(" ") })));
      for (const record of records) {
        const haystack = `${this.title(chapter)} ${record.title} ${record.text}`.toLowerCase();
        if (terms.every((term) => haystack.includes(term))) found.push({ chapter, ...record, score: terms.filter((term) => record.title.toLowerCase().includes(term)).length });
      }
    }
    found.sort((a, b) => b.score - a.score); const limited = found.slice(0, 60);
    this.root.querySelector("#manualSearchStatus").textContent = `${found.length ? this.ui.count(found.length) : this.ui.noResults}${chapters.some((item) => item.status === "rejected") ? ` ${this.ui.searchIncomplete}` : ""}`;
    this.root.querySelector("#manualResultsList").innerHTML = limited.map((item) => `<a class="manual-search-result" href="${routeHash(item.chapter.id, item.section)}" data-manual-route><strong>${escapeManualHTML(this.title(item.chapter))} › ${escapeManualHTML(item.title || this.title(item.chapter))}</strong><span>${highlightSnippet(item.text, value)}</span></a>`).join("");
    return limited;
  }
  loadMetricCatalog() {
    if (!this.metricCatalogPromise) {
      this.metricCatalogPromise = (async () => {
        let guides = null;
        try {
          const api = window.go?.main?.App;
          if (typeof api?.GetMetricGuides === "function") guides = await api.GetMetricGuides();
          else { const response = await this.fetcher("/api/metric-guides"); if (response.ok) guides = await response.json(); }
        } catch { /* Try the desktop bridge and bundled reference below. */ }
        if (!Array.isArray(guides) || !guides.length) {
          for (let attempt = 0; attempt < 20; attempt += 1) {
            const api = window.go?.main?.App;
            if (typeof api?.GetMetricGuides === "function") { try { guides = await api.GetMetricGuides(); } catch { /* Fall back to bundled reference. */ } break; }
            await new Promise((resolve) => setTimeout(resolve, 50));
          }
        }
        if (Array.isArray(guides) && guides.length) return { guides, bundled: false };
        const response = await this.fetcher(`${this.sourceURL}metric-guides.json`); if (!response.ok) throw new Error("Metric catalog unavailable");
        guides = await response.json(); if (!Array.isArray(guides) || !guides.length) throw new Error("Invalid metric catalog");
        return { guides, bundled: true };
      })().catch((error) => { this.metricCatalogPromise = null; throw error; });
    }
    return this.metricCatalogPromise;
  }
  async metricGuidesForLanguage(guides, language) {
    if (language !== "ko") return guides;
    if (!this.metricLocalizationPromise) {
      this.metricLocalizationPromise = this.fetcher(`${this.sourceURL}metric-guides.ko.json`).then(async (response) => {
        if (!response.ok) throw new Error("Korean metric reference unavailable");
        const overlay = await response.json();
        if (overlay?.version !== 1 || !overlay.guides || typeof overlay.guides !== "object" || Array.isArray(overlay.guides)) throw new Error("Invalid Korean metric reference");
        return overlay.guides;
      }).catch(() => { this.metricLocalizationPromise = null; return null; });
    }
    const entries = await this.metricLocalizationPromise;
    return entries ? guides.map((guide) => localizedMetricGuide(guide, entries[guide.id])) : guides;
  }
  async mountMetricCatalog(generation) {
    const language = this.language;
    let heading = this.content.querySelector("#metric-catalog");
    if (!heading) { heading = document.createElement("h2"); heading.id = "metric-catalog"; heading.textContent = this.ui.catalogTitle; this.content.append(heading); }
    const container = document.createElement("div"); container.id = "metricGuide"; container.className = "guide-metric-list"; container.textContent = this.ui.catalogLoading;
    let nextHeading = heading.nextElementSibling; while (nextHeading && !/^H[12]$/.test(nextHeading.tagName)) nextHeading = nextHeading.nextElementSibling;
    this.content.insertBefore(container, nextHeading);
    try {
      const catalog = await this.loadMetricCatalog(); const guides = await this.metricGuidesForLanguage(catalog.guides, language);
      if (generation !== this.renderGeneration || language !== this.language) return;
      const groups = new Map(); for (const guide of guides) { const category = guide.category || this.ui.uncategorized; if (!groups.has(category)) groups.set(category, []); groups.get(category).push(guide); }
      container.innerHTML = `${catalog.bundled ? `<p class="manual-status">${this.ui.catalogFallback}</p>` : ""}` + [...groups.entries()].map(([category, guides]) => `<details class="guide-metric-group" open><summary><span>${escapeManualHTML(category)}</span><span class="badge">${guides.length}</span></summary><div class="guide-metric-cards">${guides.map((guide) => `<article class="guide-metric-card" id="metric-${manualSlug(guide.id)}"><h3>${escapeManualHTML(guide.name)} <code>${escapeManualHTML(guide.id)}</code></h3><dl>${["unit", "source", "method", "assumptions", "missingData"].map((field) => `<dt>${escapeManualHTML(this.ui[field])}</dt><dd>${escapeManualHTML(field === "unit" ? guide.unit || "[-]" : guide[field])}</dd>`).join("")}</dl></article>`).join("")}</div></details>`).join("");
      const route = this.readRoute(); if (route.section.startsWith("metric-") && route.section !== "metric-catalog") { const target = this.content.querySelector(`#${CSS.escape(route.section)}`); if (target) { const group = target.closest("details"); if (group) group.open = true; target.scrollIntoView({ block: "start" }); this.markSection("metric-catalog"); } }
    } catch { if (generation === this.renderGeneration) container.textContent = this.ui.catalogUnavailable; }
  }
}

async function bootGuideManual() {
  renderAppInfo(); applyCachedAppSettings(); translatePage();
  // Old shared links may include a manual language. Keep their section and
  // other query parameters, but use the application's settings everywhere.
  const url = new URL(window.location.href);
  if (url.searchParams.has("lang")) { url.searchParams.delete("lang"); window.history.replaceState(window.history.state, "", url); }
  const manual = new GuideManual();
  const initialization = manual.initialize();
  // Cached settings paint immediately; the language event applies the final
  // backend snapshot even if it arrives before initialization finishes.
  void loadAndApplyAppSettings();
  try { await initialization; }
  catch { const status = document.querySelector("#manualStatus"); status.textContent = manual.ui.unavailable; status.dataset.error = "true"; }
}

if (document.querySelector("[data-guide-manual]")) void bootGuideManual();
