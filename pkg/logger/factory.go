package logger

import (
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

func NewLogger(format string, writer io.Writer, enableColor bool) *logrus.Logger {
	l := logrus.New()

	// cegah log dobel
	l.SetOutput(io.Discard)
	l.SetLevel(logrus.InfoLevel)

	// Formatters
	var formatterTerminal logrus.Formatter
	var formatterFile logrus.Formatter

	switch format {
	case "json":
		formatterTerminal = &logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		}
		formatterFile = &logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		}

	default:
		formatterTerminal = &logrus.TextFormatter{
			FullTimestamp:   true,
			ForceColors:     enableColor,
			DisableColors:   !enableColor,
			TimestampFormat: "2006-01-02 15:04:05",
		}

		formatterFile = &logrus.TextFormatter{
			FullTimestamp:   true,
			DisableColors:   true, // FILE NON-COLOR
			TimestampFormat: "2006-01-02 15:04:05",
		}
	}

	// OUTPUT ke terminal
	l.AddHook(NewWriterHook(os.Stdout, formatterTerminal, logrus.AllLevels...))

	// OUTPUT ke file (writer external)
	l.AddHook(NewWriterHook(writer, formatterFile, logrus.AllLevels...))

	return l
}
