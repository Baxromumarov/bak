"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const vscode = require("vscode");
const node_1 = require("vscode-languageclient/node");
let client = null;
let outputChannel = null;
function resolveServerPath() {
    const config = vscode.workspace.getConfiguration("bak");
    const configured = config.get("lspPath");
    if (configured && configured.trim() !== "") {
        return configured;
    }
    return "bak-lsp";
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