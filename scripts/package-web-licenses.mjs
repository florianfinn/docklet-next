import { createRequire } from "node:module";
import { readFileSync, readdirSync, existsSync, mkdirSync, copyFileSync, writeFileSync, realpathSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { createHash } from "node:crypto";

const destination = process.argv[2];
if (!destination) throw new Error("expected licence destination");
const allowed = new Set(["MIT", "Apache-2.0", "ISC", "BSD-2-Clause", "BSD-3-Clause", "OFL-1.1", "CC0-1.0", "Unlicense", "0BSD"]);
const pending = [{ root: resolve("web"), names: Object.keys(JSON.parse(readFileSync("web/package.json")).dependencies) }];
const records = new Map();
const missing=[];
while (pending.length) {
  const { root, names } = pending.pop();
  for (const name of names) {
    const require = createRequire(join(root, "package.json"));
    const parent = require.resolve.paths(name)?.find(path => existsSync(join(path,name,"package.json")));
    if (!parent) throw new Error(`web dependency unavailable: ${name}`);
    const directory=realpathSync(join(parent,name));
    const meta = JSON.parse(readFileSync(join(directory, "package.json")));
    const key = `${meta.name}@${meta.version}`;
    if (records.has(key)) continue;
    if (!meta.license?.split(" AND ").every(license=>allowed.has(license))) throw new Error(`unassessed web licence: ${key}`);
    function legalFiles(path,prefix=""){
      return readdirSync(path,{withFileTypes:true}).flatMap(entry=>entry.isDirectory() && entry.name!=="node_modules"
        ? legalFiles(join(path,entry.name),prefix+entry.name+"/")
        : entry.isFile() && /^(?:licen[sc]e|copying|notice)(?:[._-]|$)/i.test(entry.name)?[prefix+entry.name]:[]);
    }
    const files=legalFiles(directory);
    const reviewed=({"@schummar/icu-type-parser@1.21.5": {"path": "LICENSES/MIT-icu-type-parser.txt", "sha256": "872f31f366abf7225ef12fc28320977c6287ed01f6569f40070a48ac890186dd", "source": "https://github.com/schummar/schummar-translate/blob/de1cde15a76f4f60874dbe9b7c5395ae2d38b610/LICENSE"}, "victory-vendor@37.3.6": {"path": "LICENSES/MIT-victory-vendor.txt", "sha256": "34dee5fd5e5cc756efb5ad4d64652bce719bdf5e900ca5d7538253e737930bc4", "source": "https://github.com/FormidableLabs/victory/blob/d9d9ca2d5038d6ef9de91f2cef39e6fb2733baa6/LICENSE.txt"}, "react-remove-scroll-bar@2.3.8": {"path": "LICENSES/MIT-react-remove-scroll-bar.txt", "sha256": "a79aae0c0f21990d9d963bb3c5a79cdcea9a46f8523ba55c58d7fe776b6ebc84", "source": "https://github.com/theKashey/react-remove-scroll-bar/blob/7301c160fda44cb8cf2b9fdfde61efad35736196/LICENSE"}})[key];
    if (!files.length && !reviewed) missing.push(`${key}: ${JSON.stringify(meta.repository??null)}`);
    const evidence = [];
    for (const path of files) {
      const source = join(directory, path), target = join(destination, key, path);
      mkdirSync(dirname(target), { recursive: true }); copyFileSync(source, target);
      evidence.push({ path, sha256: createHash("sha256").update(readFileSync(source)).digest("hex") });
    }
    if (reviewed) {
      const data=readFileSync(reviewed.path);
      if(createHash("sha256").update(data).digest("hex") !== reviewed.sha256) throw new Error("reviewed web licence drift");
      const path="LICENSE";const target=join(destination,key,path);mkdirSync(dirname(target),{recursive:true});copyFileSync(reviewed.path,target);
      evidence.push({path,sha256:reviewed.sha256,source:reviewed.source});
    }
    records.set(key, { name: meta.name, version: meta.version, license: meta.license, evidence });
    pending.push({ root: directory, names: Object.keys(meta.dependencies ?? {}) });
  }
}
if(missing.length)throw new Error(`missing web licence texts: ${missing.join("; ")}`);
mkdirSync(destination, { recursive: true });
writeFileSync(join(destination, "index.json"), JSON.stringify([...records.values()].sort((a,b) => a.name.localeCompare(b.name)), null, 2) + "\n");
console.log(`Packaged web licences: ${records.size}`);
