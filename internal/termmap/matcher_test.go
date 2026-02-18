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

func TestMatch_WordBoundary(t *testing.T) {
	tm := TermMap{
		"elf": "精灵",
	}

	// "elf" as part of "herself" should NOT match
	result := Match(tm, []string{"She found herself alone."})
	assert.Empty(t, result.Matched)

	// "elf" as a standalone word should match
	result = Match(tm, []string{"The elf cast a spell."})
	assert.Len(t, result.Matched, 1)
	assert.Equal(t, "精灵", result.Matched["elf"])

	// "elf" at end of sentence
	result = Match(tm, []string{"She met an elf"})
	assert.Len(t, result.Matched, 1)

	// "elf" at start of sentence
	result = Match(tm, []string{"elf warriors attacked"})
	assert.Len(t, result.Matched, 1)
}

func TestMatch_WordBoundary_MultiWord(t *testing.T) {
	tm := TermMap{
		"Dan": "但",
	}

	// "Dan" inside "DanDaDan" should NOT match (no boundary after "Dan")
	result := Match(tm, []string{"DanDaDan is great"})
	assert.Empty(t, result.Matched)

	// "Dan" as standalone should match
	result = Match(tm, []string{"Dan is great"})
	assert.Len(t, result.Matched, 1)
}

func TestMatch_WordBoundary_Punctuation(t *testing.T) {
	tm := TermMap{
		"Elf": "精灵",
	}

	// Punctuation counts as word boundary
	result := Match(tm, []string{"Look, an Elf!"})
	assert.Len(t, result.Matched, 1)

	result = Match(tm, []string{"(Elf)"})
	assert.Len(t, result.Matched, 1)

	result = Match(tm, []string{`"Elf"`})
	assert.Len(t, result.Matched, 1)
}

func TestMatch_MultipleTextsOneTerm(t *testing.T) {
	tm := TermMap{
		"Okarun": "奥卡轮",
	}

	// Term appears in multiple texts, should only be in result once
	result := Match(tm, []string{"Okarun here", "Okarun there"})
	assert.Len(t, result.Matched, 1)
}

func TestContainsWordFold(t *testing.T) {
	// Case-insensitive word boundary match
	assert.True(t, ContainsWordFold("The Elf is here", "elf"))
	assert.True(t, ContainsWordFold("the elf is here", "Elf"))
	assert.False(t, ContainsWordFold("herself", "elf"))
	assert.False(t, ContainsWordFold("HERSELF", "elf"))
}
