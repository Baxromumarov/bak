package trace

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

type Runtime struct {
	writer io.Writer
	now    func() time.Time
	mu     sync.Mutex
}

type Scope struct {
	runtime *Runtime
	fn      string
	depth   int
	thread  int
	start   time.Time
	active  bool
}

func New(enabled bool, writer io.Writer) *Runtime {
	if !enabled {
		return nil
	}
	if writer == nil {
		writer = os.Stderr
	}
	return &Runtime{
		writer: writer,
		now:    time.Now,
	}
}

func (r *Runtime) Enter(fn string, depth int, thread int) Scope {
	if r == nil {
		return Scope{}
	}
	scope := Scope{
		runtime: r,
		fn:      fn,
		depth:   depth,
		thread:  thread,
		start:   r.now(),
		active:  true,
	}
	r.emit("enter", fn, depth, thread, "", 0, "")
	return scope
}

func (s *Scope) Exit(status string, err error) {
	if s == nil || s.runtime == nil || !s.active {
		return
	}
	duration := s.runtime.now().Sub(s.start).Nanoseconds()
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	s.runtime.emit("exit", s.fn, s.depth, s.thread, status, duration, errText)
	s.active = false
}

func (r *Runtime) emit(event string, fn string, depth int, thread int, status string, durationNS int64, errText string) {
	if r == nil {
		return
	}

	line := "bak.trace event=" + event +
		" fn=" + fn +
		" depth=" + strconv.Itoa(depth) +
		" thread=" + strconv.Itoa(thread)
	if status != "" {
		line += " status=" + status
	}
	if event == "exit" {
		line += " duration_ns=" + strconv.FormatInt(durationNS, 10)
	}
	if errText != "" {
		line += " error=" + strconv.Quote(errText)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.writer, line)
}
