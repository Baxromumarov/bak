package main

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"time"
)

func (s *Server) setDocument(uri, text string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.Documents[uri] = text
}

func (s *Server) setRootPath(root string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if s.RootPath == root {
		return
	}
	s.RootPath = root
	s.stdImportPaths = nil
	s.stdPackages = nil
}

func (s *Server) rootPath() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	return s.RootPath
}

func (s *Server) document(uri string) (string, bool) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	text, ok := s.Documents[uri]
	return text, ok
}

func (s *Server) documentSnapshot() map[string]string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	out := make(map[string]string, len(s.Documents))
	maps.Copy(out, s.Documents)
	return out
}

func (s *Server) cancelRequest(id json.RawMessage) {
	key := requestIDKey(id)
	if key == "" {
		return
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.canceled[key] = struct{}{}
	if cancel := s.activeRequests[key]; cancel != nil {
		cancel()
	}
}

func (s *Server) isRequestCanceled(id json.RawMessage) bool {
	key := requestIDKey(id)
	if key == "" {
		return false
	}

	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	_, ok := s.canceled[key]
	return ok
}

func (s *Server) finishRequest(id json.RawMessage) {
	key := requestIDKey(id)
	if key == "" {
		return
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	delete(s.canceled, key)
	delete(s.activeRequests, key)
}

func (s *Server) startRequest(id json.RawMessage) context.Context {
	key := requestIDKey(id)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if key == "" {
		return ctx
	}

	s.stateMu.Lock()
	if _, ok := s.canceled[key]; ok {
		cancel()
	} else {
		s.activeRequests[key] = cancel
	}
	s.stateMu.Unlock()

	return ctx
}

func requestIDKey(id json.RawMessage) string {
	if len(id) == 0 {
		return ""
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, id); err == nil {
		return compact.String()
	}
	return string(id)
}

func (s *Server) analysisResult(uri string) (*AnalysisResult, bool) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	result, ok := s.Cache[uri]
	return result, ok
}

func (s *Server) analysisResultOrNil(uri string) *AnalysisResult {
	result, _ := s.analysisResult(uri)
	return result
}

func (s *Server) setAnalysisResult(uri string, index *FileIndex, result *AnalysisResult) {
	importedURIs := s.importDependencyKeys(uri, result)

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.removeReverseDepsLocked(uri)
	s.Indexes[uri] = index
	s.Cache[uri] = result
	s.addReverseDepsLocked(uri, importedURIs)
}

func (s *Server) invalidateAnalysisForURI(uri string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	delete(s.Cache, uri)
	delete(s.Indexes, uri)
	s.removeReverseDepsLocked(uri)
}

func (s *Server) closeDocument(uri string) {
	var oldCancel context.CancelFunc
	var oldTimer *time.Timer

	s.stateMu.Lock()
	delete(s.Documents, uri)
	delete(s.Cache, uri)
	delete(s.Indexes, uri)
	s.removeReverseDepsLocked(uri)
	oldCancel = s.pendingCancel[uri]
	oldTimer = s.pendingLocks[uri]
	delete(s.pendingCancel, uri)
	delete(s.pendingLocks, uri)
	s.stateMu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}
	if oldTimer != nil {
		oldTimer.Stop()
	}

	s.invalidatePublicIndexesForURI(uri)
}

func (s *Server) Close() {
	var pendingCancels []context.CancelFunc
	var activeCancels []context.CancelFunc
	var timers []*time.Timer

	s.stateMu.Lock()
	for _, cancel := range s.pendingCancel {
		pendingCancels = append(pendingCancels, cancel)
	}
	for _, cancel := range s.activeRequests {
		activeCancels = append(activeCancels, cancel)
	}
	for _, timer := range s.pendingLocks {
		timers = append(timers, timer)
	}
	if s.workspaceTimer != nil {
		timers = append(timers, s.workspaceTimer)
	}

	s.pendingCancel = make(map[string]context.CancelFunc)
	s.activeRequests = make(map[string]context.CancelFunc)
	s.pendingLocks = make(map[string]*time.Timer)
	s.canceled = make(map[string]struct{})
	s.watchedChanges = make(map[string]struct{})
	s.ReverseDeps = make(map[string]map[string]struct{})
	s.workspaceTimer = nil
	s.stateMu.Unlock()

	for _, cancel := range pendingCancels {
		cancel()
	}
	for _, cancel := range activeCancels {
		cancel()
	}
	for _, timer := range timers {
		if timer != nil {
			timer.Stop()
		}
	}
}

func (s *Server) setIndex(uri string, index *FileIndex) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.Indexes[uri] = index
	if result := s.Cache[uri]; result != nil && result.Index == nil {
		result.Index = index
	}
}

func (s *Server) resetPendingAnalysis(uri string, delay time.Duration, analyze func(context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	var timer *time.Timer
	if delay > 0 {
		timer = time.AfterFunc(delay, func() {
			analyze(ctx)
		})
	}

	var oldCancel context.CancelFunc
	var oldTimer *time.Timer
	s.stateMu.Lock()
	oldCancel = s.pendingCancel[uri]
	oldTimer = s.pendingLocks[uri]
	s.pendingCancel[uri] = cancel
	if timer == nil {
		delete(s.pendingLocks, uri)
	} else {
		s.pendingLocks[uri] = timer
	}
	s.stateMu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}
	if oldTimer != nil {
		oldTimer.Stop()
	}
	if delay == 0 {
		analyze(ctx)
	}
}

func (s *Server) resetWorkspaceReanalysis(delay time.Duration) {
	s.stateMu.Lock()
	if len(s.watchedChanges) == 0 {
		s.stateMu.Unlock()
		return
	}
	s.stateMu.Unlock()

	timer := time.AfterFunc(delay, func() {
		s.reanalyzeOpenDocumentsAffectedByWatchedChanges()
	})

	s.stateMu.Lock()
	oldTimer := s.workspaceTimer
	s.workspaceTimer = timer
	s.stateMu.Unlock()

	if oldTimer != nil {
		oldTimer.Stop()
	}
}

func (s *Server) addWatchedChanges(uris []string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	for _, uri := range uris {
		if uri != "" {
			s.watchedChanges[uri] = struct{}{}
		}
	}
}

func (s *Server) takeWatchedChanges() map[string]struct{} {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	changes := s.watchedChanges
	s.watchedChanges = make(map[string]struct{})
	s.workspaceTimer = nil
	return changes
}

func (s *Server) reanalyzeOpenDocumentsAffectedByWatchedChanges() {
	changes := s.takeWatchedChanges()
	if len(changes) == 0 {
		return
	}
	dependents := s.dependentsOfChangedURIs(changes)

	for uri, text := range s.documentSnapshot() {
		if !uriInSet(uri, dependents) && !s.openDocumentAffectedByChanges(uri, changes) {
			continue
		}
		uri, text := uri, text
		s.invalidateAnalysisForURI(uri)
		s.resetPendingAnalysis(uri, 0, func(ctx context.Context) {
			s.analyzeAndPublish(ctx, uri, text)
		})
	}
}

func (s *Server) openDocumentAffectedByChanges(uri string, changes map[string]struct{}) bool {
	if uriInSet(uri, changes) {
		return true
	}

	result := s.analysisResultOrNil(uri)
	if result == nil {
		return true
	}
	for _, importPath := range result.Imports {
		resolved := s.resolveImportPath(uriToPath(uri), importPath)
		if resolved == "" {
			continue
		}
		if pathAffectedByChanges(resolved, changes) {
			return true
		}
	}
	return false
}

func (s *Server) importDependencyKeys(importerURI string, result *AnalysisResult) []string {
	if result == nil || len(result.Imports) == 0 {
		return nil
	}

	baseFile := uriToPath(importerURI)
	var keys []string
	seen := make(map[string]struct{})
	for _, importPath := range result.Imports {
		resolved := s.resolveImportPath(baseFile, importPath)
		for _, key := range dependencyKeysForPath(resolved) {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	return keys
}

func (s *Server) addReverseDepsLocked(importerURI string, importedKeys []string) {
	if importerURI == "" {
		return
	}
	if s.ReverseDeps == nil {
		s.ReverseDeps = make(map[string]map[string]struct{})
	}
	for _, importedURI := range importedKeys {
		if importedURI == "" || importedURI == importerURI {
			continue
		}
		dependents := s.ReverseDeps[importedURI]
		if dependents == nil {
			dependents = make(map[string]struct{})
			s.ReverseDeps[importedURI] = dependents
		}
		dependents[importerURI] = struct{}{}
	}
}

func (s *Server) removeReverseDepsLocked(importerURI string) {
	for importedURI, dependents := range s.ReverseDeps {
		delete(dependents, importerURI)
		if len(dependents) == 0 {
			delete(s.ReverseDeps, importedURI)
		}
	}
}

func (s *Server) dependentsOfChangedURIs(changes map[string]struct{}) map[string]struct{} {
	affected := make(map[string]struct{})
	queue := changeDependencyKeys(changes)

	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		for importerURI := range s.ReverseDeps[key] {
			if _, ok := affected[importerURI]; ok {
				continue
			}
			affected[importerURI] = struct{}{}
			queue = append(queue, dependencyKeysForURI(importerURI)...)
		}
	}
	return affected
}

func changeDependencyKeys(changes map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(changes)*3)
	var keys []string
	for uri := range changes {
		for _, key := range dependencyKeysForURI(uri) {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	return keys
}

func dependencyKeysForURI(uri string) []string {
	keys := []string{uri}
	path := uriToPath(uri)
	if path == "" {
		return keys
	}
	return append(keys, dependencyKeysForPath(path)...)
}

func dependencyKeysForPath(path string) []string {
	if path == "" {
		return nil
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	path = filepath.Clean(path)

	keys := []string{pathToURI(path)}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		keys = append(keys, pathToURI(filepath.Dir(path)))
	}
	return keys
}

func pathAffectedByChanges(path string, changes map[string]struct{}) bool {
	if uriInSet(pathToURI(path), changes) {
		return true
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	dir, err := filepath.Abs(path)
	if err != nil {
		dir = filepath.Clean(path)
	}
	for uri := range changes {
		changed := uriToPath(uri)
		if changed == "" {
			continue
		}
		if abs, err := filepath.Abs(changed); err == nil {
			changed = abs
		}
		if filepath.Dir(filepath.Clean(changed)) == dir {
			return true
		}
	}
	return false
}

func uriInSet(uri string, set map[string]struct{}) bool {
	if _, ok := set[uri]; ok {
		return true
	}
	path := uriToPath(uri)
	if path == "" {
		return false
	}
	if _, ok := set[pathToURI(path)]; ok {
		return true
	}
	if abs, err := filepath.Abs(path); err == nil {
		_, ok := set[pathToURI(abs)]
		return ok
	}
	return false
}

func (s *Server) publicIndex(uri string) (*FileIndex, bool) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	idx, ok := s.PublicIndexes[uri]
	return idx, ok
}

func (s *Server) setPublicIndex(uri string, idx *FileIndex) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.PublicIndexes[uri] = idx
}

func (s *Server) invalidatePublicIndexesForURI(uri string) {
	path := uriToPath(uri)
	keys := map[string]bool{uri: true}
	addPath := func(path string) {
		if path == "" {
			return
		}
		keys[pathToURI(path)] = true
		keys[pathToURI(filepath.Clean(path))] = true
		if abs, err := filepath.Abs(path); err == nil {
			keys[pathToURI(abs)] = true
			keys[pathToURI(filepath.Clean(abs))] = true
		}
	}
	addPath(path)
	addPath(filepath.Dir(path))

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	for key := range keys {
		delete(s.PublicIndexes, key)
	}
}

func (s *Server) cacheSnapshot() map[string]*AnalysisResult {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	out := make(map[string]*AnalysisResult, len(s.Cache))
	maps.Copy(out, s.Cache)
	return out
}

func (s *Server) indexSnapshot() map[string]*FileIndex {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	out := make(map[string]*FileIndex, len(s.Indexes))
	maps.Copy(out, s.Indexes)
	return out
}
