package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

var (
	debugLogger *log.Logger
	infoLogger  *log.Logger
	warnLogger  *log.Logger
	errorLogger *log.Logger
	verbose     bool
)

// Level log level
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// Init initializes the logger
func Init() {
	verbose = os.Getenv("VERBOSE") == "true"

	debugLogger = log.New(os.Stdout, "\033[36m[DEBUG]\033[0m ", log.Ldate|log.Ltime|log.Lshortfile)
	infoLogger = log.New(os.Stdout, "\033[32m[INFO]\033[0m ", log.Ldate|log.Ltime)
	warnLogger = log.New(os.Stdout, "\033[33m[WARN]\033[0m ", log.Ldate|log.Ltime)
	errorLogger = log.New(os.Stderr, "\033[31m[ERROR]\033[0m ", log.Ldate|log.Ltime|log.Lshortfile)
}

// SetVerbose sets verbose mode
func SetVerbose(v bool) {
	verbose = v
}

// Debug logs debug messages
func Debug(format string, v ...interface{}) {
	if verbose {
		debugLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

// Info logs info messages
func Info(format string, v ...interface{}) {
	infoLogger.Output(2, fmt.Sprintf(format, v...))
}

// Warn logs warning messages
func Warn(format string, v ...interface{}) {
	warnLogger.Output(2, fmt.Sprintf(format, v...))
}

// Error logs error messages
func Error(format string, v ...interface{}) {
	errorLogger.Output(2, fmt.Sprintf(format, v...))
}

// Fatal logs fatal errors and exits
func Fatal(format string, v ...interface{}) {
	errorLogger.Output(2, fmt.Sprintf(format, v...))
	os.Exit(1)
}

// PrintInfo prints formatted information
func PrintInfo(title string, items map[string]string) {
	fmt.Println()
	fmt.Printf("\033[1;34m%s\033[0m\n", title)
	fmt.Println(strings.Repeat("-", 50))
	for k, v := range items {
		fmt.Printf("  %-20s: %s\n", k, v)
	}
	fmt.Println()
}

// TimeTrack tracks execution time
func TimeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	Info("%s completed in %v", name, elapsed)
}
