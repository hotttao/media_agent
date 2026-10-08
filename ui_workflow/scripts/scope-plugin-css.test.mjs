import { test } from "node:test";
import assert from "node:assert/strict";
import postcss from "postcss";
import { scopePluginCss } from "./scope-plugin-css.mjs";

test("isolates resets, classes, media rules and pseudoelements without breaking keyframes", async () => {
  const result = await postcss([scopePluginCss()]).process(
    ":root { color:red } body { margin:0 } *, ::before { box-sizing:border-box } .button:hover { color:red } @media (min-width:1px) { .flex { display:flex } } @keyframes spin { to { transform:rotate(1turn) } } .media-workflow-ui { color:blue }",
    { from: undefined },
  );
  assert.match(result.css, /:where\(\.media-workflow-ui\) \.button:hover/);
  assert.match(result.css, /:where\(\.media-workflow-ui\) ::before/);
  assert.match(result.css, /:where\(\.media-workflow-ui\) \.flex/);
  assert.doesNotMatch(result.css, /:root|body\s*\{/);
  assert.match(result.css, /@keyframes media-workflow-spin \{ to \{/);
  assert.doesNotMatch(result.css, /:where\(\.media-workflow-ui\) to/);
});
