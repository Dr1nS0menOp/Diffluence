// SPDX-License-Identifier: Apache-2.0
// Search of a public site: a plain form, a small index, and results in a live region.
// Without script the site still has its index page with all pages.
(function () {
  'use strict';
  var input = document.getElementById('q');
  var results = document.getElementById('results');
  if (!input || !results) {
    return;
  }
  var maxResults = 20;
  var entries = null;

  function load() {
    if (entries) {
      return Promise.resolve(entries);
    }
    return fetch('search.json')
      .then(function (r) {
        return r.json();
      })
      .then(function (list) {
        entries = list;
        return list;
      });
  }

  function show(list, words) {
    results.textContent = '';
    if (words.length === 0) {
      return;
    }
    var hits = list.filter(function (e) {
      var text = (e.t + ' ' + e.x).toLowerCase();
      return words.every(function (w) {
        return text.indexOf(w) !== -1;
      });
    });
    var line = document.createElement('p');
    line.textContent = hits.length + (hits.length === 1 ? ' page found' : ' pages found');
    results.appendChild(line);
    var ul = document.createElement('ul');
    hits.slice(0, maxResults).forEach(function (e) {
      var li = document.createElement('li');
      var a = document.createElement('a');
      a.href = e.u;
      a.textContent = e.t;
      li.appendChild(a);
      ul.appendChild(li);
    });
    results.appendChild(ul);
  }

  input.addEventListener('input', function () {
    var words = input.value.toLowerCase().split(/\s+/).filter(Boolean);
    load().then(function (list) {
      show(list, words);
    });
  });
  input.form.addEventListener('submit', function (event) {
    event.preventDefault();
  });
})();
