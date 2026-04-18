"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const vscode = require("vscode");
const fs = require("fs");
const path = require("path");
const node_1 = require("vscode-languageclient/node");
let client = null;
let outputChannel = null;
function resolveServerPath() {
    const config = vscode.workspace.getConfiguration("bak");
    const configured = config.get("lspPath")?.trim();
    const workspaceRoot = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || process.cwd();
    const candidates = [];
    if (configured) {
        candidates.push(path.isAbsolute(configured) ? configured : path.join(workspaceRoot, configured));
    }
    candidates.push(path.join(workspaceRoot, "bin", "bak-lsp"));
    candidates.push(path.join(workspaceRoot, "bak-lsp"));
    candidates.push("bak-lsp");
    for (const candidate of candidates) {
        if (candidate === "bak-lsp" || fs.existsSync(candidate)) {
            return candidate;
        }
    }
    return candidates[0] ?? "bak-lsp";
}
function activate(context) {
    outputChannel = vscode.window.createOutputChannel("Bak Language Server");
    outputChannel.show(true);
    context.subscriptions.push(vscode.commands.registerCommand("bak.showOutput", () => {
        if (outputChannel) {
            outputChannel.show(true);
        }
    }));
    const serverCommand = resolveServerPath();
    const serverOptions = {
        command: serverCommand,
        args: [],
        options: { cwd: vscode.workspace.rootPath || process.cwd() },
    };
    const clientOptions = {
        documentSelector: [{ scheme: "file", language: "bak" }],
        synchronize: {
            fileEvents: vscode.workspace.createFileSystemWatcher("**/*.bak"),
        },
        outputChannel,
    };
    client = new node_1.LanguageClient("bakLsp", "Bak Language Server", serverOptions, clientOptions);
    outputChannel.appendLine(`Starting Bak LSP: ${serverCommand}`);
    client.start();
    context.subscriptions.push(client);
    context.subscriptions.push(outputChannel);
}
function deactivate() {
    if (!client) {
        return undefined;
    }
    return client.stop();
}
//# sourceMappingURL=extension.js.map