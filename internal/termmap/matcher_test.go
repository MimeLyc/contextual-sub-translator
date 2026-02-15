package termmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatch(t *testing.T) {
	tm := TermMap{
		"Momo Ayase":    "绫濑桃",
		"Okarun":        "奥卡轮",
		"Turbo Granny":  "涡轮婆婆",
		"Serpo":         "蛇颇",
		"Acrobat Silky": "杂技丝绒",
	}

	texts := []string{
		"Momo Ayase, look out!",
		"Okarun is here.",
		"This is just a regular line.",
	}

	result := Match(tm, texts)

	// Should match Momo Ayase and Okarun
	assert.Len(t, result.Matched, 2)
	assert.Equal(t, "绫濑桃", result.Matched["Momo Ayase"])
	assert.Equal(t, "奥卡轮", result.Matched["Okarun"])

	// Should not match terms not in texts
	_, hasTurbo := result.Matched["Turbo Granny"]
	assert.False(t, hasTurbo)
	_, hasSerpo := result.Matched["Serpo"]
	assert.False(t, hasSerpo)
}

func TestMatch_EmptyTermMap(t *testing.T) {
	result := Match(TermMap{}, []string{"some text"})
	assert.Empty(t, result.Matched)
}

func TestMatch_EmptyTexts(t *testing.T) {
	tm := TermMap{"hello": "world"}
	result := Match(tm, []string{})
	assert.Empty(t, result.Matched)
}

func TestMatch_CaseSensitive(t *testing.T) {
	tm := TermMap{
		"Momo": "桃",
	}

	// Lowercase "momo" should not match "Momo"
	result := Match(tm, []string{"momo is here"})
	assert.Empty(t, result.Matched)

	// Exact case should match
	result = Match(tm, []string{"Momo is here"})
	assert.Len(t, result.Matched, 1)
}

func TestMatch_SubstringMatch(t *testing.T) {
	tm := TermMap{
		"Dan": "但",
	}

	// Should match as substring
	result := Match(tm, []string{"DanDaDan is great"})
	assert.Len(t, result.Matched, 1)
	assert.Equal(t, "但", result.Matched["Dan"])
}

func TestMatch_MultipleTextsOneTerm(t *testing.T) {
	tm := TermMap{
		"Okarun": "奥卡轮",
	}

	// Term appears in multiple texts, should only be in result once
	result := Match(tm, []string{"Okarun here", "Okarun there"})
	assert.Len(t, result.Matched, 1)
}
