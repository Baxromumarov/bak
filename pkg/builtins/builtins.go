// Package builtins provides the built-in functions for the bak language.
package builtins

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/baxromumarov/bak/pkg/object"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

var (
	connMu         sync.Mutex
	activeConns    = make(map[int]net.Conn)
	listeners      = make(map[int]net.Listener)
	nextConnID     = 1
	nextListenerID = 1000000

	mutexMu     sync.Mutex
	mutexes     = make(map[int]*sync.Mutex)
	nextMutexID = 1
)

// Builtins contains all built-in functions
var Builtins = map[string]*object.Builtin{
	"fromChars": {Fn: builtinFromChars},
	"print":     {Fn: builtinPrint},
	"println":   {Fn: builtinPrintln},
	"type":      {Fn: builtinType},
	"typeof":    {Fn: builtinType},
	"int":       {Fn: builtinInt},
	"float":     {Fn: builtinFloat},
	"string":    {Fn: builtinString},
	"char":      {Fn: builtinChar},
	"Box":       {Fn: builtinBox},
	"unbox":     {Fn: builtinUnbox},
	"concat":    {Fn: builtinConcat},
	// __builtin_* functions for std library support
	"__builtin_args":               {Fn: osArgs},
	"__builtin_exit":               {Fn: osExit},
	"__builtin_getenv":             {Fn: osGetenv},
	"__builtin_setenv":             {Fn: osSetenv},
	"__builtin_cwd":                {Fn: osCwd},
	"__builtin_chdir":              {Fn: osChdir},
	"__builtin_print":              {Fn: builtinPrint},
	"__builtin_println":            {Fn: builtinPrintln},
	"__builtin_eprint":             {Fn: builtinEprint},
	"__builtin_eprintln":           {Fn: builtinEprintln},
	"__builtin_read_file":          {Fn: fsReadFile},
	"__builtin_read_file_bytes":    {Fn: fsReadFileBytes},
	"__builtin_write_file":         {Fn: fsWriteFile},
	"__builtin_write_file_bytes":   {Fn: fsWriteFileBytes},
	"__builtin_append_file":        {Fn: fsAppendFile},
	"__builtin_file_exists":        {Fn: fsExists},
	"__builtin_is_file":            {Fn: fsIsFile},
	"__builtin_is_dir":             {Fn: fsIsDir},
	"__builtin_read_dir":           {Fn: fsReadDir},
	"__builtin_remove":             {Fn: fsRemove},
	"__builtin_mkdir":              {Fn: fsMkdir},
	"__builtin_chmod":              {Fn: osChmod},
	"__builtin_exec":               {Fn: osExec},
	"__builtin_string_from_bytes":  {Fn: builtinStringFromBytes},
	"__builtin_string_ptr":         {Fn: builtinStringPtr},
	"__builtin_socket_connect":     {Fn: builtinSocketConnect},
	"__builtin_socket_read":        {Fn: builtinSocketRead},
	"__builtin_socket_write":       {Fn: builtinSocketWrite},
	"__builtin_socket_close":       {Fn: builtinSocketClose},
	"__builtin_socket_connect_tls": {Fn: builtinSocketConnectTLS},
	"__builtin_socket_set_timeout": {Fn: builtinSocketSetTimeout},
	"__builtin_socket_bind":        {Fn: builtinSocketBind},
	"__builtin_socket_accept":      {Fn: builtinSocketAccept},
	"cfg":                          {Fn: builtinCfg},
	"__builtin_sleep":              {Fn: builtinSleep},
	"__builtin_time_now":           {Fn: builtinTimeNow},
	"__builtin_time_parts":         {Fn: builtinTimeParts},
	"__builtin_monotonic_now":      {Fn: builtinMonotonicNow},
	"__builtin_thread_id":          {Fn: builtinThreadID},
	"__builtin_executable":         {Fn: osExecutable},
	"__builtin_hostname":           {Fn: osHostname},
	"__builtin_temp_dir":           {Fn: osTempDir},
	"__builtin_user_home_dir":      {Fn: osUserHomeDir},
	"__builtin_mutex_new":          {Fn: builtinMutexNew},
	"__builtin_mutex_lock":         {Fn: builtinMutexLock},
	"__builtin_mutex_unlock":       {Fn: builtinMutexUnlock},
	"__alloc_array":                {Fn: builtinAllocArray},
	"__alloc_array_zeroed":         {Fn: builtinAllocArray}, // Same for now
	"__vec_alloc":                  {Fn: builtinVecAlloc},
	"__vec_len":                    {Fn: builtinVecLen},
	"__vec_cap":                    {Fn: builtinVecCap},
	"__vec_get":                    {Fn: builtinVecGet},
	"__vec_set":                    {Fn: builtinVecSet},
	"__vec_grow":                   {Fn: builtinVecGrow},
}

// builtinConcat concatenates two strings
func builtinConcat(args ...object.Object) object.Object {
	if len(args) == 0 {
		return newError("concat: wrong number of arguments. got=%d, want at least 1", len(args))
	}

	var out strings.Builder

	for _, arg := range args {
		ch, ok := arg.(*object.String)
		if !ok {
			return newError("fromChars: element is not char")
		}

		out.WriteString(ch.Value)
	}

	return &object.String{Value: out.String()}
}

func builtinCfg(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("cfg: wrong number of arguments. got=%d, want=1", len(args))
	}
	featureName, ok := args[0].(*object.String)
	if !ok {
		return newError("cfg: argument must be string, got %s", args[0].Type())
	}
	return &object.Boolean{Value: runtimecap.CurrentFeatureEnabled(featureName.Value)}
}

// builtinFromChars: primitive to convert Vec<char, _> to string
func builtinFromChars(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("fromChars: wrong number of arguments. got=%d, want=1", len(args))
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return newError("fromChars: argument must be Vec<char, _>, got %s", args[0].Type())
	}
	// Check element type
	if vec.ElemType != "char" {
		return newError("fromChars: Vec element type must be char, got %s", vec.ElemType)
	}
	runes := make([]rune, len(vec.Elements))
	for i, el := range vec.Elements {
		ch, ok := el.(*object.Char)
		if !ok {
			return newError("fromChars: element at %d is not char", i)
		}
		runes[i] = ch.Value
	}
	return &object.String{Value: string(runes)}
}

// builtinStringFromBytes: optimized string construction from Vec<int, _> bytes
func builtinStringFromBytes(args ...object.Object) object.Object {
	if len(args) != 3 {
		return newError("wrong number of arguments. got=%d, want=3", len(args))
	}
	// Support both raw *object.Vec and *object.Struct (Vec from stdlib)
	var vec *object.Vec
	switch v := args[0].(type) {
	case *object.Vec:
		vec = v
	case *object.Struct:
		if v.Name == "Vec" || strings.HasSuffix(v.Name, ".Vec") {
			if dataField, ok := v.Fields["data"]; ok {
				if datVec, ok := dataField.(*object.Vec); ok {
					vec = datVec
				}
			}
		}
	}
	if vec == nil {
		return newError("argument to __builtin_string_from_bytes must be Vec (or Vec struct), got %s", args[0].Type())
	}
	start, ok := args[1].(*object.Integer)
	if !ok {
		return newError("start argument must be int, got %s", args[1].Type())
	}
	end, ok := args[2].(*object.Integer)
	if !ok {
		return newError("end argument must be int, got %s", args[2].Type())
	}
	s := start.Value
	e := end.Value
	if s < 0 || e < 0 || s > e || e > int64(len(vec.Elements)) {
		return newError("invalid bounds for string_from_bytes: start=%d, end=%d, len=%d", s, e, len(vec.Elements))
	}
	// Extract bytes
	var builder strings.Builder
	builder.Grow(int(e - s))
	for i := s; i < e; i++ {
		b, ok := vec.Elements[i].(*object.Integer)
		if !ok {
			return newError("vec element not an int")
		}
		builder.WriteByte(byte(b.Value))
	}
	return &object.String{Value: builder.String()}
}

// builtinStringPtr: mocked for evaluator - not meant to be used literally there, but registered.
func builtinStringPtr(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}
	return &object.Integer{Value: 0}
}

func newError(format string, a ...any) *object.Error {
	return &object.Error{Message: fmt.Sprintf(format, a...)}
}

func builtinPrint(args ...object.Object) object.Object {
	for i, arg := range args {
		if i > 0 {
			fmt.Print(" ")
		}
		switch v := arg.(type) {
		case *object.String:
			fmt.Print(v.Value)
		case *object.Char:
			fmt.Print(string(v.Value))
		default:
			fmt.Print(arg.Inspect())
		}
	}
	return &object.Void{}
}

func builtinPrintln(args ...object.Object) object.Object {
	for i, arg := range args {
		if i > 0 {
			fmt.Print(" ")
		}
		switch v := arg.(type) {
		case *object.String:
			fmt.Print(v.Value)
		case *object.Char:
			fmt.Print(string(v.Value))
		default:
			fmt.Print(arg.Inspect())
		}
	}
	fmt.Println()
	return &object.Void{}
}

func builtinEprint(args ...object.Object) object.Object {
	for i, arg := range args {
		if i > 0 {
			fmt.Fprint(os.Stderr, " ")
		}
		switch v := arg.(type) {
		case *object.String:
			fmt.Fprint(os.Stderr, v.Value)
		case *object.Char:
			fmt.Fprint(os.Stderr, string(v.Value))
		default:
			fmt.Fprint(os.Stderr, arg.Inspect())
		}
	}
	return &object.Void{}
}

func builtinEprintln(args ...object.Object) object.Object {
	for i, arg := range args {
		if i > 0 {
			fmt.Fprint(os.Stderr, " ")
		}
		switch v := arg.(type) {
		case *object.String:
			fmt.Fprint(os.Stderr, v.Value)
		case *object.Char:
			fmt.Fprint(os.Stderr, string(v.Value))
		default:
			fmt.Fprint(os.Stderr, arg.Inspect())
		}
	}
	fmt.Fprintln(os.Stderr)
	return &object.Void{}
}

func builtinType(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}

	return &object.String{Value: string(args[0].Type())}
}

func builtinInt(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}

	switch arg := args[0].(type) {
	case *object.Integer:
		return arg
	case *object.Float:
		return &object.Integer{Value: int64(arg.Value)}
	case *object.String:
		var val int64
		_, err := fmt.Sscanf(arg.Value, "%d", &val)
		if err != nil {
			return &object.Result{IsOk: false, Value: &object.String{Value: "invalid integer"}}
		}
		return &object.Result{IsOk: true, Value: &object.Integer{Value: val}}
	case *object.Boolean:
		if arg.Value {
			return &object.Integer{Value: 1}
		}
		return &object.Integer{Value: 0}
	default:
		return newError("cannot convert %s to int", args[0].Type())
	}
}

func builtinFloat(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}

	switch arg := args[0].(type) {
	case *object.Float:
		return arg
	case *object.Integer:
		return &object.Float{Value: float64(arg.Value)}
	case *object.String:
		var val float64
		_, err := fmt.Sscanf(arg.Value, "%f", &val)
		if err != nil {
			return &object.Result{IsOk: false, Value: &object.String{Value: "invalid float"}}
		}
		return &object.Result{IsOk: true, Value: &object.Float{Value: val}}
	default:
		return newError("cannot convert %s to float", args[0].Type())
	}
}

func builtinString(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}

	return &object.String{Value: args[0].Inspect()}
}

// builtinChar creates a character from an ASCII code
func builtinChar(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}

	switch arg := args[0].(type) {
	case *object.Integer:
		return &object.Char{Value: rune(arg.Value)}
	case *object.Char:
		return arg
	default:
		return newError("char: argument must be INTEGER, got %s", args[0].Type())
	}
}

// builtinBox wraps a value in a Box (heap allocation for recursive types)
func builtinBox(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}

	return &object.Box{Value: args[0]}
}

// builtinUnbox extracts the value from a Box
func builtinUnbox(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("wrong number of arguments. got=%d, want=1", len(args))
	}

	box, ok := args[0].(*object.Box)
	if !ok {
		return newError("unbox: argument must be BOX, got %s", args[0].Type())
	}

	if box.Value == nil {
		return &object.Void{}
	}

	return box.Value
}

func builtinSocketConnect(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__builtin_socket_connect: wrong number of arguments. got=%d, want=2", len(args))
	}

	hostObj, ok := args[0].(*object.String)
	if !ok {
		return newError("__builtin_socket_connect: argument 0 must be string, got %s", args[0].Type())
	}
	portObj, ok := args[1].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_connect: argument 1 must be int, got %s", args[1].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("socket.connect", runtimecap.FlagAllowNet)},
		}
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", hostObj.Value, portObj.Value), 10*time.Second)
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	connMu.Lock()
	id := nextConnID
	nextConnID++
	activeConns[id] = conn
	connMu.Unlock()

	return &object.Result{IsOk: true, Value: &object.Integer{Value: int64(id)}}
}

func builtinSocketRead(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__builtin_socket_read: wrong number of arguments. got=%d, want=2", len(args))
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_read: argument 0 must be int (fd), got %s", args[0].Type())
	}
	nObj, ok := args[1].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_read: argument 1 must be int (n), got %s", args[1].Type())
	}

	connMu.Lock()
	conn, ok := activeConns[int(fdObj.Value)]
	connMu.Unlock()

	if !ok {
		return &object.Result{IsOk: false, Value: &object.String{Value: "invalid socket fd"}}
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("socket.read", runtimecap.FlagAllowNet)},
		}
	}

	if nObj.Value < 0 {
		return &object.Result{IsOk: false, Value: &object.String{Value: "socket read count must be non-negative"}}
	}

	buf := make([]byte, int(nObj.Value))
	n, err := conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			// Return empty Vec on EOF
			return &object.Result{IsOk: true, Value: &object.Vec{Elements: []object.Object{}, Size: -1, Mutable: true, ElemType: "byte"}}
		}
		if n == 0 {
			return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
		}
		// If partial read, that's fine.
	}

	// Create Vec<byte, _> (which is Vec<int, _> in Bak runtime usually, unless we use byte array optimization?)
	// Bak currently uses Vec<Object, _> internally, so it's Vec<Integer, _>.
	// This is inefficient but consistent.
	elements := make([]object.Object, n)
	for i := range n {
		elements[i] = &object.Integer{Value: int64(buf[i])}
	}
	vec := &object.Vec{Elements: elements, Size: -1, Mutable: true, ElemType: "byte"}

	return &object.Result{IsOk: true, Value: vec}
}

func builtinSocketWrite(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__builtin_socket_write: wrong number of arguments. got=%d, want=2", len(args))
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_write: argument 0 must be int (fd), got %s", args[0].Type())
	}

	dataObj, ok := args[1].(*object.Vec)
	if !ok {
		return newError("__builtin_socket_write: argument 1 must be Vec<byte, _>, got %s", args[1].Type())
	}

	connMu.Lock()
	conn, ok := activeConns[int(fdObj.Value)]
	connMu.Unlock()

	if !ok {
		return &object.Result{IsOk: false, Value: &object.String{Value: "invalid socket fd"}}
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("socket.write", runtimecap.FlagAllowNet)},
		}
	}

	// Convert Vec<int, _> to []byte
	bytes := make([]byte, len(dataObj.Elements))
	for i, el := range dataObj.Elements {
		if intVal, ok := el.(*object.Integer); ok {
			bytes[i] = byte(intVal.Value)
		} else {
			return &object.Result{IsOk: false, Value: &object.String{Value: "invalid data type in buffer"}}
		}
	}

	_, err := conn.Write(bytes)
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	return &object.Result{IsOk: true, Value: &object.Void{}}
}

func builtinSocketClose(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_socket_close: wrong number of arguments. got=%d, want=1", len(args))
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_close: argument 0 must be int (fd), got %s", args[0].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("socket.close", runtimecap.FlagAllowNet)},
		}
	}

	connMu.Lock()
	id := int(fdObj.Value)
	conn, connOK := activeConns[id]
	if connOK {
		delete(activeConns, id)
	}
	var listener net.Listener
	var listenerOK bool
	if !connOK {
		listener, listenerOK = listeners[id]
		if listenerOK {
			delete(listeners, id)
		}
	}
	connMu.Unlock()

	if !connOK && !listenerOK {
		// Already closed or invalid, maybe just return Ok? Or Error?
		// For safety, let's return Void (successful no-op) or Error if strict.
		// Returning error for strictness.
		return &object.Result{IsOk: false, Value: &object.String{Value: "invalid socket fd"}}
	}

	if connOK {
		if err := conn.Close(); err != nil {
			return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
		}
	}

	if listenerOK {
		if err := listener.Close(); err != nil {
			return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
		}
	}

	return &object.Result{IsOk: true, Value: &object.Void{}}
}

func builtinSocketConnectTLS(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__builtin_socket_connect_tls: wrong number of arguments. got=%d, want=2", len(args))
	}

	hostObj, ok := args[0].(*object.String)
	if !ok {
		return newError("__builtin_socket_connect_tls: argument 0 must be string (host), got %s", args[0].Type())
	}

	portObj, ok := args[1].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_connect_tls: argument 1 must be int (port), got %s", args[1].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("socket.connectTls", runtimecap.FlagAllowNet)},
		}
	}

	addr := fmt.Sprintf("%s:%d", hostObj.Value, portObj.Value)

	conn, err := tls.Dial("tcp", addr, nil)
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	connMu.Lock()
	id := nextConnID
	nextConnID++
	activeConns[id] = conn
	connMu.Unlock()

	return &object.Result{IsOk: true, Value: &object.Integer{Value: int64(id)}}
}

func builtinSocketSetTimeout(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__builtin_socket_set_timeout: wrong number of arguments. got=%d, want=2", len(args))
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_set_timeout: argument 0 must be int (fd), got %s", args[0].Type())
	}

	msObj, ok := args[1].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_set_timeout: argument 1 must be int (milliseconds), got %s", args[1].Type())
	}

	connMu.Lock()
	conn, ok := activeConns[int(fdObj.Value)]
	connMu.Unlock()

	if !ok {
		return &object.Result{IsOk: false, Value: &object.String{Value: "invalid socket fd"}}
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("socket.setTimeout", runtimecap.FlagAllowNet)},
		}
	}

	duration := time.Duration(msObj.Value) * time.Millisecond
	var err error
	if msObj.Value <= 0 {
		err = conn.SetDeadline(time.Time{}) // Clear timeout
	} else {
		err = conn.SetDeadline(time.Now().Add(duration))
	}

	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	return &object.Result{IsOk: true, Value: &object.Void{}}
}

func builtinSocketBind(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__builtin_socket_bind: wrong number of arguments. got=%d, want=2", len(args))
	}

	hostObj, ok := args[0].(*object.String)
	if !ok {
		return newError("__builtin_socket_bind: argument 0 must be string (host), got %s", args[0].Type())
	}

	portObj, ok := args[1].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_bind: argument 1 must be int (port), got %s", args[1].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("socket.bind", runtimecap.FlagAllowNet)},
		}
	}

	addr := fmt.Sprintf("%s:%d", hostObj.Value, portObj.Value)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	connMu.Lock()
	id := nextListenerID
	nextListenerID++
	listeners[id] = listener
	connMu.Unlock()

	return &object.Result{IsOk: true, Value: &object.Integer{Value: int64(id)}}
}

func builtinSocketAccept(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_socket_accept: wrong number of arguments. got=%d, want=1", len(args))
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_socket_accept: argument 0 must be int (listener_fd), got %s", args[0].Type())
	}

	if !runtimecap.Current().AllowNet {
		return &object.Result{
			IsOk:  false,
			Value: &object.String{Value: runtimecap.PermissionError("socket.accept", runtimecap.FlagAllowNet)},
		}
	}

	connMu.Lock()
	listener, ok := listeners[int(fdObj.Value)]
	connMu.Unlock()

	if !ok {
		return &object.Result{IsOk: false, Value: &object.String{Value: "invalid listener fd"}}
	}

	conn, err := listener.Accept()
	if err != nil {
		return &object.Result{IsOk: false, Value: &object.String{Value: err.Error()}}
	}

	connMu.Lock()
	id := nextConnID
	nextConnID++
	activeConns[id] = conn
	connMu.Unlock()

	return &object.Result{IsOk: true, Value: &object.Integer{Value: int64(id)}}
}

func builtinSleep(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_sleep: wrong number of arguments. got=%d, want=1", len(args))
	}
	ms, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_sleep: argument must be int (ms), got %s", args[0].Type())
	}
	time.Sleep(time.Duration(ms.Value) * time.Millisecond)
	return &object.Result{IsOk: true, Value: &object.Void{}}
}

func builtinTimeNow(args ...object.Object) object.Object {
	return &object.Integer{Value: time.Now().UnixNano()}
}

func builtinTimeParts(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_time_parts: wrong number of arguments. got=%d, want=1", len(args))
	}
	nanosObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_time_parts: argument must be int (nanos), got %s", args[0].Type())
	}
	tm := time.Unix(0, nanosObj.Value).UTC()
	year, month, day := tm.Date()
	hour, m, sec := tm.Clock()
	nsec := tm.Nanosecond()
	weekday := int(tm.Weekday())
	parts := []object.Object{
		&object.Integer{Value: int64(year)},
		&object.Integer{Value: int64(month)},
		&object.Integer{Value: int64(day)},
		&object.Integer{Value: int64(hour)},
		&object.Integer{Value: int64(m)},
		&object.Integer{Value: int64(sec)},
		&object.Integer{Value: int64(nsec)},
		&object.Integer{Value: int64(weekday)},
	}
	return &object.Vec{Elements: parts, ElemType: "int", Size: -1, Mutable: false}
}

func builtinMonotonicNow(args ...object.Object) object.Object {
	// Approximation for interpreter
	return &object.Integer{Value: time.Now().UnixNano()}
}

func builtinThreadID(args ...object.Object) object.Object {
	// Interpreter is currently single-threaded for evaluation
	return &object.Integer{Value: 0}
}

func builtinMutexNew(args ...object.Object) object.Object {
	mutexMu.Lock()
	defer mutexMu.Unlock()
	id := nextMutexID
	nextMutexID++
	mutexes[id] = &sync.Mutex{}
	return &object.Integer{Value: int64(id)}
}

func builtinAllocArray(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__alloc_array: wrong number of arguments. got=%d, want=2", len(args))
	}
	capObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__alloc_array: argument 0 must be int (capacity), got %s", args[0].Type())
	}
	capacity := int(capObj.Value)
	if capacity < 0 {
		return newError("__alloc_array: negative capacity")
	}

	dummy := args[1]

	elements := make([]object.Object, capacity)
	for i := range capacity {
		elements[i] = dummy
	}

	// Return as a fixed-size Vec (acting as __Array)
	return &object.Vec{
		Elements: elements,
		Size:     capacity,
		Mutable:  true,
		ElemType: string(dummy.Type()),
	}
}

// builtinVecAlloc: allocate an opaque Vec buffer (used as backing store)
func builtinVecAlloc(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__vec_alloc: wrong number of arguments. got=%d, want=2", len(args))
	}
	capObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__vec_alloc: argument 0 must be int (capacity), got %s", args[0].Type())
	}
	capacity := int(capObj.Value)
	if capacity < 0 {
		return newError("__vec_alloc: negative capacity")
	}

	dummy := args[1]

	elements := make([]object.Object, capacity)
	for i := range capacity {
		elements[i] = dummy
	}

	return &object.Vec{
		Elements: elements,
		Size:     capacity,
		Mutable:  true,
		ElemType: string(dummy.Type()),
	}
}

// builtinVecLen: returns the underlying buffer length (capacity for buffer Vec)
func builtinVecLen(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__vec_len: wrong number of arguments. got=%d, want=1", len(args))
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return newError("__vec_len: argument must be Vec, got %s", args[0].Type())
	}
	return &object.Integer{Value: int64(len(vec.Elements))}
}

// builtinVecCap: returns capacity of the underlying buffer
func builtinVecCap(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__vec_cap: wrong number of arguments. got=%d, want=1", len(args))
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return newError("__vec_cap: argument must be Vec, got %s", args[0].Type())
	}
	return &object.Integer{Value: int64(cap(vec.Elements))}
}

// builtinVecGet: returns element at index (no Option wrapper)
func builtinVecGet(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__vec_get: wrong number of arguments. got=%d, want=2", len(args))
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return newError("__vec_get: argument 0 must be Vec, got %s", args[0].Type())
	}
	idxObj, ok := args[1].(*object.Integer)
	if !ok {
		return newError("__vec_get: argument 1 must be int, got %s", args[1].Type())
	}
	idx := int(idxObj.Value)
	if idx < 0 || idx >= len(vec.Elements) {
		return &object.Null{}
	}
	return vec.Elements[idx]
}

// builtinVecSet: sets element at index
func builtinVecSet(args ...object.Object) object.Object {
	if len(args) != 3 {
		return newError("__vec_set: wrong number of arguments. got=%d, want=3", len(args))
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return newError("__vec_set: argument 0 must be Vec, got %s", args[0].Type())
	}
	idxObj, ok := args[1].(*object.Integer)
	if !ok {
		return newError("__vec_set: argument 1 must be int, got %s", args[1].Type())
	}
	idx := int(idxObj.Value)
	if idx < 0 || idx >= len(vec.Elements) {
		return newError("__vec_set: index out of range")
	}
	vec.Elements[idx] = args[2]
	return &object.Void{}
}

// builtinVecGrow: grow underlying buffer to at least newcap, returns new Vec buffer
func builtinVecGrow(args ...object.Object) object.Object {
	if len(args) != 2 {
		return newError("__vec_grow: wrong number of arguments. got=%d, want=2", len(args))
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return newError("__vec_grow: argument 0 must be Vec, got %s", args[0].Type())
	}
	capObj, ok := args[1].(*object.Integer)
	if !ok {
		return newError("__vec_grow: argument 1 must be int, got %s", args[1].Type())
	}
	newCap := int(capObj.Value)
	if newCap <= len(vec.Elements) {
		return vec
	}
	newElements := make([]object.Object, newCap)
	copy(newElements, vec.Elements)
	// fill rest with nil (Null)
	for i := len(vec.Elements); i < newCap; i++ {
		newElements[i] = &object.Null{}
	}
	return &object.Vec{
		Elements: newElements,
		Size:     newCap,
		Mutable:  true,
		ElemType: vec.ElemType,
	}
}

func builtinMutexLock(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_mutex_lock: wrong number of arguments. got=%d, want=1", len(args))
	}
	idObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_mutex_lock: argument must be int (id), got %s", args[0].Type())
	}
	mutexMu.Lock()
	mu, ok := mutexes[int(idObj.Value)]
	mutexMu.Unlock()
	if !ok {
		return newError("invalid mutex handle: %d", idObj.Value)
	}
	mu.Lock()
	return &object.Void{}
}

func builtinMutexUnlock(args ...object.Object) object.Object {
	if len(args) != 1 {
		return newError("__builtin_mutex_unlock: wrong number of arguments. got=%d, want=1", len(args))
	}
	idObj, ok := args[0].(*object.Integer)
	if !ok {
		return newError("__builtin_mutex_unlock: argument must be int (id), got %s", args[0].Type())
	}
	mutexMu.Lock()
	mu, ok := mutexes[int(idObj.Value)]
	mutexMu.Unlock()
	if !ok {
		return newError("invalid mutex handle: %d", idObj.Value)
	}
	mu.Unlock()
	return &object.Void{}
}
