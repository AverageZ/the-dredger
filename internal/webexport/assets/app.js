(function () {
  "use strict";

  var COLORS = {
    bookmarks: "#7D56F4",
    complete: "#04B575",
    deleted: "#FF4040",
    serendipity: "#FFB347",
  };

  var data;
  try {
    data = JSON.parse(document.getElementById("dredger-data").textContent);
  } catch (e) {
    document.getElementById("views").textContent = "Failed to read snapshot data.";
    return;
  }

  // ---- tiny DOM helpers ---------------------------------------------------
  function el(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }
  function clear(node) { while (node.firstChild) node.removeChild(node.firstChild); }
  function view(name) { return document.querySelector('.view[data-view="' + name + '"]'); }

  // ---- charts -------------------------------------------------------------
  // Horizontal stacked bar from [{label, count, color}].
  function stackedBar(segments) {
    var total = segments.reduce(function (s, x) { return s + x.count; }, 0);
    var svgNS = "http://www.w3.org/2000/svg";
    var svg = document.createElementNS(svgNS, "svg");
    svg.setAttribute("class", "stackbar");
    svg.setAttribute("viewBox", "0 0 100 10");
    svg.setAttribute("preserveAspectRatio", "none");
    var x = 0;
    segments.forEach(function (seg) {
      var w = total > 0 ? (seg.count / total) * 100 : 0;
      if (w <= 0) return;
      var rect = document.createElementNS(svgNS, "rect");
      rect.setAttribute("x", x);
      rect.setAttribute("y", 0);
      rect.setAttribute("width", w);
      rect.setAttribute("height", 10);
      rect.setAttribute("fill", seg.color);
      var t = document.createElementNS(svgNS, "title");
      t.textContent = seg.label + ": " + seg.count;
      rect.appendChild(t);
      svg.appendChild(rect);
      x += w;
    });
    return svg;
  }

  function legend(segments) {
    var wrap = el("div", "legend");
    segments.forEach(function (seg) {
      var item = el("span");
      var sw = el("span", "swatch");
      sw.style.background = seg.color;
      item.appendChild(sw);
      item.appendChild(document.createTextNode(seg.label + " " + seg.count));
      wrap.appendChild(item);
    });
    return wrap;
  }

  // Ranked list with proportional fill bars from [{label, count}].
  function rankList(items, color) {
    var max = items.reduce(function (m, x) { return Math.max(m, x.count); }, 0);
    var list = el("div", "ranklist");
    items.forEach(function (it) {
      var row = el("div", "rankrow");
      row.appendChild(el("div", "rl-label", it.label));
      var track = el("div", "rl-track");
      var fill = el("div", "rl-fill");
      fill.style.width = (max > 0 ? (it.count / max) * 100 : 0) + "%";
      if (color) fill.style.background = color;
      track.appendChild(fill);
      row.appendChild(track);
      row.appendChild(el("div", "rl-count", String(it.count)));
      list.appendChild(row);
    });
    return list;
  }

  function panel(title, body) {
    var p = el("div", "panel");
    p.appendChild(el("h2", null, title));
    p.appendChild(body);
    return p;
  }

  function emptyMsg(text) { return el("div", "empty", text); }

  // ---- link cards ---------------------------------------------------------
  function card(link) {
    var c = el("div", "card " + link.status);
    var title = el("div", "c-title");
    var a = el("a");
    a.href = link.url;
    a.target = "_blank";
    a.rel = "noopener noreferrer";
    a.textContent = link.title || link.url;
    title.appendChild(a);
    c.appendChild(title);

    c.appendChild(el("div", "c-url", link.url));

    if (link.summary) c.appendChild(el("div", "c-summary", link.summary));
    else if (link.description) c.appendChild(el("div", "c-summary", link.description));

    if (link.tags && link.tags.length) {
      var tags = el("div", "c-tags");
      link.tags.forEach(function (t) { tags.appendChild(el("span", "tag", t)); });
      c.appendChild(tags);
    }

    var meta = el("div");
    var badgeCls = "badge";
    if (link.dredgeState === "Capsized") badgeCls += " capsized";
    else if (link.dredgeState === "Complete") badgeCls += " complete";
    var label = link.status + (link.dredgeState ? " · " + link.dredgeState : "");
    meta.appendChild(el("span", badgeCls, label));
    c.appendChild(meta);
    return c;
  }

  function cardGrid(links) {
    if (!links.length) return emptyMsg("Nothing here yet.");
    var grid = el("div", "cards");
    links.forEach(function (l) { grid.appendChild(card(l)); });
    return grid;
  }

  // ---- views --------------------------------------------------------------
  function renderOverview() {
    var v = view("overview");
    clear(v);

    var s = data.stats;
    var headline = el("div", "headline");
    [
      ["bookmarks", s.bookmarks != null ? s.bookmarks : (s.inbox || 0) + (s.saved || 0), "bookmarks"],
      ["deleted", s.pruned, "deleted"],
      ["total", s.total, ""],
    ].forEach(function (row) {
      var cls = "stat-card" + (row[2] ? " " + row[2] : "");
      var sc = el("div", cls);
      sc.appendChild(el("div", "num", String(row[1])));
      sc.appendChild(el("div", "label", row[0]));
      headline.appendChild(sc);
    });
    v.appendChild(headline);

    var statusSegs = [
      { label: "bookmarks", count: s.bookmarks != null ? s.bookmarks : (s.inbox || 0) + (s.saved || 0), color: COLORS.bookmarks },
      { label: "deleted", count: s.pruned, color: COLORS.deleted },
    ];
    var statusBody = el("div");
    statusBody.appendChild(stackedBar(statusSegs));
    statusBody.appendChild(legend(statusSegs));
    v.appendChild(panel("Bookmark breakdown", statusBody));

    var h = data.enrichmentHealth;
    var healthSegs = [
      { label: "complete", count: h.complete, color: COLORS.complete },
      { label: "pending", count: h.pending, color: COLORS.bookmarks },
      { label: "capsized", count: h.capsized, color: COLORS.deleted },
    ];
    var healthBody = el("div");
    healthBody.appendChild(stackedBar(healthSegs));
    healthBody.appendChild(legend(healthSegs));
    v.appendChild(panel("Enrichment health", healthBody));
  }

  function isBookmark(link) {
    return link.status === "bookmark" || link.status === "library" || link.status === "inbox";
  }

  var bookmarkLinks = data.links.filter(isBookmark);

  function renderBookmarks() {
    var v = view("bookmarks");
    clear(v);

    var toolbar = el("div", "toolbar");
    var search = el("input", "search");
    search.type = "search";
    search.placeholder = "Search title, url, summary, tags…";

    var filter = el("select", "statusfilter");
    [["bookmark", "bookmarks"], ["all", "all"], ["deleted", "deleted"]].forEach(function (o) {
      var present = o[0] === "all" ||
        (o[0] === "bookmark" && data.links.some(isBookmark)) ||
        data.links.some(function (l) { return l.status === o[0] || (o[0] === "deleted" && l.status === "pruned"); });
      if (!present) return;
      var opt = el("option", null, o[1]);
      opt.value = o[0];
      filter.appendChild(opt);
    });

    var note = el("span", "count-note");
    toolbar.appendChild(search);
    toolbar.appendChild(filter);
    toolbar.appendChild(note);
    v.appendChild(toolbar);

    var results = el("div");
    v.appendChild(results);

    function apply() {
      var q = search.value.trim().toLowerCase();
      var status = filter.value;
      var matched = data.links.filter(function (l) {
        if (status === "bookmark" && !isBookmark(l)) return false;
        if (status === "deleted" && l.status !== "deleted" && l.status !== "pruned") return false;
        if (status !== "all" && status !== "bookmark" && status !== "deleted" && l.status !== status) return false;
        if (!q) return true;
        var hay = [l.title, l.url, l.summary, l.description].join(" ").toLowerCase();
        if (hay.indexOf(q) !== -1) return true;
        return (l.tags || []).some(function (t) { return t.toLowerCase().indexOf(q) !== -1; });
      });
      note.textContent = matched.length + " of " + data.links.length + " links";
      clear(results);
      results.appendChild(cardGrid(matched));
    }

    search.addEventListener("input", apply);
    filter.addEventListener("change", apply);
    apply();
  }

  function renderTags() {
    var v = view("tags");
    clear(v);
    if (!data.tags.length) { v.appendChild(emptyMsg("No tags on bookmarks yet.")); return; }
    v.appendChild(panel("Bookmark tags by frequency", rankList(data.tags, COLORS.bookmarks)));
  }

  function renderDomains() {
    var v = view("domains");
    clear(v);
    if (!data.domains.length) { v.appendChild(emptyMsg("No domains to show.")); return; }
    v.appendChild(panel("Domains by frequency", rankList(data.domains, COLORS.serendipity)));
  }

  function shuffle(arr) {
    var a = arr.slice();
    for (var i = a.length - 1; i > 0; i--) {
      var j = Math.floor(Math.random() * (i + 1));
      var tmp = a[i]; a[i] = a[j]; a[j] = tmp;
    }
    return a;
  }

  function renderSerendipity() {
    var v = view("serendipity");
    clear(v);

    var head = el("div", "serendipity-head");
    var mark = el("span", "brand-mark", "✦");
    head.appendChild(mark);
    head.appendChild(el("strong", null, "Serendipity Shuffle"));
    var btn = el("button", "shuffle-btn", "Reshuffle");
    head.appendChild(btn);
    v.appendChild(head);

    var picks = el("div");
    v.appendChild(picks);

    function reshuffle() {
      clear(picks);
      if (!bookmarkLinks.length) { picks.appendChild(emptyMsg("Add some bookmarks to mine for inspiration.")); return; }
      picks.appendChild(cardGrid(shuffle(bookmarkLinks).slice(0, 3)));
    }
    btn.addEventListener("click", reshuffle);
    reshuffle();
  }

  // ---- tab wiring ---------------------------------------------------------
  var rendered = {};
  var renderers = {
    overview: renderOverview,
    bookmarks: renderBookmarks,
    tags: renderTags,
    domains: renderDomains,
    serendipity: renderSerendipity,
  };

  function show(name) {
    document.querySelectorAll(".tab").forEach(function (t) {
      t.classList.toggle("active", t.getAttribute("data-view") === name);
    });
    document.querySelectorAll(".view").forEach(function (vv) {
      vv.classList.toggle("active", vv.getAttribute("data-view") === name);
    });
    if (!rendered[name]) { renderers[name](); rendered[name] = true; }
  }

  document.getElementById("tabs").addEventListener("click", function (e) {
    var t = e.target.closest(".tab");
    if (t) show(t.getAttribute("data-view"));
  });

  show("overview");
})();
