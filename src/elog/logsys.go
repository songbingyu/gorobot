//cretate by esrrhs
// change by bingyu.song , change from glog https://github.com/golang/glog/blob/master/glog.go
//TODO: must be improve

package elog

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SYS = iota
	ERROR
	DEBUG
	INFO
	NUM_LEVEL
)

var logLevelToName = []string{

	INFO:  "INFO",
	DEBUG: "DEBUG",
	ERROR: "ERROR",
	SYS:   "SYS",
}

const maxLogSize = 1024 * 1024 * 1024
const bufferSize = 256 * 1024
const logDir = "./log/"

type buffer struct {
	bytes.Buffer
	next *buffer
}

func (l *logSys) getBuffer() *buffer {

	l.freeListMu.Lock()
	b := l.freeList
	if b != nil {
		l.freeList = b.next
	}
	l.freeListMu.Unlock()

	if b == nil {
		b = new(buffer)
	} else {
		b.next = nil
		b.Reset()
	}

	return b
}

func (l *logSys) putBuffer(buf *buffer) {

	if buf.Len() > 256 {
		// big buffer is slow
		return
	}

	l.freeListMu.Lock()
	buf.next = l.freeList
	l.freeList = buf
	l.freeListMu.Unlock()

}

type logSys struct {
	logLevel   int
	toStderr   bool // The -logtostderr flag.
	mu         sync.Mutex
	files      [NUM_LEVEL]*fileBuffer
	freeList   *buffer
	freeListMu sync.Mutex
}

var logger logSys

func InitLog(level int) bool {

	//TODO: add log adjust
	logger.logLevel = level
	logger.initFiles()
	return true
}

func (l *logSys) print(level int, args ...interface{}) error {

	buf, err := l.genHeader(level)
	fmt.Fprint(buf, args...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	l.writeFile(level, buf)

	return err

}

func (l *logSys) println(level int, args ...interface{}) error {

	buf, err := l.genHeader(level)
	fmt.Fprintln(buf, args...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	l.writeFile(level, buf)

	return err

}

func (l *logSys) printf(level int, format string, a ...interface{}) error {

	buf, err := l.genHeader(level)
	fmt.Fprintf(buf, format, a...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	l.writeFile(level, buf)
	return err

}

func (l *logSys) genHeader(level int) (buf *buffer, err error) {

	//header

	header := "["
	header += logLevelToName[level] + "]"

	//TODO: glog is special ,Format maybe slow ?
	header += "[" + time.Now().Format("2006-01-02 15:04:05" ) + " "

	_, file, line, ok := runtime.Caller(3)
	if !ok {
		file = "god.."
		line = 1
	} else {
		slash := strings.LastIndex(file, "/")
		if slash >= 0 {
			file = file[slash+1:]
		}
	}
	if line < 0 {
		line = 0 // not a real line number, but acceptable to someDigits
	}
	header += (" " + file + ":" + strconv.Itoa(line) + "]:")

	// add buffer pool
	buf = l.getBuffer()
	buf.WriteString(header)
	return buf, nil

}

func (l *logSys) initFiles() error {

	now := time.Now()
	for i := SYS; i < NUM_LEVEL; i++ {

		if l.files[i] == nil {

			fb := &fileBuffer{
				logger: l,
				level:  i,
			}

			if err := fb.rotateFile(now); err != nil {

				return err
			}
			l.files[i] = fb
		}
	}

	return nil

}

func (l *logSys) createFile(logLevel string, t time.Time) (f *os.File, err error) {

	name := fmt.Sprintf("%s-%d-%d-%d-%d.log",
		logLevel,
		t.Year(),
		t.Month(),
		t.Day(),
		t.Hour())
	os.Mkdir(logDir, os.ModePerm)
	pathName := logDir + name
	f, err = os.OpenFile(pathName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 666)
	return
}

func (l *logSys) writeFile(level int, buf *buffer) error {

	l.mu.Lock()
	data := buf.Bytes()
	if l.toStderr {
		os.Stderr.Write(data)
	} else {

		if l.files[level] == nil {
			if ret := l.initFiles(); ret != nil {
				l.exit(ret)
			}
		}

		l.files[level].write(data)
		os.Stderr.Write(data)
	}

	l.putBuffer(buf)
	l.mu.Unlock()

	return nil
}

func (l *logSys) lockAndFlushAll() {

	l.mu.Lock()
	l.flushAll()
	l.mu.Unlock()

}

func (l *logSys) flushAll() {

	for i := SYS; i < NUM_LEVEL; i++ {

		file := l.files[i]
		if file != nil {

			file.Flush()
			file.sync()
		}
	}
}

func (l *logSys) exit(err error) {

	//TODO : add some function on log exit
	fmt.Fprintf(os.Stderr, "log: exiting because of error: %s\n", err)
	l.flushAll()
	os.Exit(2)
}

//IO buffer  cache
type fileBuffer struct {
	logger *logSys
	*bufio.Writer
	file   *os.File
	level  int
	nbytes uint64
}

func (fb *fileBuffer) sync() error {

	return fb.file.Sync()
}

func (fb *fileBuffer) write(p []byte) (n int, err error) {

	if fb.nbytes+(uint64)(len(p)) > maxLogSize {

		if err := fb.rotateFile(time.Now()); err != nil {

			fb.logger.exit(err)
		}
	}

	n, err = fb.Writer.Write(p)
	fb.nbytes += uint64(n)
	if err != nil {
		fb.logger.exit(err)
	}
	return
}

func (fb *fileBuffer) rotateFile(now time.Time) error {

	if fb.file != nil {

		fb.Flush()
		fb.file.Close()
	}

	var err error
	fb.file, err = fb.logger.createFile(logLevelToName[fb.level], now)
	fb.nbytes = 0
	if err != nil {
		return err
	}

	fb.Writer = bufio.NewWriterSize(fb.file, bufferSize)

	// Write header.
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Log file created at: %s\n", now.Format("2006/01/02 15:04:05"))
	fmt.Fprintf(&buf, "Binary: Built with %s %s for %s/%s\n", runtime.Compiler, runtime.Version(), runtime.GOOS, runtime.GOARCH)

	n, err := fb.file.Write(buf.Bytes())
	fb.nbytes += uint64(n)
	return err
}

func Flush() {

	logger.lockAndFlushAll()
}

//Fixme:consider add println  print style interface
func LogInfo(format string, a ...interface{}) {
	if logger.logLevel >= INFO {
		logger.printf(INFO, format, a...)
	}
}

func LogInfoln(a ...interface{}) {
	if logger.logLevel >= INFO {
		logger.println(INFO, a...)
	}
}

func LogDebug(format string, a ...interface{}) {
	if logger.logLevel >= DEBUG {
		logger.printf(DEBUG, format, a...)
	}
}

func LogDebugln(a ...interface{}) {
	if logger.logLevel >= DEBUG {
		logger.println(DEBUG, a...)
	}
}

func LogError(format string, a ...interface{}) {
	if logger.logLevel >= ERROR {
		logger.printf(ERROR, format, a...)
	}
}

func LogErrorln(a ...interface{}) {
	if logger.logLevel >= ERROR {
		logger.println(ERROR, a...)
	}
}

func LogSys(format string, a ...interface{}) {
	if logger.logLevel >= SYS {
		logger.printf(SYS, format, a...)
	}
}

func LogSysln(a ...interface{}) {
	if logger.logLevel >= SYS {
		logger.println(SYS, a...)
	}
}
