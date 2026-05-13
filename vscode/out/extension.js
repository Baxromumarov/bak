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
function lspExecutableName() {
    return process.platform === "win32" ? "bak-lsp.exe" : "bak-lsp";
}
function existingExecutable(candidate) {
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
    }
    catch {
        return undefined;
    }
}
function resolveFromPath(command) {
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
function workspaceRoots() {
    const folders = vscode.workspace.workspaceFolders || [];
    if (folders.length > 0) {
        return folders.map((folder) => folder.uri.fsPath);
    }
    return [process.cwd()];
}
function resolveServerPath(context) {
    const config = vscode.workspace.getConfiguration("bak");
    const configured = config.get("lspPath")?.trim();
    const executable = lspExecutableName();
    const candidates = [];
    if (configured) {
        if (path.isAbsolute(configured)) {
            candidates.push(configured);
        }
        else {
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
    candidates.push(context.asAbsolutePath(path.join("bin", executable)));
    candidates.push(context.asAbsolutePath(executable));
    for (const candidate of candidates) {
        const found = existingExecutable(candidate);
        if (found) {
            return found;
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
function activate(context) {
    outputChannel = vscode.window.createOutputChannel("Bak Language Server");
    outputChannel.show(true);
    context.subscriptions.push(vscode.commands.registerCommand("bak.showOutput", () => {
        if (outputChannel) {
            outputChannel.show(true);
        }
    }));
    const serverCommand = resolveServerPath(context);
    if (!serverCommand) {
        const message = "Bak LSP binary not found. Build it with `go build -o bin/bak-lsp ./lsp` or set `bak.lspPath` to the full path.";
        outputChannel.appendLine(message);
        vscode.window.showErrorMessage(message);
        context.subscriptions.push(outputChannel);
        return;
    }
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