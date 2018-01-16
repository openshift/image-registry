package testutil

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/context"
)

type logrusHook struct {
	t *testing.T
}

func (h *logrusHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *logrusHook) Fire(e *logrus.Entry) error {
	line, err := e.String()
	if err != nil {
		h.t.Logf("unable to read entry: %v", err)
		return err
	}

	line = strings.TrimRight(line, " \n")
	h.t.Log(line)
	return nil
}

// WithTestLogger creates a new context with a Distribution logger which
// records the text in the test's error log.
func WithTestLogger(parent context.Context, t *testing.T) context.Context {
	log := logrus.New()
	log.Level = logrus.DebugLevel
	log.Out = ioutil.Discard
	log.Hooks.Add(&logrusHook{t: t})
	return context.WithLogger(parent, logrus.NewEntry(log))
}
