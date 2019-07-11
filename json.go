package log

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"
)

var DefaultLogger = Logger{
	Level:      DebugLevel,
	Caller:     false,
	EscapeHTML: false,
	TimeField:  "",
	TimeFormat: "",
	Writer:     &Writer{},
}

type Logger struct {
	Level      Level
	Caller     bool
	EscapeHTML bool
	TimeField  string
	TimeFormat string
	Writer     io.Writer
}

type Event struct {
	buf        []byte
	fatal      bool
	escapeHTML bool
	timeFormat string
	write      func(p []byte) (n int, err error)
}

func Debug() *Event {
	return DefaultLogger.WithLevel(DebugLevel)
}

func Info() *Event {
	return DefaultLogger.WithLevel(InfoLevel)
}

func Warn() *Event {
	return DefaultLogger.WithLevel(WarnLevel)
}

func Error() *Event {
	return DefaultLogger.WithLevel(ErrorLevel)
}

func Fatal() *Event {
	return DefaultLogger.WithLevel(FatalLevel)
}

func (l Logger) Debug() *Event {
	return l.WithLevel(DebugLevel)
}

func (l Logger) Info() *Event {
	return l.WithLevel(InfoLevel)
}

func (l Logger) Warn() *Event {
	return l.WithLevel(WarnLevel)
}

func (l Logger) Error() *Event {
	return l.WithLevel(ErrorLevel)
}

func (l Logger) Fatal() *Event {
	return l.WithLevel(FatalLevel)
}

var epool = sync.Pool{
	New: func() interface{} {
		return new(Event)
	},
}

func (l Logger) WithLevel(level Level) (e *Event) {
	if level < l.Level {
		return
	}
	e = epool.Get().(*Event)
	e.buf = e.buf[:0]
	e.fatal = level == FatalLevel
	e.escapeHTML = l.EscapeHTML
	e.timeFormat = l.TimeFormat
	e.write = l.Writer.Write
	// time
	now := timeNow()
	if l.TimeField == "" {
		e.buf = append(e.buf, "{\"time\":"...)
	} else {
		e.key('{', l.TimeField)
	}
	if e.timeFormat == "" {
		e.time(now)
	} else {
		e.buf = append(e.buf, '"')
		e.buf = now.AppendFormat(e.buf, e.timeFormat)
		e.buf = append(e.buf, '"')
	}
	// level
	switch level {
	case DebugLevel:
		e.buf = append(e.buf, ",\"level\":\"debug\""...)
	case InfoLevel:
		e.buf = append(e.buf, ",\"level\":\"info\""...)
	case WarnLevel:
		e.buf = append(e.buf, ",\"level\":\"warn\""...)
	case ErrorLevel:
		e.buf = append(e.buf, ",\"level\":\"error\""...)
	case FatalLevel:
		e.buf = append(e.buf, ",\"level\":\"fatal\""...)
	}
	// caller
	if l.Caller {
		e.caller(1)
	}
	return
}

func (e *Event) Time(key string, t time.Time) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	switch {
	case e.timeFormat != "":
		e.buf = append(e.buf, '"')
		e.buf = t.AppendFormat(e.buf, e.timeFormat)
		e.buf = append(e.buf, '"')
	default:
		e.time(t)
	}
	return e
}

func (e *Event) Timestamp() *Event {
	if e == nil {
		return nil
	}
	e.key(',', "timestamp")
	e.buf = strconv.AppendInt(e.buf, timeNow().Unix(), 10)
	return e
}

func (e *Event) Bool(key string, b bool) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = strconv.AppendBool(e.buf, b)
	return e
}

func (e *Event) Bools(key string, b []bool) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = append(e.buf, '[')
	for i, a := range b {
		if i != 0 {
			e.buf = append(e.buf, ',')
		}
		e.buf = strconv.AppendBool(e.buf, a)
	}
	e.buf = append(e.buf, ']')
	return e
}

func (e *Event) Dur(key string, d time.Duration) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = append(e.buf, '"')
	e.buf = append(e.buf, d.String()...)
	e.buf = append(e.buf, '"')
	return e
}

func (e *Event) Durs(key string, d []time.Duration) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = append(e.buf, '[')
	for i, a := range d {
		if i != 0 {
			e.buf = append(e.buf, ',')
		}
		e.buf = append(e.buf, '"')
		e.buf = append(e.buf, a.String()...)
		e.buf = append(e.buf, '"')
	}
	e.buf = append(e.buf, ']')
	return e
}

func (e *Event) Err(err error) *Event {
	if e == nil {
		return nil
	}
	if err == nil {
		e.buf = append(e.buf, ",\"error\":null"...)
	} else {
		e.buf = append(e.buf, ",\"error\":"...)
		e.string(err.Error(), e.escapeHTML)
	}
	return e
}

func (e *Event) Errs(key string, errs []error) *Event {
	if e == nil {
		return nil
	}

	e.key(',', key)
	e.buf = append(e.buf, '[')
	for i, err := range errs {
		if i != 0 {
			e.buf = append(e.buf, ',')
		}
		if err == nil {
			e.buf = append(e.buf, "null"...)
		} else {
			e.string(err.Error(), e.escapeHTML)
		}
	}
	e.buf = append(e.buf, ']')
	return e
}

func (e *Event) Float64(key string, f float64) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = strconv.AppendFloat(e.buf, f, 'f', -1, 64)
	return e
}

func (e *Event) Floats64(key string, f []float64) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = append(e.buf, '[')
	for i, a := range f {
		if i != 0 {
			e.buf = append(e.buf, ',')
		}
		e.buf = strconv.AppendFloat(e.buf, a, 'f', -1, 64)
	}
	e.buf = append(e.buf, ']')
	return e
}

func (e *Event) Floats32(key string, f []float32) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = append(e.buf, '[')
	for i, a := range f {
		if i != 0 {
			e.buf = append(e.buf, ',')
		}
		e.buf = strconv.AppendFloat(e.buf, float64(a), 'f', -1, 64)
	}
	e.buf = append(e.buf, ']')
	return e
}

func (e *Event) Int64(key string, i int64) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = strconv.AppendInt(e.buf, i, 10)
	return e
}

func (e *Event) Uint64(key string, i uint64) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = strconv.AppendUint(e.buf, i, 10)
	return e
}

func (e *Event) Float32(key string, f float32) *Event {
	return e.Float64(key, float64(f))
}

func (e *Event) Int(key string, i int) *Event {
	return e.Int64(key, int64(i))
}

func (e *Event) Int32(key string, i int32) *Event {
	return e.Int64(key, int64(i))
}

func (e *Event) Int16(key string, i int16) *Event {
	return e.Int64(key, int64(i))
}

func (e *Event) Int8(key string, i int8) *Event {
	return e.Int64(key, int64(i))
}

func (e *Event) Uint32(key string, i uint32) *Event {
	return e.Uint64(key, uint64(i))
}

func (e *Event) Uint16(key string, i uint16) *Event {
	return e.Uint64(key, uint64(i))
}

func (e *Event) Uint8(key string, i uint8) *Event {
	return e.Uint64(key, uint64(i))
}

func (e *Event) RawJSON(key string, b []byte) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = append(e.buf, b...)
	return e
}

func (e *Event) Str(key string, val string) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.string(val, e.escapeHTML)
	return e
}

func (e *Event) Strs(key string, vals []string) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.buf = append(e.buf, '[')
	for i, val := range vals {
		if i != 0 {
			e.buf = append(e.buf, ',')
		}
		e.string(val, e.escapeHTML)
	}
	e.buf = append(e.buf, ']')
	return e
}

func (e *Event) Bytes(key string, val []byte) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	e.string(*(*string)(unsafe.Pointer(&val)), e.escapeHTML)
	return e
}

func (e *Event) Interface(key string, i interface{}) *Event {
	if e == nil {
		return nil
	}
	e.key(',', key)
	marshaled, err := json.Marshal(i)
	if err != nil {
		e.string("marshaling error: "+err.Error(), e.escapeHTML)
	} else {
		e.string(*(*string)(unsafe.Pointer(&marshaled)), e.escapeHTML)
	}
	return e
}

func (e *Event) Caller() *Event {
	if e == nil {
		return nil
	}
	e.caller(0)
	return e
}

func (e *Event) Send() {
	if e == nil {
		return
	}
	e.buf = append(e.buf, '}', '\n')
	e.write(e.buf)
	if e.fatal {
		e.write(stacks(false))
		e.write(stacks(true))
		os.Exit(255)
	}
	epool.Put(e)
}

func (e *Event) Msg(msg string) {
	if e == nil {
		return
	}
	e.buf = append(e.buf, ",\"message\":"...)
	e.string(msg, e.escapeHTML)
	e.buf = append(e.buf, '}', '\n')
	e.write(e.buf)
	if e.fatal {
		e.write(stacks(false))
		e.write(stacks(true))
		os.Exit(255)
	}
	epool.Put(e)
}

func (e *Event) Msgf(format string, v ...interface{}) {
	e.Msg(fmt.Sprintf(format, v...))
}

func (e *Event) key(b byte, key string) {
	e.buf = append(e.buf, b, '"')
	e.buf = append(e.buf, key...)
	e.buf = append(e.buf, '"', ':')
}

func (e *Event) time(now time.Time) {
	now = now.UTC()
	n := len(e.buf)
	e.buf = append(e.buf, "\"2006-01-02T15:04:05.999Z\""...)
	var a, b int
	// year
	a = now.Year()
	b = a / 10
	e.buf[n+4] = byte('0' + a - 10*b)
	a = b
	b = a / 10
	e.buf[n+3] = byte('0' + a - 10*b)
	a = b
	b = a / 10
	e.buf[n+2] = byte('0' + a - 10*b)
	e.buf[n+1] = byte('0' + b)
	// month
	a = int(now.Month())
	b = a / 10
	e.buf[n+7] = byte('0' + a - 10*b)
	e.buf[n+6] = byte('0' + b)
	// day
	a = now.Day()
	b = a / 10
	e.buf[n+10] = byte('0' + a - 10*b)
	e.buf[n+9] = byte('0' + b)
	// hour
	a = now.Hour()
	b = a / 10
	e.buf[n+13] = byte('0' + a - 10*b)
	e.buf[n+12] = byte('0' + b)
	// minute
	a = now.Minute()
	b = a / 10
	e.buf[n+16] = byte('0' + a - 10*b)
	e.buf[n+15] = byte('0' + b)
	// second
	a = now.Second()
	b = a / 10
	e.buf[n+19] = byte('0' + a - 10*b)
	e.buf[n+18] = byte('0' + b)
	// milli second
	a = now.Nanosecond() / 1000000
	b = a / 10
	e.buf[n+23] = byte('0' + a - 10*b)
	a = b
	b = a / 10
	e.buf[n+22] = byte('0' + a - 10*b)
	e.buf[n+21] = byte('0' + b)
}

func (e *Event) caller(skip int) {
	_, file, line, ok := runtime.Caller(2 + skip)
	if !ok {
		file = "???"
		line = 1
	} else {
		if i := strings.LastIndex(file, "/"); i >= 0 {
			file = file[i+1:]
		}
	}
	if line < 0 {
		line = 0
	}
	e.buf = append(e.buf, ",\"caller\":\""...)
	e.buf = append(e.buf, file...)
	e.buf = append(e.buf, ':')
	e.buf = strconv.AppendInt(e.buf, int64(line), 10)
	e.buf = append(e.buf, '"')
}

var hex = "0123456789abcdef"

// https://golang.org/src/encoding/json/encode.go
func (e *Event) string(value string, escapeHTML bool) {
	e.buf = append(e.buf, '"')
	start := 0
	for i := 0; i < len(value); {
		if b := value[i]; b < utf8.RuneSelf {
			if htmlSafeSet[b] || (!escapeHTML && safeSet[b]) {
				i++
				continue
			}
			if start < i {
				e.buf = append(e.buf, value[start:i]...)
			}
			switch b {
			case '\\', '"':
				e.buf = append(e.buf, '\\', b)
			case '\n':
				e.buf = append(e.buf, '\\', 'n')
			case '\r':
				e.buf = append(e.buf, '\\', 'r')
			case '\t':
				e.buf = append(e.buf, '\\', 't')
			default:
				// This encodes bytes < 0x20 except for \t, \n and \r.
				// If escapeHTML is set, it also escapes <, >, and &
				// because they can lead to security holes when
				// user-controlled strings are rendered into JSON
				// and served to some browsers.
				e.buf = append(e.buf, '\\', 'u', '0', '0', hex[b>>4], hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(value[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				e.buf = append(e.buf, value[start:i]...)
			}
			e.buf = append(e.buf, '\\', 'u', 'f', 'f', 'f', 'd')
			i += size
			start = i
			continue
		}
		// U+2028 is LINE SEPARATOR.
		// U+2029 is PARAGRAPH SEPARATOR.
		// They are both technically valid characters in JSON strings,
		// but don't work in JSONP, which has to be evaluated as JavaScript,
		// and can lead to security holes there. It is valid JSON to
		// escape them, so we do so unconditionally.
		// See http://timelessrepo.com/json-isnt-a-javascript-subset for discussion.
		if c == '\u2028' || c == '\u2029' {
			if start < i {
				e.buf = append(e.buf, value[start:i]...)
			}
			e.buf = append(e.buf, '\\', 'u', '2', '0', '2', hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(value) {
		e.buf = append(e.buf, value[start:]...)
	}
	e.buf = append(e.buf, '"')
}

// safeSet holds the value true if the ASCII character with the given array
// position can be represented inside a JSON string without any further
// escaping.
//
// All values are true except for the ASCII control characters (0-31), the
// double quote ("), and the backslash character ("\").
var safeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      true,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      true,
	'=':      true,
	'>':      true,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}

// htmlSafeSet holds the value true if the ASCII character with the given
// array position can be safely represented inside a JSON string, embedded
// inside of HTML <script> tags, without any additional escaping.
//
// All values are true except for the ASCII control characters (0-31), the
// double quote ("), the backslash character ("\"), HTML opening and closing
// tags ("<" and ">"), and the ampersand ("&").
var htmlSafeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      false,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      false,
	'=':      true,
	'>':      false,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}

// stacks is a wrapper for runtime.Stack that attempts to recover the data for all goroutines.
func stacks(all bool) []byte {
	// We don't know how big the traces are, so grow a few times if they don't fit. Start large, though.
	n := 10000
	if all {
		n = 100000
	}
	var trace []byte
	for i := 0; i < 5; i++ {
		trace = make([]byte, n)
		nbytes := runtime.Stack(trace, all)
		if nbytes < len(trace) {
			return trace[:nbytes]
		}
		n *= 2
	}
	return trace
}
