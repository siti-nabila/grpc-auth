package mocks

import (
	"bytes"
	"github.com/sirupsen/logrus"
)

type Writer struct {
	Buffer bytes.Buffer
	Calls  int
	Error  error
	Panic  bool
}

func (w *Writer) Write(p []byte) (int, error) {
	w.Calls++
	if w.Panic {
		panic("private-writer-sentinel")
	}
	if w.Error != nil {
		return 0, w.Error
	}
	return w.Buffer.Write(p)
}

type Formatter struct{ Error error }

func (f Formatter) Format(*logrus.Entry) ([]byte, error) { return nil, f.Error }
