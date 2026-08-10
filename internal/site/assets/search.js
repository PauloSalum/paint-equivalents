(function () {
  var box = document.getElementById("q");
  if (!box) return;
  var out = document.getElementById("results");
  var index = null, pending = false;

  function load() {
    if (index || pending) return;
    pending = true;
    fetch("/search.json")
      .then(function (r) { return r.json(); })
      .then(function (d) { index = d; pending = false; run(); })
      .catch(function () { pending = false; });
  }

  function norm(s) {
    return s.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "");
  }

  function run() {
    var q = norm(box.value.trim());
    if (q.length < 2 || !index) { out.hidden = true; out.innerHTML = ""; return; }
    var hits = [];
    for (var i = 0; i < index.length && hits.length < 40; i++) {
      var p = index[i];
      var at = norm(p.n).indexOf(q);
      if (at < 0) at = norm(p.b).indexOf(q) === 0 ? 0 : -1;
      if (at >= 0) hits.push({ p: p, at: at });
    }
    hits.sort(function (a, b) { return a.at - b.at || a.p.n.length - b.p.n.length; });
    out.innerHTML = hits.slice(0, 12).map(function (h) {
      return '<li><a href="' + h.p.u + '">' +
        '<span class="sw" style="background:' + h.p.h + ';color:' + h.p.i + '"></span>' +
        '<span>' + esc(h.p.n) + '</span>' +
        '<span class="rb">' + esc(h.p.b) + "</span></a></li>";
    }).join("");
    out.hidden = hits.length === 0;
  }

  function esc(s) {
    return s.replace(/[&<>"]/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c];
    });
  }

  box.addEventListener("focus", load);
  box.addEventListener("input", function () { load(); run(); });
  document.addEventListener("click", function (e) {
    if (!out.contains(e.target) && e.target !== box) out.hidden = true;
  });
  box.addEventListener("keydown", function (e) {
    if (e.key === "Escape") { out.hidden = true; return; }
    if (e.key !== "ArrowDown") return;
    var first = out.querySelector("a");
    if (first) { e.preventDefault(); first.focus(); }
  });
})();
