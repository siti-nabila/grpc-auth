package logger

import (
	"io"
	"regexp"

	"github.com/sirupsen/logrus"
)

type (
	WriterHook struct {
		Writer    io.Writer
		Formatter logrus.Formatter
		LevelsArr []logrus.Level
	}
	FileHook struct {
		Writer    io.Writer
		Formatter logrus.Formatter
		LevelsArr []logrus.Level
	}
)

var ansi = regexp.MustCompile(`(?:\\x1b|\x1b)\[[0-9;?]*[ -/]*[@-~]`)

func NewWriterHook(w io.Writer, f logrus.Formatter, levels ...logrus.Level) *WriterHook {
	if len(levels) == 0 {
		levels = logrus.AllLevels
	}

	return &WriterHook{
		Writer:    w,
		Formatter: f,
		LevelsArr: levels,
	}
}

func NewFileHook(w io.Writer, f logrus.Formatter, levels ...logrus.Level) *FileHook {
	if len(levels) == 0 {
		levels = logrus.AllLevels
	}

	return &FileHook{
		Writer:    w,
		Formatter: f,
		LevelsArr: levels,
	}
}
func (h *WriterHook) Levels() []logrus.Level {
	return h.LevelsArr
}

func (h *WriterHook) Fire(entry *logrus.Entry) error {
	// Format message khusus untuk writer ini
	msg, err := h.Formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = h.Writer.Write(msg)
	return err
}

func (h *FileHook) Levels() []logrus.Level {
	return h.LevelsArr
}

func (h *FileHook) Fire(e *logrus.Entry) error {
	// remove ANSI color before writing to file
	clean := ansi.ReplaceAllString(e.Message, "")

	newEntry := &logrus.Entry{
		Logger:  e.Logger,
		Time:    e.Time,
		Level:   e.Level,
		Message: clean,
		Data:    e.Data,
	}

	msg, err := h.Formatter.Format(newEntry)
	if err != nil {
		return err
	}
	_, err = h.Writer.Write(msg)
	return err
}
