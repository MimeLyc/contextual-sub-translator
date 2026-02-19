package service

import (
	"context"
	"testing"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/internal/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

type inMemoryCheckpointStore struct {
	cached map[string][]string
	saved  map[string][]string
}

func (s *inMemoryCheckpointStore) Load(start, end int) ([]string, bool) {
	if s == nil {
		return nil, false
	}
	key := batchKey(start, end)
	v, ok := s.cached[key]
	if !ok {
		return nil, false
	}
	return append([]string(nil), v...), true
}

func (s *inMemoryCheckpointStore) Save(_ context.Context, start, end int, translated []string) error {
	if s.saved == nil {
		s.saved = make(map[string][]string)
	}
	s.saved[batchKey(start, end)] = append([]string(nil), translated...)
	return nil
}

func TestSubTranslator_TranslateSubtitleLines_ResumesFromCheckpoints(t *testing.T) {
	lines := []subtitle.Line{
		{Index: 1, StartTime: time.Second, EndTime: 2 * time.Second, Text: "hello"},
		{Index: 2, StartTime: 3 * time.Second, EndTime: 4 * time.Second, Text: "world"},
		{Index: 3, StartTime: 5 * time.Second, EndTime: 6 * time.Second, Text: "bye"},
	}

	mockTrans := &mockTranslator{}
	mockTrans.On(
		"BatchTranslate",
		mock.Anything,
		mock.AnythingOfType("translator.MediaMeta"),
		lines[2:3],
		"en",
		"zh",
		1,
	).Return([]subtitle.Line{
		{Index: 3, StartTime: 5 * time.Second, EndTime: 6 * time.Second, Text: "bye", TranslatedText: "再见"},
	}, nil).Once()

	subTrans := &SubTranslator{
		translator: mockTrans,
		config: TranslatorConfig{
			TargetLanguage: language.Chinese,
			BatchSize:      2,
		},
		file: &subtitle.File{Language: language.English},
	}

	cp := &inMemoryCheckpointStore{
		cached: map[string][]string{
			batchKey(0, 2): []string{"你好", "世界"},
		},
	}
	ctx := withBatchCheckpointStore(context.Background(), cp)

	ret, err := subTrans.translateSubtitleLines(ctx, translator.MediaMeta{}, lines)
	require.NoError(t, err)
	require.Len(t, ret, 3)
	assert.Equal(t, "你好", ret[0].TranslatedText)
	assert.Equal(t, "世界", ret[1].TranslatedText)
	assert.Equal(t, "再见", ret[2].TranslatedText)
	require.Contains(t, cp.saved, batchKey(2, 3))
	assert.Equal(t, []string{"再见"}, cp.saved[batchKey(2, 3)])

	mockTrans.AssertExpectations(t)
}
