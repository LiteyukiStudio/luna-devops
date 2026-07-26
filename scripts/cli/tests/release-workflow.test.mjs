import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import assert from "node:assert/strict";

const root = resolve(import.meta.dirname, "../../..");
const workflow = readFileSync(
  resolve(root, ".github/workflows/cli-release.yml"),
  "utf8",
);

test("CLI release packages same-version Skills before publishing npm", () => {
  assert.match(workflow, /^\s{2}skills-package:\n/m);
  assert.match(
    workflow,
    /--requires-cli="\$\{\{ needs\.metadata\.outputs\.version \}\}"/,
  );
  assert.match(
    workflow,
    /publish-npm:[\s\S]*?needs:[\s\S]*?-\s+skills-package[\s\S]*?runs-on:/,
  );
});

test("GitHub Release requires and downloads paired Skills", () => {
  assert.match(
    workflow,
    /release:[\s\S]*?needs:[\s\S]*?-\s+skills-package[\s\S]*?runs-on:/,
  );
  assert.match(
    workflow,
    /name:\s+cli-skills-\$\{\{ needs\.metadata\.outputs\.version \}\}/,
  );
  assert.match(workflow, /release\/final\/luna-devops-\*\.skill/);
  assert.doesNotMatch(workflow, /luna-cli-skills-\*\.zip/);
  assert.match(
    workflow,
    /release\/final\/LUNA-CLI-SKILLS-MANIFEST\.json/,
  );
});
