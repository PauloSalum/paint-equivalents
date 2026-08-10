(function () {
  var doc = typeof document === "undefined" ? null : document;
  var hexBox = doc && doc.getElementById("hex");
  var picker = doc && doc.getElementById("picker");
  var out = doc && doc.getElementById("found");
  var head = doc && doc.getElementById("foundhead");
  var index = null, wanted = null;

  function load() {
    if (index) return;
    fetch("/search.json")
      .then(function (r) { return r.json(); })
      .then(function (d) { index = d; if (wanted) rank(wanted); });
  }

  // sRGB to CIELAB under D65, the same conversion the site is built with.
  function lab(r, g, b) {
    function lin(c) {
      c /= 255;
      return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
    }
    r = lin(r); g = lin(g); b = lin(b);
    var x = (0.4124564 * r + 0.3575761 * g + 0.1804375 * b) / 0.95047;
    var y = (0.2126729 * r + 0.7151522 * g + 0.0721750 * b);
    var z = (0.0193339 * r + 0.1191920 * g + 0.9503041 * b) / 1.08883;
    function f(t) {
      return t > 0.008856451679035631 ? Math.cbrt(t) : t * 7.787037037037035 + 16 / 116;
    }
    x = f(x); y = f(y); z = f(z);
    return [116 * y - 16, 500 * (x - y), 200 * (y - z)];
  }

  var RAD = Math.PI / 180, POW25_7 = 6103515625;

  // CIEDE2000, ported from internal/color/color.go. The hue-band selection and
  // the rotation term are where naive implementations go wrong, so this mirrors
  // the Go version case for case rather than being rewritten more compactly.
  function de2000(a, b) {
    var l1 = a[0], a1 = a[1], b1 = a[2];
    var l2 = b[0], a2 = b[1], b2 = b[2];
    var c1 = Math.hypot(a1, b1), c2 = Math.hypot(a2, b2);
    var cBar = (c1 + c2) / 2;
    var cBar7 = Math.pow(cBar, 7);
    var gp = 0.5 * (1 - Math.sqrt(cBar7 / (cBar7 + POW25_7)));
    var a1p = (1 + gp) * a1, a2p = (1 + gp) * a2;
    var c1p = Math.hypot(a1p, b1), c2p = Math.hypot(a2p, b2);

    function hue(bb, ap) {
      if (ap === 0 && bb === 0) return 0;
      var h = Math.atan2(bb, ap) / RAD;
      return h < 0 ? h + 360 : h;
    }
    var h1p = hue(b1, a1p), h2p = hue(b2, a2p);

    var dLp = l2 - l1, dCp = c2p - c1p, dhp;
    if (c1p * c2p === 0) dhp = 0;
    else if (Math.abs(h2p - h1p) <= 180) dhp = h2p - h1p;
    else if (h2p - h1p > 180) dhp = h2p - h1p - 360;
    else dhp = h2p - h1p + 360;
    var dHp = 2 * Math.sqrt(c1p * c2p) * Math.sin((dhp / 2) * RAD);

    var lBarP = (l1 + l2) / 2, cBarP = (c1p + c2p) / 2, hBarP;
    if (c1p * c2p === 0) hBarP = h1p + h2p;
    else if (Math.abs(h1p - h2p) <= 180) hBarP = (h1p + h2p) / 2;
    else if (h1p + h2p < 360) hBarP = (h1p + h2p + 360) / 2;
    else hBarP = (h1p + h2p - 360) / 2;

    var t = 1 - 0.17 * Math.cos((hBarP - 30) * RAD)
              + 0.24 * Math.cos((2 * hBarP) * RAD)
              + 0.32 * Math.cos((3 * hBarP + 6) * RAD)
              - 0.20 * Math.cos((4 * hBarP - 63) * RAD);
    var dTheta = 30 * Math.exp(-Math.pow((hBarP - 275) / 25, 2));
    var cBarP7 = Math.pow(cBarP, 7);
    var rt = -Math.sin((2 * dTheta) * RAD) * 2 * Math.sqrt(cBarP7 / (cBarP7 + POW25_7));

    var sl = 1 + (0.015 * Math.pow(lBarP - 50, 2)) / Math.sqrt(20 + Math.pow(lBarP - 50, 2));
    var sc = 1 + 0.045 * cBarP;
    var sh = 1 + 0.015 * cBarP * t;
    var tl = dLp / sl, tc = dCp / sc, th = dHp / sh;
    return Math.sqrt(tl * tl + tc * tc + th * th + rt * tc * th);
  }

  function grade(de) {
    return de < 2 ? "A" : de < 3.5 ? "B" : de < 5 ? "C" : de < 10 ? "D" : "E";
  }
  function quality(de) {
    return de < 1 ? "indistinguishable" : de < 2 ? "near-perfect"
      : de < 3.5 ? "very close" : de < 5 ? "close" : de < 10 ? "similar" : "far";
  }

  function parseHex(s) {
    s = s.trim().replace(/^#/, "");
    if (/^[0-9a-f]{3}$/i.test(s)) s = s[0] + s[0] + s[1] + s[1] + s[2] + s[2];
    if (!/^[0-9a-f]{6}$/i.test(s)) return null;
    return [parseInt(s.slice(0, 2), 16), parseInt(s.slice(2, 4), 16), parseInt(s.slice(4, 6), 16)];
  }

  function ink(r, g, b) {
    function lin(c) {
      c /= 255;
      return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
    }
    var y = 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
    return (y + 0.05) / 0.05 > 1.05 / (y + 0.05) ? "#111111" : "#ffffff";
  }

  function esc(s) {
    return s.replace(/[&<>"]/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c];
    });
  }

  function rank(rgb) {
    wanted = rgb;
    if (!index) { load(); return; }
    var target = lab(rgb[0], rgb[1], rgb[2]);
    var scored = new Array(index.length);
    for (var i = 0; i < index.length; i++) {
      var p = index[i];
      scored[i] = { p: p, de: de2000(target, [p.l, p.a, p.d]) };
    }
    scored.sort(function (x, y) { return x.de - y.de; });

    var hex = "#" + rgb.map(function (v) {
      return ("0" + v.toString(16)).slice(-2);
    }).join("").toUpperCase();
    head.innerHTML = 'Closest paints to <span class="sw" style="background:' + hex +
      ";color:" + ink(rgb[0], rgb[1], rgb[2]) + '">' + hex + "</span>";

    out.innerHTML = scored.slice(0, 40).map(function (h) {
      var g = grade(h.de);
      return '<li><a href="' + h.p.u + '">' +
        '<span class="sw big" style="background:' + h.p.h + ";color:" + h.p.i + '">' + g + "</span>" +
        '<span class="mname">' + esc(h.p.n) + ' <em>' + esc(h.p.b) + "</em></span></a>" +
        '<span class="de ' + g + '">&Delta;E ' + h.de.toFixed(1) + " &middot; " + quality(h.de) + "</span></li>";
    }).join("");
    out.hidden = false;
  }

  function fromText() {
    var rgb = parseHex(hexBox.value);
    if (!rgb) return;
    picker.value = "#" + rgb.map(function (v) {
      return ("0" + v.toString(16)).slice(-2);
    }).join("");
    rank(rgb);
  }

  // Exported so the colour maths can be checked against the Go implementation
  // it was ported from; the browser ignores this.
  if (typeof module !== "undefined") module.exports = { de2000: de2000, lab: lab };
  if (!hexBox) return;

  hexBox.addEventListener("focus", load);
  hexBox.addEventListener("input", fromText);
  picker.addEventListener("input", function () {
    hexBox.value = picker.value.toUpperCase();
    rank(parseHex(picker.value));
  });

  var start = location.hash.replace(/^#/, "");
  if (parseHex(start)) { hexBox.value = "#" + start.replace(/^#/, "").toUpperCase(); fromText(); }
  else { load(); }
})();
