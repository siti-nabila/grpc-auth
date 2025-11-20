package logger

import (
	"io"

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
	line, err := h.Formatter.Format(e)
	if err != nil {
		return err
	}
	_, err = h.Writer.Write(line)
	return err
}
