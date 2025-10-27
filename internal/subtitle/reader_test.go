package subtitle

import (
	"fmt"
	"testing"

	"golang.org/x/text/language"
)

func TestDetectLanguage(t *testing.T) {
	lines := []Line{
		{
			Text: "Hello, world!",
		},
		{
			Text: "こんにちは、世界!",
		},
		{
			Text: "こんにちは、世界!",
		},

		{
			Text: "Привет, мир!",
		},
	}
	lang := detectLanguage(lines)
	if lang != language.Japanese {
		t.Errorf("expected ja, got %s", lang)
	}
}

func TestLanguageCode(t *testing.T) {
	base, script, region := language.Japanese.Raw()
	iso3 := base.ISO3()
	str := base.String()
	lang := language.All.Make("chinese")
	fmt.Println(str, iso3, script, region, lang)
	// lang := FormatLanguageCode(langStr)
	// if lang != "Chinese" {
	// 	t.Errorf("expected Chinese, got %s", lang)
	// }
}
