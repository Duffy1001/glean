package app

import (
	"encoding/json"
	"errors"
	"io"
)

type flusher interface {
	Flush() error
}

type jsonArraySink struct {
	w io.Writer
	// compact matches app's --compact output setting.
	compact bool

	started  bool
	wroteAny bool
}

func newJSONStreamArraySink(w io.Writer, compact bool) *jsonArraySink {
	return &jsonArraySink{w: w, compact: compact}
}

func (s *jsonArraySink) Start() error {
	if s.started {
		return errors.New("array sink already started")
	}
	s.started = true
	if _, err := s.w.Write([]byte("[")); err != nil {
		return err
	}
	return maybeFlush(s.w)
}

func (s *jsonArraySink) WriteItem(item any) error {
	if !s.started {
		return errors.New("array sink not started")
	}
	if s.wroteAny {
		if _, err := s.w.Write([]byte(",")); err != nil {
			return err
		}
	} else {
		s.wroteAny = true
	}
	if !s.compact {
		if _, err := s.w.Write([]byte("\n")); err != nil {
			return err
		}
	}
	var b []byte
	var err error
	if s.compact {
		b, err = json.Marshal(item)
	} else {
		// Prefix keeps each item aligned with other array elements.
		b, err = json.MarshalIndent(item, "  ", "  ")
	}
	if err != nil {
		return err
	}
	if !s.compact {
		// json.MarshalIndent with a non-empty prefix doesn't reliably prefix the first
		// line. Ensure the opening line is aligned with the rest of the array.
		if len(b) < 2 || b[0] != ' ' || b[1] != ' ' {
			b = append([]byte("  "), b...)
		}
	}
	if _, err := s.w.Write(b); err != nil {
		return err
	}
	return maybeFlush(s.w)
}

func (s *jsonArraySink) Finish() error {
	if !s.started {
		return errors.New("array sink not started")
	}
	if !s.compact && s.wroteAny {
		if _, err := s.w.Write([]byte("\n")); err != nil {
			return err
		}
	}
	if _, err := s.w.Write([]byte("]\n")); err != nil {
		return err
	}
	return maybeFlush(s.w)
}

func maybeFlush(w io.Writer) error {
	f, ok := w.(flusher)
	if !ok {
		return nil
	}
	return f.Flush()
}
