#!/usr/bin/env node
"use strict";

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..", "..");
const pkg = require(path.join(root, "package.json"));
const binaryName = process.platform === "win32" ? "oci-identity-apps.exe" : "oci-identity-apps";
const outputPath = path.join(root, "npm", "bin", binaryName);
const commit = process.env.GITHUB_SHA || process.env.npm_package_gitHead || "npm";
const date = process.env.SOURCE_DATE_EPOCH
  ? new Date(Number(process.env.SOURCE_DATE_EPOCH) * 1000).toISOString()
  : "npm";

fs.mkdirSync(path.dirname(outputPath), { recursive: true });

const ldflags = [
  "-s",
  "-w",
  `-X github.com/adrianmross/oci-identity-apps/internal/cli.version=${pkg.version}`,
  `-X github.com/adrianmross/oci-identity-apps/internal/cli.commit=${commit}`,
  `-X github.com/adrianmross/oci-identity-apps/internal/cli.date=${date}`
].join(" ");

const result = spawnSync("go", ["build", "-ldflags", ldflags, "-o", outputPath, "./cmd/oci-identity-apps"], {
  cwd: root,
  stdio: "inherit"
});

if (result.error) {
  if (result.error.code === "ENOENT") {
    console.error("Go is required to install @adrianmross/oci-identity-apps from npm.");
    console.error("Install Go 1.25.6 or newer, then rerun `npm rebuild @adrianmross/oci-identity-apps`.");
  } else {
    console.error(result.error.message);
  }
  process.exit(1);
}

process.exit(result.status === null ? 1 : result.status);
