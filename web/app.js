const main = document.querySelector("#main");
const notice = document.querySelector("#notice");
const runButton = document.querySelector("#run");
const state = { view: "jobs", selected: null, timer: null, sort: "" };
const providerDefaults = {
  ollama: "http://127.0.0.1:11434/v1",
  openai: "https://api.openai.com/v1",
  anthropic: "https://api.anthropic.com",
  cursor: "",
  compatible: "",
};
let modelLoad = 0;

const settingsButton = document.querySelector("#settings");
const settingsMenu = document.querySelector("#settings-menu");
const settingsViews = new Set(["schedule", "context", "model"]);

document.querySelectorAll(".tabs button").forEach((button) => {
  button.addEventListener("click", () => show(button.dataset.view));
});
settingsButton.addEventListener("click", (event) => {
  event.stopPropagation();
  setSettingsOpen(settingsMenu.hidden);
});
settingsMenu.querySelectorAll("button").forEach((button) => {
  button.addEventListener("click", () => {
    setSettingsOpen(false);
    show(button.dataset.view);
  });
});
document.addEventListener("click", (event) => {
  if (!settingsMenu.hidden && !event.target.closest(".menu")) setSettingsOpen(false);
});
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && !settingsMenu.hidden) {
    setSettingsOpen(false);
    settingsButton.focus();
  }
});
runButton.addEventListener("click", () => startRun());

function setSettingsOpen(open) {
  settingsMenu.hidden = !open;
  settingsButton.setAttribute("aria-expanded", open ? "true" : "false");
}

function h(tag, attrs, kids) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    if (value == null || value === false) continue;
    if (key === "class") node.className = value;
    else if (key.startsWith("on") && typeof value === "function") node.addEventListener(key.slice(2), value);
    else node.setAttribute(key, value);
  }
  for (const kid of kids || []) {
    if (kid == null || kid === false) continue;
    node.append(kid.nodeType ? kid : document.createTextNode(String(kid)));
  }
  return node;
}

function showNotice(text, open) {
  if (!text) {
    notice.hidden = true;
    notice.textContent = "";
    return;
  }
  notice.hidden = false;
  notice.textContent = text;
  if (!open) setTimeout(() => { if (notice.textContent === text) notice.hidden = true; }, 5000);
}

async function api(path, opts = {}) {
  const res = await fetch(path, {
    method: opts.method || "GET",
    headers: { "Content-Type": "application/json" },
    body: opts.body == null ? undefined : JSON.stringify(opts.body),
  });
  const text = await res.text();
  let data = null;
  if (text) {
    try { data = JSON.parse(text); } catch { data = { error: text }; }
  }
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

function show(view) {
  const already = state.painted && state.view === view;
  state.view = view;
  document.querySelectorAll(".tabs button, .menu-pop button").forEach((button) => {
    button.classList.toggle("active", button.dataset.view === view);
  });
  settingsButton.classList.toggle("active", settingsViews.has(view));
  if (already) return;
  state.painted = true;
  const render = views[view] || views.jobs;
  render().catch((err) => {
    main.replaceChildren(h("p", { class: "empty" }, [err.message]));
  });
}

const views = { jobs: renderJobs, sources: renderSources, profile: renderProfile, schedule: renderSchedule, context: renderContext, model: renderModel };

function compensation(job) {
  const min = job.salary_min || 0;
  const max = job.salary_max || 0;
  if (!min && !max) return { text: "Unknown", known: false };
  const code = (job.currency || "").toUpperCase();
  const mark = code === "GBP" ? "£" : code === "EUR" ? "€" : "$";
  const suffix = code === "CAD" || code === "AUD" || (code && code !== "USD" && code !== "GBP" && code !== "EUR") ? " " + code : "";
  const fmt = (n) => mark + Number(n).toLocaleString("en-US");
  let text;
  if (min && max && min !== max) text = `${fmt(min)}–${fmt(max)}`;
  else if (min && max) text = fmt(min);
  else if (min) text = `${fmt(min)}+`;
  else text = `Up to ${fmt(max)}`;
  return { text: text + suffix, known: true };
}

function when(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString();
}

async function renderJobs() {
  const jobsCall = Array.isArray(state.sources) && state.sources.length === 0
    ? Promise.resolve({ jobs: [] })
    : api(jobsQuery());
  const [overview, list, sourceData] = await Promise.all([
    api("/api/overview"),
    jobsCall,
    api("/api/sources").catch(() => ({ sources: [] })),
  ]);
  if (state.view !== "jobs") return;
  state.jobs = list.jobs || [];
  state.sourceList = sourceData.sources || [];
  const counts = overview.counts || {};
  const purgeable = counts.purgeable || 0;
  const head = h("div", {}, [
    h("div", { class: "spread" }, [
      h("h1", {}, ["Roles"]),
      purgeable > 0
        ? h("button", {
            type: "button",
            class: "danger",
            title: "Remove roles that are not saved or favorited",
            onclick: () => purgeUnkept(purgeable),
          }, [purgeable === 1 ? "Clear 1 other role" : `Clear ${purgeable} other roles`])
        : null,
    ]),
    statusStats(counts),
  ]);
  const filters = h("div", { class: "filters" }, [
    h("input", { type: "search", id: "q", "aria-label": "Filter roles", placeholder: "Title, company, or location", value: state.q || "" }),
    h("select", { id: "status", "aria-label": "Which roles to show" }, statusOptions()),
    h("select", { id: "sort", class: "sort", "aria-label": "Sort roles" }, sortOptions()),
  ]);
  const sourceBar = sourceFilters(state.sourceList);
  const banner = overview.active
    ? h("p", { class: "help" }, [`Search running since ${when(overview.active.started_at)}. ${overview.active.message || ""}`])
    : overview.last
      ? h("p", { class: "help" }, [`Last ${overview.last.trigger} run ${overview.last.status}: fetched ${overview.last.fetched}, new ${overview.last.new}, matched ${overview.last.matched}, rejected ${overview.last.rejected}.`])
      : null;
  if (overview.active) poll(overview.active.id);
  const listCol = h("div", { class: "roles-col" }, [
    h("div", { class: "list-head" }, [head, banner, filters, sourceBar]),
    h("div", { id: "joblist", class: "joblist" }),
  ]);
  const splitter = h("button", {
    class: "splitter",
    type: "button",
    role: "separator",
    "aria-orientation": "vertical",
    "aria-label": "Resize roles and job details",
  });
  const detail = h("aside", { class: "panel", id: "detail" }, [
    h("div", { class: "empty-detail" }, [
      h("h2", {}, [state.selected ? "Loading role" : "Select a role"]),
      h("p", { class: "muted" }, ["The posting, fit notes, and actions show up here."]),
    ]),
  ]);
  const layout = h("div", { class: "layout" }, [listCol, splitter, detail]);
  main.replaceChildren(layout);
  mountSplit(layout, splitter, detail);
  const q = main.querySelector("#q");
  const sel = main.querySelector("#status");
  const sort = main.querySelector("#sort");
  sel.value = state.status == null ? "matched,saved,new" : state.status;
  sort.value = state.sort || "";
  q.addEventListener("input", () => { state.q = q.value; paintJobList(); });
  sel.addEventListener("change", () => { state.status = sel.value; renderJobs(); });
  sort.addEventListener("change", () => { state.sort = sort.value; renderJobs(); });
  const opened = state.selected;
  paintJobList();
  if (state.selected && state.selected === opened) loadDetail(state.selected);
}

function currentStatus() {
  return state.status == null ? "matched,saved,new" : state.status;
}

async function purgeUnkept(n) {
  const noun = n === 1 ? "role" : "roles";
  if (!confirm(`Remove ${n} ${noun} that are not saved or favorited? Saved and starred roles stay. A later search can find the others again.`)) return;
  try {
    const res = await api("/api/jobs/purge", { method: "POST", body: {} });
    const removed = res.removed || 0;
    showNotice(removed === 1 ? "Removed 1 role." : `Removed ${removed} roles.`);
    renderJobs();
  } catch (err) {
    showNotice(err.message);
  }
}

function statusStats(counts) {
  const current = currentStatus();
  const open = (counts.matched || 0) + (counts.saved || 0) + (counts.new || 0);
  const chips = [
    ["Open", open, "matched,saved,new"],
    ["Matched", counts.matched || 0, "matched"],
    ["Saved", counts.saved || 0, "saved"],
    ["Favorites", counts.favorite || 0, "favorite"],
    ["New", counts.new || 0, "new"],
    ["Filtered", counts.rejected || 0, "rejected"],
  ];
  return h("div", { class: "stats", role: "group", "aria-label": "Filter by status" }, chips.map(([label, count, value]) =>
    h("button", {
      type: "button",
      class: "stat" + (current === value ? " on" : ""),
      "aria-pressed": current === value ? "true" : "false",
      onclick: () => { state.status = value; renderJobs(); },
    }, [h("strong", {}, [String(count)]), label])));
}

function detailWidth() {
  const saved = Number(localStorage.getItem("jobfinder.detailWidth"));
  if (saved >= 360) return saved;
  return 560;
}

function mountSplit(layout, handle, panel) {
  const apply = (px) => {
    const total = layout.getBoundingClientRect().width;
    const cap = Math.max(300, total - 440);
    const width = Math.min(cap, Math.max(300, px));
    panel.style.flexBasis = width + "px";
    panel.style.width = width + "px";
    return width;
  };
  apply(detailWidth());
  const remember = (px) => {
    try { localStorage.setItem("jobfinder.detailWidth", String(Math.round(px))); } catch { /* ignore */ }
  };
  handle.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) return;
    event.preventDefault();
    handle.setPointerCapture(event.pointerId);
    handle.classList.add("dragging");
    const previous = document.body.style.userSelect;
    document.body.style.userSelect = "none";
    const move = (ev) => {
      const rect = layout.getBoundingClientRect();
      apply(rect.right - ev.clientX);
    };
    const stop = (ev) => {
      handle.classList.remove("dragging");
      document.body.style.userSelect = previous;
      const rect = layout.getBoundingClientRect();
      remember(apply(rect.right - ev.clientX));
      handle.removeEventListener("pointermove", move);
      handle.removeEventListener("pointerup", stop);
      handle.removeEventListener("pointercancel", stop);
    };
    handle.addEventListener("pointermove", move);
    handle.addEventListener("pointerup", stop);
    handle.addEventListener("pointercancel", stop);
  });
  handle.addEventListener("keydown", (event) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const current = panel.getBoundingClientRect().width;
    const next = current + (event.key === "ArrowLeft" ? 40 : -40);
    remember(apply(next));
  });
}

function paintJobList() {
  const host = document.querySelector("#joblist");
  if (!host) return;
  const jobs = (state.jobs || []).filter((job) => !queryValue() || matches(job, queryValue()));
  const previous = state.selected;
  if (!jobs.length) state.selected = null;
  else if (!jobs.some((job) => job.id === state.selected)) state.selected = jobs[0].id;
  const empty = Array.isArray(state.sources) && state.sources.length === 0
    ? "No sources selected. Choose one above to see its roles."
    : "Nothing in this view. Run a search, or choose another status or source.";
  const rows = jobs.length
    ? jobs.map((job) => jobButton(job))
    : [h("p", { class: "empty" }, [empty])];
  host.replaceChildren(...rows);
  if (!jobs.length) {
    const panel = document.querySelector("#detail");
    if (panel) panel.replaceChildren(h("div", { class: "empty-detail" }, [
      h("h2", {}, ["No roles"]),
      h("p", { class: "muted" }, [empty]),
    ]));
    return;
  }
  if (state.selected && state.selected !== previous) loadDetail(state.selected);
}

function jobsQuery() {
  const params = new URLSearchParams();
  if (state.status === "favorite") params.set("favorite", "1");
  else params.set("status", statusValue());
  if (state.sort) params.set("sort", state.sort);
  if (Array.isArray(state.sources) && state.sources.length) params.set("source", state.sources.join(","));
  return "/api/jobs?" + params.toString();
}

function sourceFilters(sources) {
  const allOn = state.sources == null;
  const chips = [
    h("button", {
      type: "button",
      class: "source-chip" + (allOn ? " on" : ""),
      "aria-pressed": allOn ? "true" : "false",
      onclick: () => { state.sources = null; renderJobs(); },
    }, ["All sources"]),
  ];
  for (const src of sources) {
    const on = allOn || state.sources.includes(src.id);
    chips.push(h("button", {
      type: "button",
      class: "source-chip" + (on ? " on" : ""),
      "aria-pressed": on ? "true" : "false",
      onclick: () => toggleListedSource(src.id, sources),
    }, [
      sourceLogo({ source_kind: src.kind, source_name: src.name }),
      src.name || src.kind,
    ]));
  }
  return h("div", { class: "source-filters", role: "group", "aria-label": "Sources to view" }, chips);
}

function toggleListedSource(id, sources) {
  const ids = sources.map((src) => src.id);
  let next = state.sources == null ? ids.slice() : state.sources.slice();
  next = next.includes(id) ? next.filter((item) => item !== id) : next.concat(id);
  state.sources = next.length === ids.length ? null : next;
  renderJobs();
}

const sourceDomains = {
  remotive: "remotive.com",
  remoteok: "remoteok.com",
  arbeitnow: "arbeitnow.com",
  linkedin: "linkedin.com",
  adzuna: "adzuna.com",
  usajobs: "usajobs.gov",
  greenhouse: "greenhouse.io",
  lever: "lever.co",
  ashby: "ashbyhq.com",
  slack: "slack.com",
};

function sourceHost(job) {
  const kind = job.source_kind || "";
  if (kind === "jsearch" || kind === "rss" || kind === "json") {
    const fromURL = hostOf(job.url);
    if (fromURL) return fromURL;
  }
  return sourceDomains[kind] || hostOf(job.url);
}

function hostOf(raw) {
  try {
    const url = new URL(raw);
    if (url.protocol === "http:" || url.protocol === "https:") return url.hostname;
  } catch { /* the posting has no usable link */ }
  return "";
}

function sourceLogo(job) {
  const label = job.source_name || job.source_kind || "Source";
  const fallback = () => h("span", { class: "logo-fallback", "aria-hidden": "true" }, [label.slice(0, 1).toUpperCase()]);
  const host = sourceHost(job);
  if (!host) return h("span", { class: "source-logo-wrap", title: label }, [fallback()]);
  const img = h("img", {
    class: "source-logo",
    alt: "",
    width: "20",
    height: "20",
    src: "https://www.google.com/s2/favicons?domain=" + encodeURIComponent(host) + "&sz=64",
    referrerpolicy: "no-referrer",
    loading: "lazy",
  });
  img.addEventListener("error", () => img.replaceWith(fallback()));
  return h("span", { class: "source-logo-wrap", title: label }, [img]);
}

function statusValue() {
  if (state.status == null) return "matched,saved,new";
  return state.status;
}
function queryValue() {
  return (state.q || "").trim().toLowerCase();
}
function matches(job, q) {
  return [job.title, job.company, job.location, job.fit_summary, job.source_name].join(" ").toLowerCase().includes(q);
}
function statusOptions() {
  const options = [
    ["matched,saved,new", "Open"],
    ["matched", "Matched"],
    ["matched,saved", "Matched and saved"],
    ["saved", "Saved"],
    ["favorite", "Favorites"],
    ["new", "Not scored"],
    ["rejected", "Filtered out"],
    ["dismissed", "Dismissed"],
    ["", "Everything"],
  ];
  return options.map(([value, label]) => h("option", { value }, [label]));
}
function sortOptions() {
  const options = [
    ["", "Best match"],
    ["start_desc", "Starting pay, high to low"],
    ["start_asc", "Starting pay, low to high"],
    ["high_desc", "Highest pay, high to low"],
    ["high_asc", "Highest pay, low to high"],
  ];
  return options.map(([value, label]) => h("option", { value }, [label]));
}

function fitPoints(job) {
  const pros = Array.isArray(job.fit_pros) ? job.fit_pros.filter(Boolean) : [];
  const cons = Array.isArray(job.fit_cons) ? job.fit_cons.filter(Boolean) : [];
  if (pros.length || cons.length) return { pros, cons };
  return parseFitSummary(job.fit_summary || "");
}

function parseFitSummary(summary) {
  const text = String(summary || "").trim();
  if (!text) return { pros: [], cons: [] };
  const lines = text.split("\n").map((line) => line.trim()).filter(Boolean);
  if (lines.some((line) => line.startsWith("+ ") || line.startsWith("- ") || line.startsWith("− "))) {
    const pros = [];
    const cons = [];
    for (const line of lines) {
      if (line.startsWith("+ ")) pros.push(line.slice(2).trim());
      else if (line.startsWith("- ")) cons.push(line.slice(2).trim());
      else if (line.startsWith("− ")) cons.push(line.slice(2).trim());
    }
    return { pros, cons };
  }
  const gapAt = text.toLowerCase().indexOf("gaps:");
  if (gapAt < 0) return { pros: [], cons: [] };
  const head = text.slice(0, gapAt).trim();
  const tail = text.slice(gapAt + "gaps:".length).trim();
  const pros = head ? head.split(/(?<=[.!?])\s+/).map((part) => part.trim()).filter(Boolean) : [];
  const cons = tail.split(";").map((part) => part.trim().replace(/[.]+$/, "")).filter(Boolean);
  return { pros, cons };
}

function fitPreview(job) {
  const { pros, cons } = fitPoints(job);
  if (!pros.length && !cons.length) {
    return job.fit_summary ? h("p", { class: "summary" }, [job.fit_summary]) : null;
  }
  const line = (text, sign, cls) => h("li", { class: cls }, [h("span", { class: "sign" }, [sign]), text]);
  const items = [
    ...pros.slice(0, 1).map((row) => line(row, "+", "fit-pros")),
    ...cons.slice(0, 1).map((row) => line(row, "−", "fit-cons")),
  ];
  const extra = pros.length + cons.length - items.length;
  return h("ul", { class: "fit fit-preview" }, [
    ...items,
    extra > 0 ? h("li", { class: "fit-more" }, [extra === 1 ? "1 more point" : extra + " more points"]) : null,
  ]);
}

function fitLists(job) {
  const { pros, cons } = fitPoints(job);
  if (!pros.length && !cons.length) {
    return job.fit_summary ? h("p", { class: "summary" }, [job.fit_summary]) : null;
  }
  const items = (rows, sign, cls) => rows.length
    ? h("ul", { class: cls }, rows.map((row) => h("li", {}, [h("span", { class: "sign" }, [sign]), row])))
    : null;
  return h("div", { class: "fit" }, [items(pros, "+", "fit-pros"), items(cons, "−", "fit-cons")]);
}

function starGlyph(on) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
  path.setAttribute("d", "M12 3.4l2.3 4.9 5.4.7-4 3.7 1 5.3L12 15.6 7.3 18l1-5.3-4-3.7 5.4-.7z");
  path.setAttribute("fill", on ? "currentColor" : "none");
  path.setAttribute("stroke", "currentColor");
  path.setAttribute("stroke-width", "1.6");
  path.setAttribute("stroke-linejoin", "round");
  svg.append(path);
  return svg;
}

function starButton(job, inList) {
  const on = !!job.favorite;
  return h("button", {
    type: "button",
    class: "star" + (on ? " on" : "") + (inList ? " in-list" : ""),
    "aria-pressed": on ? "true" : "false",
    "aria-label": on ? "Remove favorite" : "Favorite",
    title: on ? "Remove favorite" : "Favorite",
    onclick: (event) => {
      event.stopPropagation();
      toggleFavorite(job);
    },
  }, [starGlyph(on)]);
}

function jobButton(job) {
  const score = job.score < 0 ? "–" : String(job.score);
  const klass = job.score >= 70 ? "score high" : job.score >= 0 && job.score < 50 ? "score low" : "score";
  return h("div", {
    class: "job" + (state.selected === job.id ? " selected" : ""),
  }, [
    h("button", {
      class: "job-open",
      type: "button",
      onclick: (event) => {
        state.selected = job.id;
        document.querySelectorAll(".job").forEach((node) => node.classList.remove("selected"));
        event.currentTarget.closest(".job").classList.add("selected");
        loadDetail(job.id);
      },
    }, [
      h("div", { class: klass }, [score]),
      h("div", { class: "job-main" }, [
        h("h3", {}, [job.title || "Untitled"]),
        h("div", { class: "meta" }, [
          [job.company, job.location || (job.remote ? "Remote" : "")].filter(Boolean).join(" · "),
          " ",
          h("span", { class: "pill " + job.status }, [job.status]),
        ]),
        fitPreview(job),
      ]),
      paySide(job),
    ]),
    starButton(job, true),
  ]);
}

async function loadDetail(id) {
  const panel = document.querySelector("#detail");
  if (!panel) return;
  let job;
  try {
    job = await api("/api/jobs/" + encodeURIComponent(id));
  } catch (err) {
    if (state.selected === id) panel.replaceChildren(h("p", {}, [err.message]));
    return;
  }
  if (state.selected !== id || state.view !== "jobs" || !panel.isConnected) return;
  const link = job.url ? h("a", { class: "ghost", href: job.url, target: "_blank", rel: "noreferrer" }, ["Open posting"]) : null;
  const pay = compensation(job);
  const sourceName = job.source_name || job.source_kind || "Source";
  panel.replaceChildren(
    h("section", { class: pay.known ? "pay-standout" : "pay-standout unknown" }, [
      h("div", { class: "source-mark" }, [sourceLogo(job), h("span", { class: "source-name" }, [sourceName])]),
      h("span", { class: "label" }, ["Compensation"]),
      h("strong", {}, [pay.text]),
    ]),
    h("div", { class: "detail-sticky" }, [
      h("div", { class: "spread" }, [
        h("h2", {}, [job.title || "Untitled"]),
        starButton(job),
      ]),
      h("p", { class: "muted" }, [[job.company, job.location].filter(Boolean).join(" · ")]),
      h("div", { class: "row" }, [
        h("span", { class: "pill " + job.status }, [job.status]),
        link,
        job.status === "saved"
          ? h("button", { class: "ghost", type: "button", onclick: () => setStatus(job.id, "matched") }, ["Remove from saved"])
          : h("button", { class: "primary", type: "button", onclick: () => setStatus(job.id, "saved") }, ["Save"]),
        h("button", { class: "ghost", type: "button", onclick: () => setStatus(job.id, "dismissed") }, ["Dismiss"]),
        h("button", { class: "ghost", type: "button", id: "research", onclick: () => research(job.id) }, ["Research role"]),
      ]),
    ]),
    fitLists(job),
    job.filter_reason ? h("p", { class: "help" }, [job.filter_reason]) : null,
    h("h3", {}, ["Posting"]),
    h("div", { class: "pre" }, [job.description || "No description was stored."]),
    h("h3", {}, ["Research"]),
    h("div", { class: "pre", id: "brief" }, [job.research || "No briefing yet."]),
  );
}

async function toggleFavorite(job) {
  try {
    await api("/api/jobs/" + encodeURIComponent(job.id), { method: "PATCH", body: { favorite: !job.favorite } });
    showNotice(job.favorite ? "Removed from favorites." : "Favorited.");
    state.selected = job.id;
    await renderJobs();
  } catch (err) {
    showNotice(err.message, true);
  }
}

async function setStatus(id, status) {
  try {
    await api("/api/jobs/" + encodeURIComponent(id), { method: "PATCH", body: { status } });
    const note = status === "saved" ? "Saved." : status === "matched" ? "Removed from saved." : "Dismissed.";
    showNotice(note);
    state.selected = id;
    await renderJobs();
  } catch (err) {
    showNotice(err.message, true);
  }
}

async function research(id) {
  const button = document.querySelector("#research");
  if (button) {
    button.disabled = true;
    button.textContent = "Researching…";
  }
  try {
    const job = await api("/api/jobs/" + encodeURIComponent(id) + "/research", { method: "POST", body: {} });
    const brief = document.querySelector("#brief");
    if (brief) brief.textContent = job.research || "";
    showNotice("Briefing saved.");
  } catch (err) {
    showNotice(err.message, true);
  } finally {
    if (button) {
      button.disabled = false;
      button.textContent = "Research role";
    }
  }
}

async function startRun() {
  runButton.disabled = true;
  runButton.textContent = "Searching…";
  try {
    const run = await api("/api/runs", { method: "POST", body: { force: document.querySelector("#force").checked } });
    poll(run.id);
  } catch (err) {
    runButton.disabled = false;
    runButton.textContent = "Run search";
    showNotice(err.message, true);
  }
}

function poll(id) {
  if (state.timer) return;
  state.timer = setInterval(async () => {
    try {
      const run = await api("/api/runs/" + encodeURIComponent(id));
      if (run.status === "running") return;
      clearInterval(state.timer);
      state.timer = null;
      runButton.disabled = false;
      runButton.textContent = "Run search";
      const extra = run.message && run.message !== "finished" ? " " + run.message.split("\n")[0] : "";
      showNotice(`Search ${run.status}. ${run.matched} matched, ${run.new} new.${extra}`, true);
      if (state.view === "jobs") renderJobs();
    } catch (err) {
      clearInterval(state.timer);
      state.timer = null;
      runButton.disabled = false;
      runButton.textContent = "Run search";
      showNotice(err.message, true);
    }
  }, 1200);
}

async function renderSources() {
  const [kinds, data] = await Promise.all([api("/api/source-kinds"), api("/api/sources")]);
  if (state.view !== "sources") return;
  state.kinds = kinds.kinds || [];
  const sources = data.sources || [];
  const quick = h("div", { class: "kinds" }, state.kinds.filter((kind) => kind.public).map((kind) =>
    h("button", { class: "ghost", type: "button", onclick: () => addKind(kind) }, ["Add " + kind.label])));
  const list = h("div", {}, sources.map((src) => h("div", { class: "source" }, [
    sourceLogo({ source_kind: src.kind, source_name: src.name }),
    h("div", {}, [
      h("strong", {}, [src.name]),
      h("div", { class: "meta" }, [src.kind]),
    ]),
    h("div", { class: "source-actions" }, [
      sourceSwitch(src),
      h("button", { class: "danger", type: "button", onclick: () => removeSource(src) }, ["Remove"]),
    ]),
  ])));
  const form = h("form", { class: "stack", onsubmit: (event) => saveSource(event) }, [
    h("label", { class: "field" }, [h("span", {}, ["Kind"]), h("select", { id: "kind", name: "kind", onchange: () => fillFields() }, state.kinds.map((kind) => h("option", { value: kind.kind }, [kind.label])))]),
    h("p", { class: "help", id: "kind-help" }, [""]),
    h("label", { class: "field" }, [h("span", {}, ["Name"]), h("input", { name: "name", type: "text", required: "true" })]),
    h("div", { id: "fields", class: "stack" }),
    h("label", { class: "switch-field" }, [
      h("input", { class: "switch-input", name: "enabled", type: "checkbox", checked: "checked" }),
      h("span", { class: "switch-ui", "aria-hidden": "true" }),
      "Enabled",
    ]),
    h("button", { class: "primary", type: "submit" }, ["Save source"]),
  ]);
  main.replaceChildren(h("div", { class: "page" }, [
    h("h1", {}, ["Sources"]),
    h("p", { class: "muted" }, ["Turn on the boards you want searched. LinkedIn uses public guest listings. Indeed and Dice need a JSearch key."]),
    h("section", { class: "card stack" }, [h("h2", {}, ["Your sources"]), sources.length ? list : h("p", { class: "muted" }, ["No sources yet."]), quick]),
    h("section", { class: "card" }, [h("h2", {}, ["Add a source"]), form]),
  ]));
  fillFields();
}

function selectedKind() {
  const value = document.querySelector("#kind")?.value;
  return (state.kinds || []).find((kind) => kind.kind === value);
}

function fillFields() {
  const kind = selectedKind();
  const help = document.querySelector("#kind-help");
  const box = document.querySelector("#fields");
  if (!kind || !box) return;
  if (help) help.textContent = kind.summary || "";
  const name = document.querySelector('input[name="name"]');
  if (name && !name.dataset.touched) name.value = kind.label;
  box.replaceChildren(...(kind.fields || []).map((field) => h("label", { class: "field" }, [
    h("span", {}, [field.label + (field.required ? " *" : "")]),
    field.kind === "textarea"
      ? h("textarea", { name: "cfg_" + field.name, placeholder: field.placeholder || "" })
      : h("input", { name: "cfg_" + field.name, type: field.kind === "password" ? "password" : "text", placeholder: field.placeholder || "" }),
    field.help ? h("span", { class: "help" }, [field.help]) : null,
  ])));
}

async function addKind(kind) {
  try {
    await api("/api/sources", { method: "POST", body: { kind: kind.kind, name: kind.label, enabled: true, config: {} } });
    showNotice(kind.label + " added.");
    renderSources();
  } catch (err) {
    showNotice(err.message, true);
  }
}

function saveSource(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const kind = selectedKind();
  const config = {};
  for (const field of kind.fields || []) {
    const input = form.elements["cfg_" + field.name];
    if (input && input.value.trim()) config[field.name] = input.value.trim();
  }
  api("/api/sources", {
    method: "POST",
    body: {
      kind: kind.kind,
      name: form.elements.name.value.trim(),
      enabled: form.elements.enabled.checked,
      config,
    },
  }).then(() => {
    showNotice("Source saved.");
    renderSources();
  }).catch((err) => showNotice(err.message, true));
}

function sourceSwitch(src) {
  const on = !!src.enabled;
  const name = src.name || src.kind || "source";
  return h("button", {
    type: "button",
    class: "switch" + (on ? " on" : ""),
    role: "switch",
    "aria-checked": on ? "true" : "false",
    "aria-label": (on ? "Disable " : "Enable ") + name,
    onclick: () => toggleSource(src),
  }, [h("span", { class: "knob" })]);
}

async function toggleSource(src) {
  try {
    await api("/api/sources/" + encodeURIComponent(src.id), {
      method: "PUT",
      body: { kind: src.kind, name: src.name, enabled: !src.enabled, config: src.config || {} },
    });
    renderSources();
  } catch (err) {
    showNotice(err.message, true);
  }
}

async function removeSource(src) {
  if (!confirm("Remove " + src.name + "? Stored jobs stay.")) return;
  try {
    await api("/api/sources/" + encodeURIComponent(src.id), { method: "DELETE" });
    renderSources();
  } catch (err) {
    showNotice(err.message, true);
  }
}

async function renderProfile() {
  const profile = await api("/api/profile");
  if (state.view !== "profile") return;
  const form = h("form", { class: "stack", onsubmit: (event) => saveProfile(event) }, [
    h("label", { class: "field" }, [h("span", {}, ["What you want"]), h("textarea", { name: "notes" }, [profile.notes || ""])]),
    h("div", { class: "grid-2" }, [
      field("Keywords, any of these", "keywords", (profile.keywords || []).join(", ")),
      field("Must have, all of these", "must_have", (profile.must_have || []).join(", ")),
      field("Exclude", "exclude", (profile.exclude || []).join(", ")),
      field("Locations", "locations", (profile.locations || []).join(", ")),
      field("Employment types", "employment_types", (profile.employment_types || []).join(", "), "full_time, contract"),
      num("Minimum pay", "min_salary", profile.min_salary || 0),
      num("Minimum model score", "min_score", profile.min_score ?? 70),
    ]),
    h("label", { class: "check" }, [h("input", { type: "checkbox", name: "remote_only", checked: profile.remote_only ? "checked" : null }), "Remote only"]),
    h("label", { class: "check" }, [h("input", { type: "checkbox", name: "include_remote", checked: profile.include_remote ? "checked" : null }), "Remote roles can satisfy a location"]),
    h("p", { class: "help" }, ["Pay is compared as published, with no currency conversion. A role with no salary is kept. Keywords and excludes run before any model call."]),
    h("button", { class: "primary", type: "submit" }, ["Save profile"]),
  ]);
  main.replaceChildren(h("div", { class: "page" }, [h("h1", {}, ["Profile"]), h("section", { class: "card" }, [form])]));
}

function field(label, name, value, placeholder) {
  return h("label", { class: "field" }, [h("span", {}, [label]), h("input", { type: "text", name, value: value || "", placeholder: placeholder || "" })]);
}
function num(label, name, value) {
  return h("label", { class: "field" }, [h("span", {}, [label]), h("input", { type: "number", name, value: String(value) })]);
}
function listSplit(value) {
  return value.split(",").map((part) => part.trim()).filter(Boolean);
}

function saveProfile(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const body = {
    notes: form.elements.notes.value,
    keywords: listSplit(form.elements.keywords.value),
    must_have: listSplit(form.elements.must_have.value),
    exclude: listSplit(form.elements.exclude.value),
    locations: listSplit(form.elements.locations.value),
    employment_types: listSplit(form.elements.employment_types.value),
    min_salary: Number(form.elements.min_salary.value || 0),
    min_score: Number(form.elements.min_score.value || 0),
    remote_only: form.elements.remote_only.checked,
    include_remote: form.elements.include_remote.checked,
  };
  api("/api/profile", { method: "PUT", body }).then(() => showNotice("Profile saved.")).catch((err) => showNotice(err.message, true));
}

async function renderSchedule() {
  const data = await api("/api/schedule");
  const runs = await api("/api/runs");
  if (state.view !== "schedule") return;
  const form = h("form", { class: "stack", onsubmit: (event) => saveSchedule(event) }, [
    h("label", { class: "field" }, [h("span", {}, ["Cron expression"]), h("input", { name: "schedule", type: "text", value: data.schedule || "", placeholder: "0 8 * * 1-5" })]),
    h("label", { class: "field" }, [h("span", {}, ["Timezone"]), h("input", { name: "timezone", type: "text", value: data.timezone || "Local", placeholder: "America/Los_Angeles" })]),
    h("p", { class: "help", id: "next" }, [data.next ? "Next run " + when(data.next) + "." : "No schedule. Searches run only when you click Run search."]),
    h("div", { class: "kinds" }, [
      preset("Weekdays at 8:00", "0 8 * * 1-5"),
      preset("Every hour", "@hourly"),
      preset("Daily at 7:00", "0 7 * * *"),
      preset("Turn off", ""),
    ]),
    h("button", { class: "primary", type: "submit" }, ["Save schedule"]),
  ]);
  const rows = (runs.runs || []).map((run) => h("tr", {}, [
    h("td", {}, [when(run.started_at)]),
    h("td", {}, [run.trigger]),
    h("td", {}, [run.status]),
    h("td", {}, [`${run.fetched} fetched · ${run.matched} matched · ${run.new} new`]),
    h("td", {}, [run.message || ""]),
  ]));
  main.replaceChildren(h("div", { class: "page" }, [
    h("h1", {}, ["Schedule"]),
    h("section", { class: "card" }, [form]),
    h("section", { class: "card" }, [
      h("h2", {}, ["Recent runs"]),
      rows.length ? h("div", { class: "table-wrap" }, [h("table", {}, [h("thead", {}, [h("tr", {}, [h("th", {}, ["When"]), h("th", {}, ["Trigger"]), h("th", {}, ["Status"]), h("th", {}, ["Counts"]), h("th", {}, ["Notes"])])]), h("tbody", {}, rows)])]) : h("p", { class: "muted" }, ["No runs yet."]),
    ]),
  ]));
}

function preset(label, expr) {
  return h("button", { class: "ghost", type: "button", onclick: () => { const input = document.querySelector('input[name="schedule"]'); if (input) input.value = expr; } }, [label]);
}

function saveSchedule(event) {
  event.preventDefault();
  const form = event.currentTarget;
  api("/api/schedule", { method: "PUT", body: { schedule: form.elements.schedule.value.trim(), timezone: form.elements.timezone.value.trim() || "Local" } })
    .then((data) => {
      const next = document.querySelector("#next");
      if (next) next.textContent = data.next ? "Next run " + when(data.next) + "." : "Schedule off.";
      showNotice("Schedule saved.");
    })
    .catch((err) => showNotice(err.message, true));
}

async function renderContext() {
  const [ctx, settings] = await Promise.all([api("/api/context"), api("/api/settings")]);
  if (state.view !== "context") return;
  state.settings = settings;
  const kiwi = settings.kiwi || {};
  const form = h("form", { class: "stack", onsubmit: (event) => saveKiwi(event, settings) }, [
    h("label", { class: "check" }, [h("input", { type: "checkbox", name: "enabled", checked: kiwi.enabled ? "checked" : null }), "Also write pages to KiwiFS"]),
    h("label", { class: "field" }, [h("span", {}, ["KiwiFS base URL"]), h("input", { name: "base_url", type: "text", value: kiwi.base_url || "" })]),
    h("div", { class: "grid-2" }, [
      h("label", { class: "field" }, [h("span", {}, ["API key"]), h("input", { name: "api_key", type: "password", value: kiwi.api_key || "", placeholder: kiwi.key_set ? "Saved" : "" })]),
      h("label", { class: "field" }, [h("span", {}, ["Space"]), h("input", { name: "space", type: "text", value: kiwi.space || "" })]),
    ]),
    h("label", { class: "field" }, [h("span", {}, ["Path prefix"]), h("input", { name: "path_prefix", type: "text", value: kiwi.path_prefix || "job-search" })]),
    h("p", { class: "help" }, ["Pages go under this prefix so a shared workspace SCHEMA.md and playbook are left alone. Local copies are always written."]),
    h("div", { class: "row" }, [
      h("button", { class: "primary", type: "submit" }, ["Save KiwiFS"]),
      h("button", { class: "ghost", type: "button", onclick: () => syncNow() }, ["Write context now"]),
    ]),
  ]);
  main.replaceChildren(h("div", { class: "page" }, [
    h("h1", {}, ["Context"]),
    h("section", { class: "card" }, [form]),
    h("section", { class: "card" }, [
      h("h2", {}, ["Local folder"]),
      h("p", {}, [ctx.local_dir || ""]),
      h("p", { class: "help" }, [ctx.last_sync && ctx.last_sync.at ? `Last write ${when(ctx.last_sync.at)} · ${ctx.last_sync.files || 0} files · KiwiFS ${ctx.last_sync.kiwi || "disabled"}` : "Nothing written yet. A search writes this folder when it finishes."]),
      h("h3", { id: "context-file" }, ["index.md"]),
      h("div", { id: "context-doc", class: "md" }, ["Loading…"]),
    ]),
  ]));
  loadContextDoc("index.md");
}

async function loadContextDoc(path) {
  let ctx;
  try {
    ctx = await api("/api/context?path=" + encodeURIComponent(path));
  } catch (err) {
    showNotice(err.message, true);
    return;
  }
  const host = document.querySelector("#context-doc");
  const title = document.querySelector("#context-file");
  if (!host || state.view !== "context") return;
  const current = ctx.path || path;
  if (title) title.textContent = current;
  host.replaceChildren();
  if (current !== "index.md") {
    host.append(h("p", { class: "md-nav" }, [
      h("button", { type: "button", class: "linkish", onclick: () => loadContextDoc("index.md") }, ["Index"]),
    ]));
  }
  if (!ctx.preview) {
    host.append(h("p", {}, ["This page has not been written yet."]));
    return;
  }
  host.append(renderMarkdown(ctx.preview, (href) => {
    loadContextDoc(resolveContextPath(current, href));
  }));
}

function resolveContextPath(current, href) {
  const base = current.includes("/") ? current.slice(0, current.lastIndexOf("/") + 1) : "";
  const parts = (base + href).split("/");
  const out = [];
  for (const part of parts) {
    if (part === "" || part === ".") continue;
    if (part === "..") out.pop();
    else out.push(part);
  }
  return out.join("/");
}

function metaValue(raw) {
  let value = raw.trim();
  if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
    value = value.slice(1, -1);
  }
  return value;
}

function renderMarkdown(text, onLink) {
  const root = h("div", { class: "md-body" });
  let body = String(text || "").replace(/\r\n/g, "\n");
  const front = body.match(/^---\n([\s\S]*?)\n---\n*/);
  if (front) {
    body = body.slice(front[0].length);
    const meta = h("dl");
    for (const line of front[1].split("\n")) {
      const cut = line.indexOf(":");
      if (cut < 1) continue;
      meta.append(h("div", {}, [
        h("dt", {}, [line.slice(0, cut).trim()]),
        h("dd", {}, [metaValue(line.slice(cut + 1))]),
      ]));
    }
    if (meta.childNodes.length) root.append(h("details", { class: "md-meta" }, [h("summary", {}, ["Page details"]), meta]));
  }
  const lines = body.split("\n");
  let list = null;
  const endList = () => { list = null; };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (!line.trim()) {
      endList();
      continue;
    }
    const heading = /^(#{1,3})\s+(.*)$/.exec(line);
    if (heading) {
      endList();
      const level = String(Math.min(heading[1].length + 1, 4));
      const node = h("h" + level);
      appendInline(node, heading[2], onLink);
      root.append(node);
      continue;
    }
    const item = /^[-*+]\s+(.*)$/.exec(line);
    if (item) {
      if (!list) {
        list = h("ul");
        root.append(list);
      }
      const li = h("li");
      appendInline(li, item[1], onLink);
      list.append(li);
      continue;
    }
    endList();
    const chunk = [line];
    while (i + 1 < lines.length && lines[i + 1].trim() && !/^(#{1,3})\s+/.test(lines[i + 1]) && !/^[-*+]\s+/.test(lines[i + 1])) {
      i++;
      chunk.push(lines[i]);
    }
    const para = h("p");
    appendInline(para, chunk.join(" "), onLink);
    root.append(para);
  }
  return root;
}

function appendInline(parent, text, onLink) {
  const re = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\[[^\]]+\]\([^)\s]+\))/g;
  let last = 0;
  for (const match of text.matchAll(re)) {
    if (match.index > last) parent.append(document.createTextNode(text.slice(last, match.index)));
    const token = match[0];
    if (token.startsWith("`")) parent.append(h("code", {}, [token.slice(1, -1)]));
    else if (token.startsWith("**")) parent.append(h("strong", {}, [token.slice(2, -2)]));
    else {
      const link = /^\[([^\]]+)\]\(([^)\s]+)\)$/.exec(token);
      const label = link[1];
      const href = link[2];
      if (/^https?:\/\//i.test(href)) {
        parent.append(h("a", { href, target: "_blank", rel: "noreferrer" }, [label]));
      } else if (onLink && href.endsWith(".md")) {
        parent.append(h("button", { type: "button", class: "linkish", onclick: () => onLink(href) }, [label]));
      } else parent.append(document.createTextNode(label));
    }
    last = match.index + token.length;
  }
  if (last < text.length) parent.append(document.createTextNode(text.slice(last)));
}

function saveKiwi(event, settings) {
  event.preventDefault();
  const form = event.currentTarget;
  const next = structuredClone(settings);
  next.kiwi = {
    enabled: form.elements.enabled.checked,
    base_url: form.elements.base_url.value.trim(),
    api_key: form.elements.api_key.value,
    space: form.elements.space.value.trim(),
    path_prefix: form.elements.path_prefix.value.trim(),
  };
  api("/api/settings", { method: "PUT", body: next }).then(() => showNotice("Context settings saved.")).catch((err) => showNotice(err.message, true));
}

async function syncNow() {
  try {
    const status = await api("/api/context/sync", { method: "POST", body: {} });
    showNotice(`Wrote ${status.files} files. KiwiFS: ${status.kiwi}.`, true);
    renderContext();
  } catch (err) {
    showNotice(err.message, true);
  }
}

function paySide(job) {
  const pay = compensation(job);
  const name = job.source_name || job.source_kind || "Source";
  return h("div", { class: pay.known ? "pay-side" : "pay-side unknown" }, [
    h("div", { class: "source-mark" }, [sourceLogo(job), h("span", { class: "source-name" }, [name])]),
    h("span", { class: "label" }, ["Compensation"]),
    h("strong", {}, [pay.text]),
  ]);
}

async function renderModel() {
  const settings = await api("/api/settings");
  if (state.view !== "model") return;
  state.settings = settings;
  const llm = settings.llm || {};
  const form = h("form", { id: "model-form", class: "stack", onsubmit: (event) => saveModel(event, settings) }, [
    h("label", { class: "field" }, [h("span", {}, ["Provider"]), h("select", {
      name: "provider",
      onchange: (event) => {
        const next = event.target.value;
        const url = event.target.form.elements.base_url;
        const known = new Set(Object.values(providerDefaults).filter(Boolean));
        if (!url.value.trim() || known.has(url.value.trim())) url.value = providerDefaults[next] || "";
        loadModels();
      },
    }, [
      ["ollama", "Ollama (local)"],
      ["anthropic", "Claude"],
      ["openai", "OpenAI"],
      ["cursor", "Cursor-compatible gateway"],
      ["compatible", "Other OpenAI-compatible endpoint"],
    ].map(([value, label]) => h("option", { value, selected: llm.provider === value ? "selected" : null }, [label])))]),
    h("label", { class: "field" }, [h("span", {}, ["Base URL"]), h("input", { name: "base_url", type: "text", value: llm.base_url || "" })]),
    h("div", { class: "model-row" }, [
      h("label", { class: "field" }, [h("span", {}, ["Model"]), h("select", { name: "model", id: "model-select" }, [
        h("option", { value: llm.model || "" }, [llm.model || "Loading models…"]),
      ])]),
      h("button", { class: "ghost", type: "button", onclick: () => loadModels() }, ["Load models"]),
      h("p", { class: "help", id: "model-status" }, ["Loading models from the provider."]),
    ]),
    h("label", { class: "field" }, [h("span", {}, ["API key"]), h("input", { name: "api_key", type: "password", value: llm.api_key || "", placeholder: llm.key_set ? "Saved" : "Not required for Ollama" })]),
    h("p", { class: "help" }, ["Cursor does not publish a general chat API. Point the Cursor provider at an OpenAI-compatible gateway and supply that gateway's key. Ollama uses /v1/chat/completions on the base URL."]),
    h("label", { class: "check" }, [h("input", { type: "checkbox", name: "score_with_llm", checked: settings.score_with_llm ? "checked" : null }), "Score matches with the model"]),
    h("label", { class: "check" }, [h("input", { type: "checkbox", name: "expand_sources", checked: settings.expand_sources ? "checked" : null }), "Let the model add a public source when fewer than 8 roles pass the rules"]),
    h("label", { class: "check" }, [h("input", { type: "checkbox", name: "auto_research", checked: settings.auto_research ? "checked" : null }), "Brief the top matched roles during a run"]),
    h("div", { class: "grid-2" }, [
      num("Roles to score per run", "max_llm_jobs", settings.max_llm_jobs || 25),
      num("Cache hours (0 refetches every run)", "cache_hours", settings.cache_hours ?? 6),
    ]),
    h("button", { class: "primary", type: "submit" }, ["Save model"]),
  ]);
  main.replaceChildren(h("div", { class: "page" }, [h("h1", {}, ["Model"]), h("section", { class: "card" }, [form])]));
  loadModels();
}

async function loadModels() {
  const ticket = ++modelLoad;
  const form = document.querySelector("#model-form");
  if (!form || state.view !== "model") return;
  const status = document.querySelector("#model-status");
  const select = form.elements.model;
  const current = select.value;
  if (status) status.textContent = "Loading models…";
  try {
    const data = await api("/api/models", {
      method: "POST",
      body: {
        provider: form.elements.provider.value,
        base_url: form.elements.base_url.value.trim(),
        api_key: form.elements.api_key.value,
      },
    });
    if (ticket !== modelLoad || state.view !== "model") return;
    const ids = data.models || [];
    fillModelSelect(select, ids, current);
    if (status) status.textContent = ids.length === 1 ? "1 model from the provider." : `${ids.length} models from the provider.`;
  } catch (err) {
    if (ticket !== modelLoad || state.view !== "model") return;
    fillModelSelect(select, [], current);
    if (status) status.textContent = err.message;
  }
}

function fillModelSelect(select, ids, selected) {
  const names = ids.slice();
  if (selected && !names.includes(selected)) names.unshift(selected);
  const options = names.length ? names : [selected || ""];
  select.replaceChildren(...options.map((id) => h("option", { value: id }, [id || "No models loaded"])));
  if (selected && [...select.options].some((option) => option.value === selected)) select.value = selected;
}

function saveModel(event, settings) {
  event.preventDefault();
  const form = event.currentTarget;
  const next = structuredClone(settings);
  next.llm = {
    provider: form.elements.provider.value,
    base_url: form.elements.base_url.value.trim(),
    model: form.elements.model.value.trim(),
    api_key: form.elements.api_key.value,
  };
  next.score_with_llm = form.elements.score_with_llm.checked;
  next.expand_sources = form.elements.expand_sources.checked;
  next.auto_research = form.elements.auto_research.checked;
  next.max_llm_jobs = Number(form.elements.max_llm_jobs.value || 25);
  next.cache_hours = Number(form.elements.cache_hours.value || 0);
  api("/api/settings", { method: "PUT", body: next }).then((saved) => {
    state.settings = saved;
    const llm = saved.llm || {};
    showNotice(`Saved ${llm.provider || "model"} · ${llm.base_url || "default URL"} · ${llm.model || "default model"}.`);
    renderModel();
  }).catch((err) => showNotice(err.message, true));
}

show("jobs");
