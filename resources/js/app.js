// The JavaScript of this application, written by hand and served as it is.
//
// RULE 13 forbids Node in the build, not the .js extension. There is no
// package.json, no npm install and no transpilation, so everything here has to
// be what a browser executes directly: no import that needs resolving, no JSX,
// no TypeScript.
//
// HTMX and Alpine arrive embedded in the binary and are already loaded by the
// layout. This file only configures them.
//
// The limit is about content rather than form. Alpine holds client state that is
// ephemeral and invisible to the server -- an open menu, a focused tab. A
// business rule in x-data is a badly written HTMX fragment, and a fetch, a
// global store or a validation here is what `aru doctor` refuses.

(function () {
  "use strict";

  // Send the CSRF token with every HTMX request that changes state.
  //
  // The token is rendered into the page by @csrf. Reading it once and attaching
  // it here means no form has to remember, and a missing token becomes a
  // deployment problem instead of a per-page bug.
  document.body.addEventListener("htmx:configRequest", function (event) {
    var field = document.querySelector('input[name="_csrf"]');
    if (field) {
      event.detail.headers["X-CSRF-Token"] = field.value;
    }
  });

  // An HTMX request that fails leaves the page as it was, which looks like
  // nothing happened. Saying so on the console is the difference between a bug
  // report and a shrug.
  document.body.addEventListener("htmx:responseError", function (event) {
    console.error("htmx: " + event.detail.xhr.status + " from " + event.detail.pathInfo.requestPath);
  });
})();
