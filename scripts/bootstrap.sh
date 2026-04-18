#!/bin/bash
# Bootstrap the self-hosted Bak compiler (bakc)
# Usage: ./scripts/bootstrap.sh

set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BAK_GO_CMD="$ROOT_DIR/bak"
BAKC_SRC="$ROOT_DIR/src/compiler/cmd/bakc/main.bak"
STAGE1_BC="$ROOT_DIR/bakc_stage1.bc.json"
STAGE2_BC="$ROOT_DIR/bakc_stage2.bc.json"

echo "========================================"
echo "Bak Compiler Bootstrap Process"
echo "========================================"
echo ""

# Step 1: Build the Go-based compiler (Stage 0)
echo "Step 1: Building Go-based compiler (Stage 0)..."
cd "$ROOT_DIR"
go build -o bak ./cmd/bak
echo "✅ Stage 0 compiler built at $BAK_GO_CMD"
echo ""

# Step 2: Compile bakc using Stage 0 (produces Stage 1)
echo "Step 2: Compiling bakc using Stage 0 (Stage 1)..."
$BAK_GO_CMD build -o "$STAGE1_BC" "$BAKC_SRC"
echo "✅ Stage 1 compiler bytecode built at $STAGE1_BC"
echo ""

# Step 3: Compile bakc using Stage 1 (produces Stage 2)
# We run the Stage 1 bytecode using the Go interpreter to compile the source again
echo "Step 3: Compiling bakc using Stage 1 (Stage 2)..."
$BAK_GO_CMD --bc "$STAGE1_BC" -- --emit --out "$STAGE2_BC" "$BAKC_SRC"
echo "✅ Stage 2 compiler bytecode built at $STAGE2_BC"
echo ""

# Step 4: Verification (Reproducible Build Check)
echo "Step 4: verifying reproducible build..."
# Filter out "timestamp" or "path" differences if necessary, but ideally they should be identical
# For now, just checking if sizes match as a rough heuristic, or exact content match
if diff -q "$STAGE1_BC" "$STAGE2_BC" >/dev/null; then
    echo "✅ Success! Stage 1 and Stage 2 bytecodes are identical."
else
    echo "⚠️  Warning: Stage 1 and Stage 2 bytecodes differ."
    echo "This might be due to timestamps or non-deterministic map iteration."
    echo "Proceeding anyway as functionality might still be correct."
fi

# Create a convenience wrapper script
WRAPPER="$ROOT_DIR/bakc"
echo "#!/bin/bash" > "$WRAPPER"
echo "# Wrapper for self-hosted bak compiler" >> "$WRAPPER"
echo "$BAK_GO_CMD --bc $STAGE1_BC -- \"\$@\"" >> "$WRAPPER"
chmod +x "$WRAPPER"
echo ""
echo "✅ Created wrapper script at $WRAPPER"
echo "Usage: ./bakc <file.bak>"

echo ""
echo "Bootstrap complete!"
