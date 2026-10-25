import * as vscode from "vscode";
import * as fs from "fs";
import * as path from "path";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from "vscode-languageclient/node";

let client: LanguageClient | null = null;
let outputChannel: vscode.OutputChannel | null = null;

function lspExecutableName(): string {
  return process.platform === "win32" ? "bak-lsp.exe" : "bak-lsp";
}

function existingExecutable(candidate: string): string | undefined {
  if (!candidate || !fs.existsSync(candidate)) {
    return undefined;
  }
  try {
    const stat = fs.statSync(candidate);
    if (!stat.isFile()) {
      return undefined;
    }
    fs.accessSync(candidate, fs.constants.X_OK);
    return candidate;
  } catch {
    return undefined;
  }
}

function resolveFromPath(command: string): string | undefined {
  const pathEnv = process.env.PATH || "";
  const pathExt = process.platform === "win32"
    ? (process.env.PATHEXT || ".EXE;.CMD;.BAT;.COM").split(";")
    : [""];

  for (const dir of pathEnv.split(path.delimiter)) {
    if (!dir) {
      continue;
    }
    for (const ext of pathExt) {
      const candidate = path.join(dir, command.endsWith(ext.toLowerCase()) || command.endsWith(ext) ? command : command + ext);
      const found = existingExecutable(candidate);
      if (found) {
        return found;
      }
    }
  }

  return undefined;
}

function workspaceRoots(): string[] {
  const folders = vscode.workspace.workspaceFolders || [];
  if (folders.length > 0) {
    return folders.map((folder) => folder.uri.fsPath);
  }
  return [process.cwd()];
}

function workspaceCwd(): string {
  return workspaceRoots()[0] || process.cwd();
}

function bundledServerPath(context: vscode.ExtensionContext): string | undefined {
  const target = `${process.platform}-${process.arch}`;
  const executable = lspExecutableName();

  // Release packages contain only the LSP for their target platform. Keep the
  // legacy location as a fallback so a development checkout built before the
  // platform-specific layout continues to work.
  return existingExecutable(context.asAbsolutePath(path.join("bin", target, executable))) ||
    existingExecutable(context.asAbsolutePath(path.join("bin", executable))) ||
    existingExecutable(context.asAbsolutePath(executable));
}

function resolveServerPath(context: vscode.ExtensionContext): string | undefined {
  const resource = vscode.window.activeTextEditor?.document.uri || vscode.workspace.workspaceFolders?.[0]?.uri;
  const config = vscode.workspace.getConfiguration("bak", resource);
  const configured = config.get<string>("lspPath")?.trim();
  const executable = lspExecutableName();

  const candidates: string[] = [];
  if (configured) {
    if (path.isAbsolute(configured)) {
      candidates.push(configured);
    } else {
      for (const root of workspaceRoots()) {
        candidates.push(path.join(root, configured));
      }
      candidates.push(context.asAbsolutePath(configured));
    }
  }

  for (const root of workspaceRoots()) {
    candidates.push(path.join(root, "bin", executable));
    candidates.push(path.join(root, executable));
  }

  for (const candidate of candidates) {
    const found = existingExecutable(candidate);
    if (found) {
      return found;
    }
  }

  if (!configured) {
    const bundled = bundledServerPath(context);
    if (bundled) {
      return bundled;
    }
  }

  if (configured && !path.isAbsolute(configured) && !configured.includes(path.sep)) {
    const found = resolveFromPath(configured);
    if (found) {
      return found;
    }
  }

  return resolveFromPath(executable);
}

async function stopClient(): Promise<void> {
  const runningClient = client;
  client = null;
  if (runningClient) {
    await runningClient.stop();
  }
}

async function startClient(context: vscode.ExtensionContext): Promise<void> {
  const serverCommand = resolveServerPath(context);
  if (!serverCommand) {
    const message = "Bak LSP binary not found. Build it with `go build -o bin/bak-lsp ./lsp` or set `bak.lspPath` to the full path.";
    outputChannel?.appendLine(message);
    vscode.window.showErrorMessage(message);
    return;
  }

  const serverOptions: ServerOptions = {
    command: serverCommand,
    args: [],
    options: { cwd: workspaceCwd() },
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "bak" }],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher("**/*.bak"),
    },
    outputChannel: outputChannel || undefined,
  };

  client = new LanguageClient(
    "bakLsp",
    "Bak Language Server",
    serverOptions,
    clientOptions
  );

  outputChannel?.appendLine(`Starting Bak LSP: ${serverCommand}`);
  await client.start();
}

async function restartClient(context: vscode.ExtensionContext): Promise<void> {
  outputChannel?.appendLine("Restarting Bak language server...");
  await stopClient();
  await startClient(context);
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  outputChannel = vscode.window.createOutputChannel("Bak Language Server", { log: true });
  context.subscriptions.push(
    vscode.commands.registerCommand("bak.showOutput", () => {
      outputChannel?.show(true);
    }),
    vscode.commands.registerCommand("bak.restartLanguageServer", () => restartClient(context)),
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration("bak.lspPath")) {
        void restartClient(context);
      }
    })
  );

  await startClient(context);
  context.subscriptions.push(outputChannel);
}

export function deactivate(): Thenable<void> {
  return stopClient();
}
