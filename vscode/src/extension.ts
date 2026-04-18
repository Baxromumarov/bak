import * as vscode from "vscode";
import * as path from "path";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from "vscode-languageclient/node";

let client: LanguageClient | null = null;
let outputChannel: vscode.OutputChannel | null = null;

function resolveServerPath(): string {
  const config = vscode.workspace.getConfiguration("bak");
  const configured = config.get<string>("lspPath");
  if (configured && configured.trim() !== "") {
    return configured;
  }
  return "bak-lsp";
}

export function activate(context: vscode.ExtensionContext) {
  outputChannel = vscode.window.createOutputChannel("Bak Language Server");
  outputChannel.show(true);
  context.subscriptions.push(
    vscode.commands.registerCommand("bak.showOutput", () => {
      if (outputChannel) {
        outputChannel.show(true);
      }
    })
  );
  const serverCommand = resolveServerPath();

  const serverOptions: ServerOptions = {
    command: serverCommand,
    args: [],
    options: { cwd: vscode.workspace.rootPath || process.cwd() },
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "bak" }],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher("**/*.bak"),
    },
    outputChannel,
  };

  client = new LanguageClient(
    "bakLsp",
    "Bak Language Server",
    serverOptions,
    clientOptions
  );

  outputChannel.appendLine(`Starting Bak LSP: ${serverCommand}`);
  client.start();
  context.subscriptions.push(client);
  context.subscriptions.push(outputChannel);
}

export function deactivate(): Thenable<void> | undefined {
  if (!client) {
    return undefined;
  }
  return client.stop();
}
