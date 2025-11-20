package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

type (
	LoggerMode string
	Loggers    struct {
		DB   *logrus.Logger
		HTTP *logrus.Logger
	}
)

const (
	JSONMode LoggerMode = "json"
	TextMode LoggerMode = "text"
	dirLog   string     = "logs"
)

var (
	Logs *Loggers
)

func InitLog(modes ...LoggerMode) error {

	// 1. cek folder, kalau tidak ada → buat
	if _, err := os.Stat(dirLog); os.IsNotExist(err) {
		if err := os.MkdirAll(dirLog, 0755); err != nil {
			return fmt.Errorf("failed create logs dir: %w", err)
		}
	}

	// 2. Writer with timestamp filename
	httpWriter, err := prepareWriter(filepath.Join(dirLog, "http"))
	if err != nil {
		return err
	}

	dbWriter, err := prepareWriter(filepath.Join(dirLog, "db"))
	if err != nil {
		return err
	}
	var format = "text"
	for _, mode := range modes {
		switch mode {
		case JSONMode:
			format = "json"
		}

	}

	// 4. Buat logger langsung dari NewLogger()
	httpLogger := NewLogger(format, httpWriter, true)
	dbLogger := NewLogger(format, dbWriter, true)

	Logs = &Loggers{
		HTTP: httpLogger,
		DB:   dbLogger,
	}

	return nil

}

func prepareWriter(prefix string) (io.Writer, error) {

	// ROTATE PER HARI
	rotateWriter, err := rotatelogs.New(
		prefix+"-%Y-%m-%d.log",
		rotatelogs.WithRotationTime(24*time.Hour),
		rotatelogs.WithRotationCount(30),
	)
	if err != nil {
		return nil, fmt.Errorf("failed create rotate logs: %w", err)
	}

	// LUMBERJACK SEBAGAI POST-PROCESS (compress, max size)
	lumberWriter := &lumberjack.Logger{
		Filename:   prefix + ".log", // base file
		MaxSize:    20,              // MB
		MaxAge:     30,              // days
		MaxBackups: 10,
		Compress:   true,
	}
	return io.MultiWriter(rotateWriter, lumberWriter), nil
}
