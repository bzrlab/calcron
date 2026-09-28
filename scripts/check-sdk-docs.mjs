import { readFile } from "node:fs/promises";

const guide = await readFile("docs/sdk.md", "utf8");
const nodePackage = JSON.parse(await readFile("sdk/node/package.json", "utf8"));
const required = ["npm install @bzrlab/calcron", "Node and Go SDKs", "`set`", "`cancel`", "`extend`", "`throttle`", "`start`", "`signal`", "at least once", "idempotency", "API reference", "Typed event contracts", "EventOf", "exactly one of", "Node / TypeScript example", "Go example"];
if (!required.every(text => guide.includes(text))) throw new Error("SDK guide is incomplete");
if (nodePackage.private || !nodePackage.exports?.["."] || !nodePackage.types || !nodePackage.engines?.node) throw new Error("Node package is not publishable");
console.log("sdk documentation verified");
