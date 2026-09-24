package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeLogMessageFlattensAllLineEndings(t *testing.T) {
	assert.Equal(t, "one two three", sanitizeLogMessage("one\r\ntwo\rthree"))
}
