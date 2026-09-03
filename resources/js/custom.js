/* This project's client behaviours, and the only script here that is.
 *
 * ui.js is the framework's: it binds the components once on `document` and an
 * upgrade replaces it. This file is yours, nothing upstream rewrites it, and it
 * is served from this origin like every other asset -- no CDN, no bundler, no
 * minifier, so what is written here is what the browser runs.
 *
 * It loads after ui.js, which is what puts `arandu.ui` on the page. Both are
 * deferred, so the order the layout lists them in is the order they run in.
 *
 * # The two registries
 *
 * Both take a NAME. Markup carries the name and never the code, which is what
 * keeps the policy at script-src 'self' with no unsafe-eval: nothing here reads
 * an attribute and evaluates it.
 *
 *     arandu.ui.action('archive-message', function (event, element) {
 *         // element is the closest ancestor carrying the attribute that named
 *         // this action, which is rarely the one the event came from.
 *     });
 *
 *     arandu.ui.define('message-list', {
 *         mounted:   function (ctx) { },
 *         updated:   function (ctx) { },
 *         destroyed: function (ctx) { },
 *     });
 *
 * The markup names them and carries nothing else:
 *
 *     <div data-kyse-behavior="message-list" data-kyse-props='{"open":true}'>
 *       <button data-kyse-on-click="archive-message">Archive</button>
 *     </div>
 *
 * An action is delegated: click, change and submit reach it wherever the
 * element is, including markup that arrived in a swap, and there is nothing to
 * bind or unbind. A behaviour is the case that has a lifecycle -- `ctx.element`
 * is the element, `ctx.props` is the JSON the server wrote beside it, and the
 * same object reaches all three hooks, so a timer or an observer taken in
 * mounted is released in destroyed. Register at any time: a name defined after
 * the page parsed mounts what is already on it.
 *
 * A name nothing registered does nothing and says so once in the console.
 *
 * # Why this comment stays
 *
 * It is also what keeps the file from being empty, and an empty asset is
 * refused when the application starts rather than served: a zero-byte script is
 * a <script> that loads, runs nothing, and looks exactly like a behaviour
 * somebody forgot to register.
 */
