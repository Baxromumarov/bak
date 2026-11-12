use zed_extension_api::{self as zed, LanguageServerId, Result, Worktree};

struct BakExtension;

impl zed::Extension for BakExtension {
    fn new() -> Self {
        Self
    }

    fn language_server_command(
        &mut self,
        _language_server_id: &LanguageServerId,
        worktree: &Worktree,
    ) -> Result<zed::Command> {
        let command = worktree.which("bak-lsp").ok_or_else(|| {
            "Bak language server not found. Install bak-lsp on PATH (for example, ~/.local/bin/bak-lsp)."
                .to_string()
        })?;

        Ok(zed::Command {
            command,
            args: Vec::new(),
            env: Default::default(),
        })
    }
}

zed::register_extension!(BakExtension);
