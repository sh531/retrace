// load runs the page's classic scripts in this test's global scope, as a
// browser runs them in the page's, and returns that scope, so their
// functions can be called as properties of it. Each test file runs in its
// own process, so the scripts' globals don't leak between files.
import fs from "node:fs";
import vm from "node:vm";

const assets = new URL("../web/assets/", import.meta.url);

export function load(...names) {
  for (const name of names) {
    vm.runInThisContext(fs.readFileSync(new URL(name, assets), "utf8"), { filename: name });
  }
  return globalThis;
}
