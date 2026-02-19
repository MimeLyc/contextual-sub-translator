package subtitle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSRTBytes(t *testing.T) {
	data := []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n\n2\n00:00:03,000 --> 00:00:04,000\nWorld\n")

	file, err := ReadSRTBytes(data, "embedded://sample")
	require.NoError(t, err)
	require.Len(t, file.Lines, 2)
	assert.Equal(t, "Hello", file.Lines[0].Text)
	assert.Equal(t, "World", file.Lines[1].Text)
	assert.Equal(t, "SRT", file.Format)
	assert.Equal(t, "embedded://sample", file.Path)
}
