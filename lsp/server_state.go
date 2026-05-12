package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"time"
)

func (s *Server) setDocument(uri, text string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.Documents[uri] = text
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
	for uri, text := range s.Documents {
		out[uri] = text
	}
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
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.Indexes[uri] = index
	s.Cache[uri] = result
}

func (s *Server) invalidateAnalysisForURI(uri string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	delete(s.Cache, uri)
	delete(s.Indexes, uri)
}

func (s *Server) closeDocument(uri string) {
	var oldCancel context.CancelFunc
	var oldTimer *time.Timer

	s.stateMu.Lock()
	delete(s.Documents, uri)
	delete(s.Cache, uri)
	delete(s.Indexes, uri)
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
	timer := time.AfterFunc(delay, func() {
		s.reanalyzeOpenDocuments()
	})

	s.stateMu.Lock()
	oldTimer := s.workspaceTimer
	s.workspaceTimer = timer
	s.stateMu.Unlock()

	if oldTimer != nil {
		oldTimer.Stop()
	}
}

func (s *Server) reanalyzeOpenDocuments() {
	for uri, text := range s.documentSnapshot() {
		uri, text := uri, text
		s.invalidateAnalysisForURI(uri)
		s.resetPendingAnalysis(uri, 0, func(ctx context.Context) {
			s.analyzeAndPublish(ctx, uri, text)
		})
	}
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
	for uri, result := range s.Cache {
		out[uri] = result
	}
	return out
}

func (s *Server) indexSnapshot() map[string]*FileIndex {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	out := make(map[string]*FileIndex, len(s.Indexes))
	for uri, idx := range s.Indexes {
		out[uri] = idx
	}
	return out
}
