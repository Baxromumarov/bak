// Package packages provides package management and visibility enforcement for the bak language.
package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/strfmt"
)

// Symbol represents a symbol exported from a package
type Symbol struct {
	Name       string
	Visibility ast.Visibility
	Kind       SymbolKind
	Node       ast.Node // The AST node that defines this symbol
}

// SymbolKind represents the kind of symbol
type SymbolKind int

const (
	SymbolFunc SymbolKind = iota
	SymbolStruct
	SymbolEnum
	SymbolConst
	SymbolType
	SymbolAlias
)

func (k SymbolKind) String() string {
	switch k {
	case SymbolFunc:
		return "function"
	case SymbolStruct:
		return "struct"
	case SymbolEnum:
		return "enum"
	case SymbolConst:
		return "constant"
	case SymbolType:
		return "type"
	case SymbolAlias:
		return "alias"
	default:
		return "unknown"
	}
}

// Package represents a parsed package with its symbols
type Package struct {
	Name            string
	Path            string             // file path or package path
	Symbols         map[string]*Symbol // name -> symbol
	Program         *ast.Program       // the parsed AST
	Imports         []string           // paths of imported packages
	ResolvedImports []string           // normalized import paths
	Used            map[string]bool    // tracks which exported symbols have been used by importers
	Fingerprint     string             // source file/directory fingerprint for stale-cache detection
	mu              sync.RWMutex
}

// SymbolSummary is a stable, serializable view of a package symbol.
type SymbolSummary struct {
	Name       string
	Kind       string
	Visibility string
}

// GraphNode is a stable snapshot of one package in a registry graph.
type GraphNode struct {
	Path            string
	Name            string
	Imports         []string
	ResolvedImports []string
	Symbols         []SymbolSummary
}

// ImportCycleError carries the resolved import chain that formed a cycle.
type ImportCycleError struct {
	Chain   []string
	Message string
}

func (e *ImportCycleError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func newImportCycleError(prefix string, chain []string) *ImportCycleError {
	copied := append([]string{}, chain...)
	return &ImportCycleError{
		Chain: copied,
		Message: strfmt.Named(
			"{prefix}: {chain}; move shared declarations into a third package",
			"prefix", prefix,
			"chain", strings.Join(copied, " -> "),
		),
	}
}

// NewPackage creates a new Package
func NewPackage(name, path string, program *ast.Program) *Package {
	pkg := &Package{
		Name:            name,
		Path:            path,
		Symbols:         make(map[string]*Symbol),
		Program:         program,
		Imports:         []string{},
		ResolvedImports: []string{},
		Used:            make(map[string]bool),
		Fingerprint:     packageFingerprint(path),
	}
	pkg.extractSymbols()
	return pkg
}

// extractSymbols extracts all symbols from the package's AST
func (p *Package) extractSymbols() {
	for _, stmt := range p.Program.Statements {
		switch node := stmt.(type) {
		case *ast.FunctionDecl:
			p.registerNamedSymbol(node.Name, node.Visibility, SymbolFunc, node)
		case *ast.StructDecl:
			p.registerNamedSymbol(node.Name, node.Visibility, SymbolStruct, node)
		case *ast.EnumDecl:
			p.registerNamedSymbol(node.Name, node.Visibility, SymbolEnum, node)
		case *ast.ConstStatement:
			p.registerNamedSymbol(node.Name, node.Visibility, SymbolConst, node)
		case *ast.ConstBlock:
			for _, c := range node.Constants {
				p.registerNamedSymbol(c.Name, c.Visibility, SymbolConst, c)
			}
		case *ast.TypeDecl:
			p.registerNamedSymbol(node.Name, node.Visibility, SymbolType, node)
		case *ast.AliasDecl:
			p.registerNamedSymbol(node.Name, node.Visibility, SymbolAlias, node)
		case *ast.ImportStatement:
			p.addImportPath(node.Path)
		case *ast.ImportBlock:
			p.addImportStatements(node.Imports)
		}
	}
}

func (p *Package) registerNamedSymbol(
	nameNode *ast.Identifier,
	visibility ast.Visibility,
	kind SymbolKind,
	node ast.Node,
) {
	if nameNode == nil || nameNode.Value == "" {
		return
	}

	p.Symbols[nameNode.Value] = &Symbol{
		Name:       nameNode.Value,
		Visibility: visibility,
		Kind:       kind,
		Node:       node,
	}
}

func (p *Package) addImportPath(path string) {
	if path == "" {
		return
	}
	p.Imports = append(p.Imports, path)
}

func (p *Package) addImportStatements(imports []*ast.ImportStatement) {
	for _, imp := range imports {
		if imp == nil {
			continue
		}
		p.addImportPath(imp.Path)
	}
}

// GetPublicSymbols returns only the public symbols
func (p *Package) GetPublicSymbols() map[string]*Symbol {
	p.mu.RLock()
	defer p.mu.RUnlock()

	public := make(map[string]*Symbol)
	for name, sym := range p.Symbols {
		if sym.Visibility == ast.Public {
			public[name] = sym
		}
	}
	return public
}

// MarkUsed marks a symbol as used in this package (called by importers)
func (p *Package) MarkUsed(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Used[name] = true
}

// WasUsed reports whether a symbol was marked used by importers
func (p *Package) WasUsed(name string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Used[name]
}

func (p *Package) AddResolvedImport(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ResolvedImports = append(p.ResolvedImports, path)
}

func (p *Package) GetResolvedImports() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.ResolvedImports) == 0 {
		return nil
	}
	copied := make([]string, len(p.ResolvedImports))
	copy(copied, p.ResolvedImports)
	return copied
}

// GetSymbol returns a symbol by name, checking visibility
func (p *Package) GetSymbol(name string, fromSamePackage bool) (*Symbol, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	sym, exists := p.Symbols[name]
	if !exists {
		publicNames := publicSymbolNames(p.Symbols)
		if suggestion := bestSuggestion(name, publicNames); suggestion != "" {
			return nil, fmt.Errorf("symbol '%s' not found in package '%s'; did you mean '%s'?", name, p.Name, suggestion)
		}
		if available := formatSymbolList(p.Symbols, 6); available != "" {
			return nil, fmt.Errorf("symbol '%s' not found in package '%s'; available public symbols: %s", name, p.Name, available)
		}
		return nil, fmt.Errorf("symbol '%s' not found in package '%s'", name, p.Name)
	}

	if !fromSamePackage && sym.Visibility != ast.Public {
		return nil, fmt.Errorf("symbol '%s' in package '%s' is private; export it with pub if it should be accessible from other packages", name, p.Name)
	}

	return sym, nil
}

// Registry manages all loaded packages
type Registry struct {
	packages    map[string]*Package // path -> package
	loading     map[string]bool     // tracks packages currently being loaded (for cycle detection)
	projectRoot string
	mu          sync.RWMutex
}

// NewRegistry creates a new package registry
func NewRegistry() *Registry {
	return &Registry{
		packages: make(map[string]*Package),
		loading:  make(map[string]bool),
	}
}

func NewRegistryWithProjectRoot(root string) *Registry {
	r := NewRegistry()
	r.SetProjectRoot(root)
	return r
}

func (r *Registry) SetProjectRoot(root string) {
	root = strings.TrimSpace(root)
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		root = filepath.Clean(root)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.projectRoot = root
}

func (r *Registry) ProjectRoot() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.projectRoot
}

// global registry instance
var GlobalRegistry = NewRegistry()

// RegisterPackage adds a package to the registry
func (r *Registry) RegisterPackage(pkg *Package) {
	if pkg == nil {
		return
	}
	pkg.Path = normalizePath(pkg.Path)
	if pkg.Fingerprint == "" {
		pkg.Fingerprint = packageFingerprint(pkg.Path)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.packages[pkg.Path] = pkg
}

// GetPackage retrieves a package by path
func (r *Registry) GetPackage(path string) (*Package, bool) {
	path = normalizePath(path)
	r.mu.RLock()
	pkg, exists := r.packages[path]
	r.mu.RUnlock()
	if !exists || pkg == nil {
		return nil, false
	}
	if packageStale(pkg) {
		r.mu.Lock()
		if current := r.packages[path]; current == pkg {
			delete(r.packages, path)
		}
		r.mu.Unlock()
		return nil, false
	}
	return pkg, true
}

// SnapshotGraph returns a deterministic snapshot of the loaded package graph.
func (r *Registry) SnapshotGraph() []GraphNode {
	r.mu.RLock()
	defer r.mu.RUnlock()

	paths := make([]string, 0, len(r.packages))
	for path := range r.packages {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	graph := make([]GraphNode, 0, len(paths))
	for _, path := range paths {
		pkg := r.packages[path]
		if pkg == nil {
			continue
		}
		graph = append(graph, packageGraphNode(pkg))
	}
	return graph
}

func packageGraphNode(pkg *Package) GraphNode {
	pkg.mu.RLock()
	defer pkg.mu.RUnlock()

	imports := append([]string(nil), pkg.Imports...)
	resolved := append([]string(nil), pkg.ResolvedImports...)
	sort.Strings(imports)
	sort.Strings(resolved)

	symbolNames := make([]string, 0, len(pkg.Symbols))
	for name := range pkg.Symbols {
		symbolNames = append(symbolNames, name)
	}
	sort.Strings(symbolNames)

	symbols := make([]SymbolSummary, 0, len(symbolNames))
	for _, name := range symbolNames {
		sym := pkg.Symbols[name]
		if sym == nil {
			continue
		}
		symbols = append(symbols, SymbolSummary{
			Name:       sym.Name,
			Kind:       sym.Kind.String(),
			Visibility: sym.Visibility.String(),
		})
	}

	return GraphNode{
		Path:            pkg.Path,
		Name:            pkg.Name,
		Imports:         imports,
		ResolvedImports: resolved,
		Symbols:         symbols,
	}
}

func packageStale(pkg *Package) bool {
	if pkg == nil || pkg.Fingerprint == "" {
		return false
	}
	current := packageFingerprint(pkg.Path)
	return current == "" || current != pkg.Fingerprint
}

func packageFingerprint(path string) string {
	path = normalizePath(path)
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		return fileFingerprint(path, info)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".bak") ||
			strings.HasPrefix(name, "test_") ||
			strings.HasSuffix(name, "_test.bak") {
			continue
		}
		filePath := filepath.Join(path, name)
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return ""
		}
		parts = append(parts, fileFingerprint(filePath, fileInfo))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func fileFingerprint(path string, info os.FileInfo) string {
	if info == nil {
		return ""
	}
	return strfmt.Named(
		"{path}:{size}:{mtime}",
		"Path", normalizePath(path),
		"Size", info.Size(),
		"Mtime", info.ModTime().UnixNano(),
	)
}

// IsLoading checks if a package is currently being loaded (for cycle detection)
func (r *Registry) IsLoading(path string) bool {
	path = normalizePath(path)
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loading[path]
}

// StartLoading marks a package as being loaded
func (r *Registry) StartLoading(path string) {
	path = normalizePath(path)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loading[path] = true
}

// FinishLoading marks a package as done loading
func (r *Registry) FinishLoading(path string) {
	path = normalizePath(path)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.loading, path)
}

// RemovePackage removes a package from the registry, forcing a reload on next import.
func (r *Registry) RemovePackage(path string) {
	path = normalizePath(path)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.packages, path)
	delete(r.loading, path)
}

func (r *Registry) RecordResolvedImport(pkgPath, importPath string) {
	pkgPath = normalizePath(pkgPath)
	importPath = normalizePath(importPath)

	r.mu.RLock()
	pkg, exists := r.packages[pkgPath]
	r.mu.RUnlock()
	if !exists {
		return
	}

	pkg.AddResolvedImport(importPath)
}

// CheckCyclicImport checks for cyclic imports starting from a package
func (r *Registry) CheckCyclicImport(
	startPath string,
	importPath string,
	visited map[string]bool,
) error {

	startPath = normalizePath(startPath)
	importPath = normalizePath(importPath)

	return r.checkCyclicImport(startPath, importPath, visited, []string{startPath})
}

func (r *Registry) checkCyclicImport(
	startPath string,
	importPath string,
	visited map[string]bool,
	chain []string,
) error {
	// Normalize paths
	startPath = normalizePath(startPath)
	importPath = normalizePath(importPath)

	if startPath == importPath {
		cycle := append(append([]string{}, chain...), importPath)
		if len(cycle) == 1 {
			cycle = append(cycle, importPath)
		}
		return newImportCycleError("package import cycle detected", cycle)
	}

	// Allow same-directory imports (files in the same package can import each other)
	if filepath.Dir(startPath) == filepath.Dir(importPath) {
		return nil
	}

	if visited[importPath] {
		cycle := append(append([]string{}, chain...), importPath)
		return newImportCycleError("cyclic import detected", cycle)
	}

	visited[importPath] = true
	defer delete(visited, importPath)

	// Check if the imported package has already been loaded
	pkg, exists := r.GetPackage(importPath)
	if !exists {
		// Package not loaded yet - no cycle from this path yet
		return nil
	}

	// Recursively check imports of the imported package
	imports := pkg.GetResolvedImports()
	if len(imports) == 0 {
		imports = pkg.Imports
	}
	for _, imp := range imports {

		if err := r.checkCyclicImport(
			startPath,
			imp,
			visited,
			append(chain, importPath),
		); err != nil {
			return err
		}
	}

	return nil
}

// normalizePath normalizes a file path for comparison
func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absPath
}

func publicSymbolNames(symbols map[string]*Symbol) []string {
	names := make([]string, 0, len(symbols))
	for name, sym := range symbols {
		if sym != nil && sym.Visibility == ast.Public {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func formatSymbolList(symbols map[string]*Symbol, limit int) string {
	entries := make([]string, 0, len(symbols))
	for name, sym := range symbols {
		if sym == nil || sym.Visibility != ast.Public {
			continue
		}
		entries = append(entries, strfmt.Named(
			"{name} ({Kind})",
			"name", name,
			"Kind", sym.Kind),
		)
	}
	sort.Strings(entries)
	if len(entries) == 0 {
		return ""
	}

	if limit > 0 && len(entries) > limit {
		return strfmt.Named(
			"{value}, and {expr} more",
			"value", strings.Join(entries[:limit], ", "),
			"expr", len(entries)-limit,
		)
	}

	return strings.Join(entries, ", ")
}

func bestSuggestion(name string, candidates []string) string {
	name = strings.TrimSpace(name)
	if name == "" || len(candidates) == 0 {
		return ""
	}
	target := strings.ToLower(name)
	best := ""
	bestLower := ""
	bestDist := -1
	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		candLower := strings.ToLower(cand)
		if candLower == target {
			continue
		}
		dist := levenshteinDistance(target, candLower)
		if bestDist == -1 || dist < bestDist {
			bestDist = dist
			best = cand
			bestLower = candLower
		}
	}
	if best == "" {
		return ""
	}
	if bestDist <= suggestionThreshold(target) {
		return best
	}

	if strings.HasPrefix(bestLower, target) ||
		strings.HasPrefix(target, bestLower) {
		if absInt(len(bestLower)-len(target)) <= 4 {
			return best
		}
	}
	return ""
}

func suggestionThreshold(name string) int {
	switch {
	case len(name) >= 10:
		return 4
	case len(name) >= 6:
		return 3
	default:
		return 2
	}
}

func levenshteinDistance(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	for j := 0; j <= len(br); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr := make([]int, len(br)+1)
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = minInt(del, ins, sub)
		}
		prev = curr
	}
	return prev[len(br)]
}

func minInt(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= a && b <= c {
		return b
	}
	return c
}

func absInt(val int) int {
	if val < 0 {
		return -val
	}
	return val
}

// GetSymbolFromPackage retrieves a symbol from a package, checking visibility
func (r *Registry) GetSymbolFromPackage(pkgPath string, symbolName string, callerPkgPath string) (*Symbol, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalizedPkgPath := normalizePath(pkgPath)
	pkg, exists := r.packages[normalizedPkgPath]
	if !exists {
		return nil, fmt.Errorf("package '%s' not found", pkgPath)
	}

	fromSamePackage := normalizedPkgPath == normalizePath(callerPkgPath)
	return pkg.GetSymbol(symbolName, fromSamePackage)
}

// GetAnySymbolFromPackage retrieves a symbol without applying visibility rules.
// It is intended for diagnostics that need to distinguish "missing" from
// "exists but private".
func (r *Registry) GetAnySymbolFromPackage(pkgPath string, symbolName string) (*Symbol, bool) {
	pkg, exists := r.GetPackage(pkgPath)
	if !exists {
		return nil, false
	}

	sym, err := pkg.GetSymbol(symbolName, true)
	if err != nil {
		return nil, false
	}
	return sym, true
}

// GetAllSymbolsFromPackage returns all accessible symbols from a package
func (r *Registry) GetAllSymbolsFromPackage(pkgPath string, callerPkgPath string) (map[string]*Symbol, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalizedPkgPath := normalizePath(pkgPath)
	pkg, exists := r.packages[normalizedPkgPath]
	if !exists {
		return nil, fmt.Errorf("package '%s' not found", pkgPath)
	}

	fromSamePackage := normalizedPkgPath == normalizePath(callerPkgPath)
	if fromSamePackage {
		return pkg.Symbols, nil
	}
	return pkg.GetPublicSymbols(), nil
}

// GetAllPackages returns all registered packages
func (r *Registry) GetAllPackages() []*Package {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Package, 0, len(r.packages))
	for _, pkg := range r.packages {
		result = append(result, pkg)
	}

	return result
}

// Reset clears the registry (useful for testing)
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.packages = make(map[string]*Package)
	r.loading = make(map[string]bool)
}
