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
		return argCountError("concat", len(args), "at least 1")
	}

	var out strings.Builder

	for _, arg := range args {
		ch, ok := arg.(*object.String)
		if !ok {
			return newError("fromChars: element is not char")
		}

		out.WriteString(ch.Value)
	}

	return object.NewString(out.String())
}

func builtinCfg(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("cfg", len(args), "1")
	}
	featureName, ok := args[0].(*object.String)
	if !ok {
		return argTypeError("cfg", args[0], "string")
	}
	return object.NewBool(runtimecap.CurrentFeatureEnabled(featureName.Value))
}

// builtinFromChars: primitive to convert Vec<char, _> to string
func builtinFromChars(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("fromChars", len(args), "1")
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return argTypeError("fromChars", args[0], "Vec<char, _>")
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
	return object.NewString(string(runes))
}

// builtinStringFromBytes: optimized string construction from Vec<int, _> bytes
func builtinStringFromBytes(args ...object.Object) object.Object {
	if len(args) != 3 {
		return argCountError("", len(args), "3")
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
		return argTypeError("", args[1], "start argument must be int")
	}
	end, ok := args[2].(*object.Integer)
	if !ok {
		return argTypeError("", args[2], "end argument must be int")
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
	return object.NewString(builder.String())
}

// builtinStringPtr: mocked for evaluator - not meant to be used literally there, but registered.
func builtinStringPtr(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("", len(args), "1")
	}
	return object.NewInteger(0)
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
	return object.NewVoid()
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
	return object.NewVoid()
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
	return object.NewVoid()
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
	return object.NewVoid()
}

func builtinType(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("", len(args), "1")
	}

	return object.NewString(string(args[0].Type()))
}

func builtinInt(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("", len(args), "1")
	}

	switch arg := args[0].(type) {
	case *object.Integer:
		return arg
	case *object.Float:
		return &object.Integer{
			Value: int64(arg.Value),
		}
	case *object.String:
		var val int64
		_, err := fmt.Sscanf(arg.Value, "%d", &val)
		if err != nil {
			return resultErrString("invalid integer")
		}

		return resultOkInt(val)
	case *object.Boolean:
		if arg.Value {
			return object.NewInteger(1)
		}
		return object.NewInteger(0)
	default:
		return newError("cannot convert %s to int", args[0].Type())
	}
}

func builtinFloat(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("", len(args), "1")
	}

	switch arg := args[0].(type) {
	case *object.Float:
		return arg
	case *object.Integer:
		return object.NewFloat(float64(arg.Value))
	case *object.String:
		var val float64
		_, err := fmt.Sscanf(arg.Value, "%f", &val)
		if err != nil {
			return resultErrString("invalid float")
		}
		return resultOk(object.NewFloat(val))
	default:
		return newError("cannot convert %s to float", args[0].Type())
	}
}

func builtinString(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("", len(args), "1")
	}

	return object.NewString(args[0].Inspect())
}

// builtinChar creates a character from an ASCII code
func builtinChar(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("", len(args), "1")
	}

	switch arg := args[0].(type) {
	case *object.Integer:
		return object.NewChar(rune(arg.Value))
	case *object.Char:
		return arg
	default:
		return argTypeError("char", args[0], "INTEGER")
	}
}

func builtinSocketConnect(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("__builtin_socket_connect", len(args), "2")
	}

	hostObj, ok := args[0].(*object.String)
	if !ok {
		return nthArgTypeError("__builtin_socket_connect", 0, args[0], "string")
	}
	portObj, ok := args[1].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_connect", 1, args[1], "int")
	}

	if !runtimecap.Current().AllowNet {
		return resultErrString(runtimecap.PermissionError("socket.connect", runtimecap.FlagAllowNet))
	}

	conn, err := net.DialTimeout(
		"tcp",
		fmt.Sprintf(
			"%s:%d",
			hostObj.Value,
			portObj.Value,
		),
		10*time.Second,
	)
	if err != nil {
		return resultErr(err)
	}

	connMu.Lock()
	id := nextConnID
	nextConnID++
	activeConns[id] = conn
	connMu.Unlock()

	return resultOkInt(int64(id))
}

func builtinSocketRead(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("__builtin_socket_read", len(args), "2")
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_read", 0, args[0], "int (fd)")
	}
	nObj, ok := args[1].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_read", 1, args[1], "int (n)")
	}

	connMu.Lock()
	conn, ok := activeConns[int(fdObj.Value)]
	connMu.Unlock()

	if !ok {
		return resultErrString("invalid socket fd")
	}

	if !runtimecap.Current().AllowNet {
		return resultErrString(runtimecap.PermissionError("socket.read", runtimecap.FlagAllowNet))
	}

	if nObj.Value < 0 {
		return resultErrString("socket read count must be non-negative")
	}

	buf := make([]byte, int(nObj.Value))
	n, err := conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			// Return empty Vec on EOF
			return resultOk(&object.Vec{
				Elements: []object.Object{},
				Size:     -1,
				Mutable:  true,
				ElemType: "byte",
			})
		}
		if n == 0 {
			return resultErr(err)
		}
		// If partial read, that's fine.
	}

	// Create Vec<byte, _> (which is Vec<int, _> in Bak runtime usually, unless we use byte array optimization?)
	// Bak currently uses Vec<Object, _> internally, so it's Vec<Integer, _>.
	// This is inefficient but consistent.
	elements := make([]object.Object, n)
	for i := range n {
		elements[i] = object.NewInteger(int64(buf[i]))
	}
	vec := &object.Vec{
		Elements: elements,
		Size:     -1,
		Mutable:  true,
		ElemType: "byte",
	}

	return resultOk(vec)
}

func builtinSocketWrite(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("__builtin_socket_write", len(args), "2")
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_write", 0, args[0], "int (fd)")
	}

	dataObj, ok := args[1].(*object.Vec)
	if !ok {
		return nthArgTypeError("__builtin_socket_write", 1, args[1], "Vec<byte, _>")
	}

	connMu.Lock()
	conn, ok := activeConns[int(fdObj.Value)]
	connMu.Unlock()

	if !ok {
		return resultErrString("invalid socket fd")
	}

	if !runtimecap.Current().AllowNet {
		return resultErrString(runtimecap.PermissionError("socket.write", runtimecap.FlagAllowNet))
	}

	// Convert Vec<int, _> to []byte
	bytes := make([]byte, len(dataObj.Elements))
	for i, el := range dataObj.Elements {
		if intVal, ok := el.(*object.Integer); ok {
			bytes[i] = byte(intVal.Value)
		} else {
			return resultErrString("invalid data type in buffer")
		}
	}

	_, err := conn.Write(bytes)
	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

func builtinSocketClose(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("__builtin_socket_close", len(args), "1")
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_close", 0, args[0], "int (fd)")
	}

	if !runtimecap.Current().AllowNet {
		return resultErrString(runtimecap.PermissionError(
			"socket.close",
			runtimecap.FlagAllowNet,
		),
		)
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
		return resultErrString("invalid socket fd")
	}

	if connOK {
		if err := conn.Close(); err != nil {
			return resultErr(err)
		}
	}

	if listenerOK {
		if err := listener.Close(); err != nil {
			return resultErr(err)
		}
	}

	return resultOkVoid()
}

func builtinSocketConnectTLS(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("__builtin_socket_connect_tls", len(args), "2")
	}

	hostObj, ok := args[0].(*object.String)
	if !ok {
		return nthArgTypeError("__builtin_socket_connect_tls", 0, args[0], "string (host)")
	}

	portObj, ok := args[1].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_connect_tls", 1, args[1], "int (port)")
	}

	if !runtimecap.Current().AllowNet {
		return resultErrString(runtimecap.PermissionError(
			"socket.connectTls",
			runtimecap.FlagAllowNet,
		),
		)
	}

	addr := fmt.Sprintf("%s:%d", hostObj.Value, portObj.Value)

	conn, err := tls.Dial("tcp", addr, nil)
	if err != nil {
		return resultErr(err)
	}

	connMu.Lock()
	id := nextConnID
	nextConnID++
	activeConns[id] = conn
	connMu.Unlock()

	return resultOkInt(int64(id))
}

func builtinSocketSetTimeout(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("__builtin_socket_set_timeout", len(args), "2")
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_set_timeout", 0, args[0], "int (fd)")
	}

	msObj, ok := args[1].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_set_timeout", 1, args[1], "int (milliseconds)")
	}

	connMu.Lock()
	conn, ok := activeConns[int(fdObj.Value)]
	connMu.Unlock()

	if !ok {
		return resultErrString("invalid socket fd")
	}

	if !runtimecap.Current().AllowNet {
		return resultErrString(runtimecap.PermissionError(
			"socket.setTimeout",
			runtimecap.FlagAllowNet,
		),
		)
	}

	duration := time.Duration(msObj.Value) * time.Millisecond
	var err error
	if msObj.Value <= 0 {
		err = conn.SetDeadline(time.Time{}) // Clear timeout
	} else {
		err = conn.SetDeadline(time.Now().Add(duration))
	}

	if err != nil {
		return resultErr(err)
	}

	return resultOkVoid()
}

func builtinSocketBind(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("__builtin_socket_bind", len(args), "2")
	}

	hostObj, ok := args[0].(*object.String)
	if !ok {
		return nthArgTypeError("__builtin_socket_bind", 0, args[0], "string (host)")
	}

	portObj, ok := args[1].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_bind", 1, args[1], "int (port)")
	}

	if !runtimecap.Current().AllowNet {
		return resultErrString(runtimecap.PermissionError(
			"socket.bind",
			runtimecap.FlagAllowNet,
		),
		)
	}

	addr := fmt.Sprintf("%s:%d", hostObj.Value, portObj.Value)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return resultErr(err)
	}

	connMu.Lock()
	id := nextListenerID
	nextListenerID++
	listeners[id] = listener
	connMu.Unlock()

	return resultOkInt(int64(id))
}

func builtinSocketAccept(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("__builtin_socket_accept", len(args), "1")
	}

	fdObj, ok := args[0].(*object.Integer)
	if !ok {
		return nthArgTypeError("__builtin_socket_accept", 0, args[0], "int (listener_fd)")
	}

	if !runtimecap.Current().AllowNet {
		return resultErrString(runtimecap.PermissionError(
			"socket.accept",
			runtimecap.FlagAllowNet,
		),
		)
	}

	connMu.Lock()
	listener, ok := listeners[int(fdObj.Value)]
	connMu.Unlock()

	if !ok {
		return resultErrString("invalid listener fd")
	}

	conn, err := listener.Accept()
	if err != nil {
		return resultErr(err)
	}

	connMu.Lock()
	id := nextConnID
	nextConnID++
	activeConns[id] = conn
	connMu.Unlock()

	return resultOkInt(int64(id))
}

func builtinSleep(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("__builtin_sleep", len(args), "1")
	}
	ms, ok := args[0].(*object.Integer)
	if !ok {
		return argTypeError("__builtin_sleep", args[0], "int (ms)")
	}

	time.Sleep(time.Duration(ms.Value) * time.Millisecond)

	return resultOkVoid()
}

func builtinTimeNow(args ...object.Object) object.Object {
	return object.NewInteger(time.Now().UnixNano())
}

func builtinTimeParts(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("__builtin_time_parts", len(args), "1")
	}
	nanosObj, ok := args[0].(*object.Integer)
	if !ok {
		return argTypeError("__builtin_time_parts", args[0], "int (nanos)")
	}

	tm := time.Unix(0, nanosObj.Value).UTC()
	year, month, day := tm.Date()
	hour, m, sec := tm.Clock()
	nsec := tm.Nanosecond()
	weekday := int(tm.Weekday())

	parts := []object.Object{
		object.NewInteger(int64(year)),
		object.NewInteger(int64(month)),
		object.NewInteger(int64(day)),
		object.NewInteger(int64(hour)),
		object.NewInteger(int64(m)),
		object.NewInteger(int64(sec)),
		object.NewInteger(int64(nsec)),
		object.NewInteger(int64(weekday)),
	}

	return &object.Vec{
		Elements: parts,
		ElemType: "int",
		Size:     -1,
		Mutable:  false,
	}
}

func builtinMonotonicNow(args ...object.Object) object.Object {
	// Approximation for interpreter
	return object.NewInteger(time.Now().UnixNano())
}

func builtinThreadID(args ...object.Object) object.Object {
	// Interpreter is currently single-threaded for evaluation
	return object.NewInteger(0)
}

func builtinMutexNew(args ...object.Object) object.Object {
	mutexMu.Lock()
	defer mutexMu.Unlock()
	id := nextMutexID
	nextMutexID++
	mutexes[id] = &sync.Mutex{}
	return object.NewInteger(int64(id))
}

func builtinAllocArray(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("__alloc_array", len(args), "2")
	}
	capObj, ok := args[0].(*object.Integer)
	if !ok {
		return nthArgTypeError("__alloc_array", 0, args[0], "int (capacity)")
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
		return argCountError("__vec_alloc", len(args), "2")
	}
	capObj, ok := args[0].(*object.Integer)
	if !ok {
		return nthArgTypeError("__vec_alloc", 0, args[0], "int (capacity)")
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
		return argCountError("__vec_len", len(args), "1")
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return argTypeError("__vec_len", args[0], "Vec")
	}
	return object.NewInteger(int64(len(vec.Elements)))
}

// builtinVecCap: returns capacity of the underlying buffer
func builtinVecCap(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("__vec_cap", len(args), "1")
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return argTypeError("__vec_cap", args[0], "Vec")
	}
	return object.NewInteger(int64(cap(vec.Elements)))
}

// builtinVecGet: returns element at index (no Option wrapper)
func builtinVecGet(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("__vec_get", len(args), "2")
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return nthArgTypeError("__vec_get", 0, args[0], "Vec")
	}
	idxObj, ok := args[1].(*object.Integer)
	if !ok {
		return nthArgTypeError("__vec_get", 1, args[1], "int")
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
		return argCountError("__vec_set", len(args), "3")
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return nthArgTypeError("__vec_set", 0, args[0], "Vec")
	}
	idxObj, ok := args[1].(*object.Integer)
	if !ok {
		return nthArgTypeError("__vec_set", 1, args[1], "int")
	}
	idx := int(idxObj.Value)
	if idx < 0 || idx >= len(vec.Elements) {
		return newError("__vec_set: index out of range")
	}
	vec.Elements[idx] = args[2]
	return object.NewVoid()
}

// builtinVecGrow: grow underlying buffer to at least newcap, returns new Vec buffer
func builtinVecGrow(args ...object.Object) object.Object {
	if len(args) != 2 {
		return argCountError("__vec_grow", len(args), "2")
	}
	vec, ok := args[0].(*object.Vec)
	if !ok {
		return nthArgTypeError("__vec_grow", 0, args[0], "Vec")
	}
	capObj, ok := args[1].(*object.Integer)
	if !ok {
		return nthArgTypeError("__vec_grow", 1, args[1], "int")
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
		return argCountError("__builtin_mutex_lock", len(args), "1")
	}
	idObj, ok := args[0].(*object.Integer)
	if !ok {
		return argTypeError("__builtin_mutex_lock", args[0], "int (id)")
	}
	mutexMu.Lock()
	mu, ok := mutexes[int(idObj.Value)]
	mutexMu.Unlock()
	if !ok {
		return newError("invalid mutex handle: %d", idObj.Value)
	}
	mu.Lock()
	return object.NewVoid()
}

func builtinMutexUnlock(args ...object.Object) object.Object {
	if len(args) != 1 {
		return argCountError("__builtin_mutex_unlock", len(args), "1")
	}
	idObj, ok := args[0].(*object.Integer)
	if !ok {
		return argTypeError("__builtin_mutex_unlock", args[0], "int (id)")
	}
	mutexMu.Lock()
	mu, ok := mutexes[int(idObj.Value)]
	mutexMu.Unlock()
	if !ok {
		return newError("invalid mutex handle: %d", idObj.Value)
	}
	mu.Unlock()
	return object.NewVoid()
}
