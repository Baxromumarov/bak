#!/usr/bin/env node

import {
  copyFileSync,
  cpSync,
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const extensionDirectory = resolve(scriptDirectory, "..");
const defaultDataDirectory = join(homedir(), ".local", "share", "zed");
const defaultSettingsPath = join(homedir(), ".config", "zed", "settings.json");
const defaultLspDestination = join(homedir(), ".local", "bin", "bak-lsp");

function usage(message) {
  if (message) console.error(`error: ${message}\n`);
  console.error(`Usage: ${basename(process.argv[1])} --lsp-binary PATH [options]

Options:
  --zed-data PATH       Zed data directory (default: ${defaultDataDirectory})
  --settings PATH       Zed settings file (default: ${defaultSettingsPath})
  --lsp-destination PATH
                        Installation path for bak-lsp (default: ${defaultLspDestination})
  --dry-run             Validate inputs and print the intended installation
  --help                Show this message`);
  process.exit(message ? 1 : 0);
}

function parseArguments(arguments_) {
  const options = {
    dataDirectory: defaultDataDirectory,
    settingsPath: defaultSettingsPath,
    lspDestination: defaultLspDestination,
    lspBinary: undefined,
    dryRun: false,
  };
  for (let index = 0; index < arguments_.length; index += 1) {
    const argument = arguments_[index];
    if (argument === "--help") usage();
    if (argument === "--dry-run") {
      options.dryRun = true;
      continue;
    }
    const key = {
      "--zed-data": "dataDirectory",
      "--settings": "settingsPath",
      "--lsp-destination": "lspDestination",
      "--lsp-binary": "lspBinary",
    }[argument];
    if (!key) usage(`unknown option ${argument}`);
    const value = arguments_[index + 1];
    if (!value || value.startsWith("--")) usage(`${argument} requires a path`);
    options[key] = resolve(value);
    index += 1;
  }
  if (!options.lspBinary) usage("--lsp-binary is required");
  return options;
}

// Zed settings use JSONC. This small parser removes comments only outside
// strings, then removes trailing commas, so we can validate the narrow edits
// made below without reformatting or discarding the user's comments.
function parseJsonc(text, sourceName) {
  let output = "";
  let inString = false;
  let escaped = false;
  for (let index = 0; index < text.length; index += 1) {
    const character = text[index];
    const next = text[index + 1];
    if (inString) {
      output += character;
      if (escaped) escaped = false;
      else if (character === "\\") escaped = true;
      else if (character === '"') inString = false;
      continue;
    }
    if (character === '"') {
      inString = true;
      output += character;
      continue;
    }
    if (character === "/" && next === "/") {
      while (index < text.length && text[index] !== "\n") index += 1;
      output += "\n";
      continue;
    }
    if (character === "/" && next === "*") {
      index += 2;
      while (index < text.length && !(text[index] === "*" && text[index + 1] === "/")) index += 1;
      index += 1;
      continue;
    }
    output += character;
  }
  try {
    return JSON.parse(output.replace(/,(\s*[}\]])/g, "$1"));
  } catch (error) {
    throw new Error(`${sourceName} is not valid JSONC after the Bak installation edit: ${error.message}`);
  }
}

function atomicWrite(path, contents) {
  const temporaryPath = `${path}.bak-installing`;
  writeFileSync(temporaryPath, contents);
  renameSync(temporaryPath, path);
}

function insertBakLanguage(settings) {
  if (settings.includes('"Bak"')) return settings;
  const languageEntry = [
    '    "Bak": {',
    '      "format_on_save": "on",',
    '      "formatter": "language_server",',
    '      "language_servers": ["bak-lsp"],',
    '      "semantic_tokens": "combined",',
    '      "tab_size": 4,',
    '      "hard_tabs": false',
    '    },',
    '',
  ].join("\n");
  const languageAnchor = /("languages"\s*:\s*\{\s*\n)/;
  if (!languageAnchor.test(settings)) {
    throw new Error('could not find the "languages" object in Zed settings');
  }
  return settings.replace(languageAnchor, `$1${languageEntry}`);
}

function insertBakLsp(settings, binaryPath) {
  if (/"lsp"\s*:\s*\{[\s\S]*?"bak-lsp"\s*:/.test(settings)) return settings;
  if (/"lsp"\s*:/.test(settings)) {
    throw new Error('an existing "lsp" object was found; add bak-lsp there instead of risking a duplicate settings key');
  }
  const entry = [
    '  "lsp": {',
    '    "bak-lsp": {',
    '      "binary": {',
    `        "path": ${JSON.stringify(binaryPath)}`,
    '      }',
    '    }',
    '  },',
    '',
  ].join("\n");
  const preferredAnchor = /(^\s*"agent_servers"\s*:)/m;
  if (preferredAnchor.test(settings)) return settings.replace(preferredAnchor, `${entry}$1`);
  const closingBrace = settings.lastIndexOf("}");
  if (closingBrace === -1) throw new Error("could not find the end of Zed settings");
  return `${settings.slice(0, closingBrace).replace(/\s*$/, "")},\n${entry}}${settings.slice(closingBrace + 1)}`;
}

function manifest() {
  return {
    id: "bak",
    name: "Bak",
    version: "0.1.0",
    schema_version: 1,
    description: "Language support for Bak: syntax, themes, snippets, formatting, and LSP tooling.",
    repository: "https://github.com/baxromumarov/bak",
    authors: ["Bak contributors"],
    lib: { kind: "Rust", version: "0.7.0" },
    themes: ["themes/bak-go.json"],
    icon_themes: [],
    languages: ["languages/bak"],
    grammars: {
      bak: {
        repository: "https://github.com/baxromumarov/bak",
        rev: "b8c8e1138b1c690cecc18d74279aa3743d1fe30b",
        path: "zed/tree-sitter-bak",
      },
    },
    language_servers: {
      "bak-lsp": {
        language: "Bak",
        languages: [],
        language_ids: {},
        code_action_kinds: null,
      },
    },
    context_servers: {},
    slash_commands: {},
    snippets: "snippets/bak.json",
    capabilities: [],
  };
}

function install(options) {
  if (!existsSync(options.lspBinary) || !statSync(options.lspBinary).isFile()) {
    throw new Error(`Bak LSP binary does not exist: ${options.lspBinary}`);
  }
  if (!existsSync(options.settingsPath)) {
    throw new Error(`Zed settings file does not exist: ${options.settingsPath}`);
  }

  const extensionDestination = join(options.dataDirectory, "extensions", "installed", "bak");
  const indexPath = join(options.dataDirectory, "extensions", "index.json");
  const settingsBefore = readFileSync(options.settingsPath, "utf8");
  const settingsAfter = insertBakLsp(insertBakLanguage(settingsBefore), options.lspDestination);
  parseJsonc(settingsAfter, options.settingsPath);

  const indexBefore = existsSync(indexPath)
    ? JSON.parse(readFileSync(indexPath, "utf8"))
    : { extensions: {} };
  indexBefore.extensions ??= {};
  indexBefore.extensions.bak = { manifest: manifest(), dev: true };

  if (options.dryRun) {
    console.log(`Would install Bak extension to ${extensionDestination}`);
    console.log(`Would install bak-lsp to ${options.lspDestination}`);
    console.log(`Would update ${options.settingsPath} and ${indexPath}`);
    return;
  }

  mkdirSync(extensionDestination, { recursive: true });
  cpSync(join(extensionDirectory, "languages"), join(extensionDestination, "languages"), { recursive: true });
  cpSync(join(extensionDirectory, "themes"), join(extensionDestination, "themes"), { recursive: true });
  cpSync(join(extensionDirectory, "snippets"), join(extensionDestination, "snippets"), { recursive: true });
  copyFileSync(join(extensionDirectory, "extension.toml"), join(extensionDestination, "extension.toml"));
  copyFileSync(join(extensionDirectory, "build", "extension.wasm"), join(extensionDestination, "extension.wasm"));
  mkdirSync(join(extensionDestination, "grammars"), { recursive: true });
  copyFileSync(join(extensionDirectory, "build", "bak.wasm"), join(extensionDestination, "grammars", "bak.wasm"));

  mkdirSync(dirname(options.lspDestination), { recursive: true });
  const lspNeedsCopy = !existsSync(options.lspDestination)
    || !readFileSync(options.lspBinary).equals(readFileSync(options.lspDestination));
  if (lspNeedsCopy) copyFileSync(options.lspBinary, options.lspDestination);
  // Zed launches this binary directly.
  chmodSync(options.lspDestination, 0o755);

  if (!existsSync(`${options.settingsPath}.bak`)) {
    copyFileSync(options.settingsPath, `${options.settingsPath}.bak`);
  }
  atomicWrite(options.settingsPath, settingsAfter);
  if (existsSync(indexPath) && !existsSync(`${indexPath}.bak`)) {
    copyFileSync(indexPath, `${indexPath}.bak`);
  }
  mkdirSync(dirname(indexPath), { recursive: true });
  atomicWrite(indexPath, `${JSON.stringify(indexBefore, null, 2)}\n`);

  console.log(`Installed Bak support for Zed at ${extensionDestination}`);
  console.log("Restart Zed to load Bak syntax, themes, snippets, and bak-lsp.");
}

try {
  install(parseArguments(process.argv.slice(2)));
} catch (error) {
  console.error(`Bak Zed installation failed: ${error.message}`);
  process.exit(1);
}
