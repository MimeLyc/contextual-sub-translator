package translator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/agent"
	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/internal/termmap"
	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
)

var properNounPattern = regexp.MustCompile(`\b[A-Z][A-Za-z0-9'’-]*(?:\s+[A-Z][A-Za-z0-9'’-]*)*\b`)

var (
	htmlCommentPattern   = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlLineBreakPattern = regexp.MustCompile(`(?i)<\s*(?:br\s*/?|/p|/div)\s*>`)
	htmlTagPattern       = regexp.MustCompile(`(?is)</?[a-z][^>]*>`)
)

var ignoredProperNouns = map[string]struct{}{
	"a":    {},
	"an":   {},
	"and":  {},
	"he":   {},
	"i":    {},
	"it":   {},
	"she":  {},
	"the":  {},
	"they": {},
	"we":   {},
	"you":  {},
}

// agentTranslator is the unified AI layer for all translation
// It uses an agent with tool calling support for enhanced translation quality
type agentTranslator struct {
	agent          *agent.LLMAgent
	searchEnabled  bool
	mu             sync.Mutex
	collectedCalls []agent.ToolCallRecord
}

// NewAgentTranslator creates a new agent-based translator
func NewAgentTranslator(agentInstance *agent.LLMAgent, searchEnabled bool) Translator {
	return &agentTranslator{
		agent:         agentInstance,
		searchEnabled: searchEnabled,
	}
}

func (t *agentTranslator) Translate(
	ctx context.Context,
	media MediaMeta,
	subtitleTexts []string,
	sourceLang string,
	targetLang string,
) ([]string, error) {
	// Remember whether the show has a term map before filtering
	hasTermMap := len(media.TermMap) > 0

	// Filter term map to only terms appearing in this batch
	if hasTermMap {
		result := termmap.Match(termmap.TermMap(media.TermMap), subtitleTexts)
		media.TermMap = map[string]string(result.Matched)
	}

	systemPrompt := t.buildContextPrompt(media, sourceLang, targetLang, hasTermMap, subtitleTexts)
	userMessage, err := buildTranslationUserMessage(subtitleTexts)
	if err != nil {
		return nil, fmt.Errorf("build translation request failed: %w", err)
	}

	maxAttempts := 2
	var lastErr error
	var previousOutput string
	var bestTranslations []string

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptMessage := userMessage
		if attempt == 2 && strings.TrimSpace(previousOutput) != "" {
			attemptMessage = buildRepairUserMessage(userMessage, previousOutput, lastErr, len(subtitleTexts))
		}

		req := agent.AgentRequest{
			SystemPrompt: systemPrompt,
			UserMessage:  attemptMessage,
		}
		result, execErr := t.agent.Execute(ctx, req)
		if execErr != nil {
			lastErr = fmt.Errorf("agent execution failed: %w", execErr)
			if attempt < maxAttempts {
				log.Warn("Translation attempt %d/%d failed to execute: %v; retrying", attempt, maxAttempts, execErr)
				continue
			}
			return nil, lastErr
		}

		previousOutput = result.Content
		log.Debug("Agent translate call completed: lines=%d, iterations=%d, tool_calls=%d", len(subtitleTexts), result.Iterations, len(result.ToolCalls))

		if len(result.ToolCalls) > 0 {
			log.Debug("Agent used %d tool calls in %d iterations", len(result.ToolCalls), result.Iterations)
			for _, tc := range result.ToolCalls {
				log.Debug("  - Tool: %s, Error: %v", tc.ToolName, tc.IsError)
			}
			t.mu.Lock()
			t.collectedCalls = append(t.collectedCalls, result.ToolCalls...)
			t.mu.Unlock()
		}

		translations, parseErr := parseTranslationOutput(result.Content, len(subtitleTexts))
		if parseErr == nil {
			fixInlineBreakers(subtitleTexts, translations)
		}

		// Hard validation failed (structural: line count, JSON parse).
		// These make the output unusable — retry.
		if parseErr != nil {
			lastErr = parseErr
			if attempt < maxAttempts {
				log.Warn("Translation output failed validation on attempt %d/%d: %v; retrying with repair prompt", attempt, maxAttempts, parseErr)
				continue
			}
			return nil, fmt.Errorf("translation validation failed after repair retry: %w", lastErr)
		}

		// Soft validation (term mapping): quality check, not structural.
		// Try repair once, but accept the best result if repair also fails.
		termErr := validateTermMappings(subtitleTexts, translations, media.TermMap)
		if termErr == nil {
			return normalizeTranslatedLines(translations), nil
		}
		if attempt < maxAttempts {
			lastErr = termErr
			bestTranslations = translations
			log.Warn("Translation output has term mapping issues on attempt %d/%d: %v; retrying with repair prompt", attempt, maxAttempts, termErr)
			continue
		}
		// Repair also didn't fix term mapping — accept with warning.
		log.Warn("Accepting translation despite term mapping issues: %v", termErr)
		return normalizeTranslatedLines(translations), nil
	}

	// Should only reach here if all attempts had hard failures.
	// If we have a structurally valid result from a previous attempt, use it.
	if bestTranslations != nil {
		log.Warn("Returning best-effort translation despite term mapping issues")
		return normalizeTranslatedLines(bestTranslations), nil
	}
	return nil, fmt.Errorf("translation validation failed after repair retry: %w", lastErr)
}

func (t *agentTranslator) BatchTranslate(
	ctx context.Context,
	media MediaMeta,
	subtitleLines []subtitle.Line,
	sourceLanguage string,
	targetLanguage string,
	batchSize int) ([]subtitle.Line, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	allTranslations, err := t.batchTranslate(ctx, media, subtitleLines, sourceLanguage, targetLanguage, batchSize, 0, len(subtitleLines))
	if err != nil {
		return nil, err
	}

	for i, line := range subtitleLines {
		subtitleLines[i] = subtitle.Line{
			Index:          line.Index,
			StartTime:      line.StartTime,
			EndTime:        line.EndTime,
			Text:           line.Text,
			TranslatedText: allTranslations[i],
		}
	}

	return subtitleLines, nil
}

func (t *agentTranslator) batchTranslate(
	ctx context.Context,
	media MediaMeta,
	subtitleLines []subtitle.Line,
	sourceLanguage string,
	targetLanguage string,
	batchSize int,
	startIncluded int,
	endExcluded int,
) ([]string, error) {
	return batchTranslateWithFallback(
		ctx,
		media,
		subtitleLines,
		sourceLanguage,
		targetLanguage,
		batchSize,
		startIncluded,
		endExcluded,
		t.Translate,
	)
}

type translateBatchFunc func(
	ctx context.Context,
	media MediaMeta,
	subtitleTexts []string,
	sourceLanguage string,
	targetLanguage string,
) ([]string, error)

func batchTranslateWithFallback(
	ctx context.Context,
	media MediaMeta,
	subtitleLines []subtitle.Line,
	sourceLanguage string,
	targetLanguage string,
	batchSize int,
	startIncluded int,
	endExcluded int,
	translate translateBatchFunc,
) ([]string, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("batch size must be greater than 0")
	}
	if translate == nil {
		return nil, fmt.Errorf("translate function must not be nil")
	}
	if startIncluded < 0 || endExcluded < startIncluded || endExcluded > len(subtitleLines) {
		return nil, fmt.Errorf("invalid batch range %d-%d for %d subtitle lines", startIncluded, endExcluded, len(subtitleLines))
	}

	var allTranslations []string

	for i := startIncluded; i < endExcluded; i += batchSize {
		batchStart := time.Now()
		end := min(i+batchSize, endExcluded, len(subtitleLines))
		lineStart, lineEnd := subtitleLineRange(subtitleLines, i, end)

		batch := subtitleLines[i:end]

		var subtitleTexts []string
		for _, line := range batch {
			// Deal with original line breaker in subtitle file to avoid LLM misunderstanding
			formattedText := strings.ReplaceAll(line.Text, "\n", inlineBreakerPlaceholder)
			subtitleTexts = append(subtitleTexts, formattedText)
		}

		translations, err := translate(ctx, media, subtitleTexts, sourceLanguage, targetLanguage)
		if err != nil {
			if len(subtitleTexts) == 1 {
				return nil, fmt.Errorf("batch translation failed for lines %d-%d: %w", lineStart, lineEnd, err)
			}
			nextBatchSize := nextFallbackBatchSize(len(subtitleTexts))
			log.Warn("batch translation failed for lines %d-%d: %v; retrying with batch size %d", lineStart, lineEnd, err, nextBatchSize)
			if translations, err = batchTranslateWithFallback(ctx, media, subtitleLines, sourceLanguage, targetLanguage, nextBatchSize, i, end, translate); err != nil {
				return nil, fmt.Errorf("retry batch translation failed for lines %d-%d: %w", lineStart, lineEnd, err)
			}
		}

		if len(translations) != len(subtitleTexts) {
			if len(subtitleTexts) == 1 {
				if len(translations) == 0 {
					return nil, fmt.Errorf("single-line translation returned no content for line %d", lineStart)
				}
				log.Warn("Single-line translation count mismatch at line %d: expected 1, got %d; using first candidate", lineStart, len(translations))
				translations = []string{translations[0]}
			} else {
				nextBatchSize := nextFallbackBatchSize(len(subtitleTexts))
				if nextBatchSize >= len(subtitleTexts) {
					return nil, fmt.Errorf("translation count mismatch for lines %d-%d: expected %d, got %d", lineStart, lineEnd, len(subtitleTexts), len(translations))
				}
				log.Warn("batch translation count mismatch for lines %d-%d: expected %d, got %d; retrying with batch size %d", lineStart, lineEnd, len(subtitleTexts), len(translations), nextBatchSize)
				if translations, err = batchTranslateWithFallback(ctx, media, subtitleLines, sourceLanguage, targetLanguage, nextBatchSize, i, end, translate); err != nil {
					return nil, fmt.Errorf("retry batch translation failed for lines %d-%d: %w", lineStart, lineEnd, err)
				}
			}
		}

		allTranslations = append(allTranslations, translations...)
		log.Debug("Batch translated lines %d-%d in %s (size=%d)", lineStart, lineEnd, time.Since(batchStart), len(subtitleTexts))
	}

	return allTranslations, nil
}

func nextFallbackBatchSize(currentBatchSize int) int {
	if currentBatchSize <= 1 {
		return 1
	}
	return max(currentBatchSize/2, 1)
}

func subtitleLineRange(lines []subtitle.Line, startIncluded int, endExcluded int) (int, int) {
	start := startIncluded + 1
	end := endExcluded
	if startIncluded >= 0 && startIncluded < len(lines) && lines[startIncluded].Index > 0 {
		start = lines[startIncluded].Index
	}
	last := endExcluded - 1
	if last >= 0 && last < len(lines) && lines[last].Index > 0 {
		end = lines[last].Index
	}
	return start, end
}

// buildContextPrompt builds the system prompt for translation.
// hasTermMap indicates whether the show has a term map at all (before per-batch filtering).
func (t *agentTranslator) buildContextPrompt(
	media MediaMeta,
	sourceLanguage string,
	targetLanguage string,
	hasTermMap bool,
	subtitleTexts []string,
) string {
	var prompt strings.Builder

	prompt.WriteString("You are a professional subtitle translator. Translate the following subtitles from " + sourceLanguage + " to " + targetLanguage + ".\n\n")

	prompt.WriteString("=== MEDIA INFORMATION ===\n")
	if media.Title != "" {
		prompt.WriteString(fmt.Sprintf("Show Title: %s\n", media.Title))
	}
	if media.OriginalTitle != "" {
		prompt.WriteString(fmt.Sprintf("Original Title: %s\n", media.OriginalTitle))
	}
	if len(media.Genre) > 0 {
		prompt.WriteString(fmt.Sprintf("Genre: %s\n", strings.Join(media.Genre, ", ")))
	}
	if media.Year > 0 {
		prompt.WriteString(fmt.Sprintf("Year: %d\n", media.Year))
	}
	if media.Studio != "" {
		prompt.WriteString(fmt.Sprintf("Production Studio: %s\n", media.Studio))
	}
	if media.Plot != "" {
		prompt.WriteString(fmt.Sprintf("Plot Summary: %s\n", media.Plot))
	}

	if len(media.TermMap) > 0 {
		prompt.WriteString("\n=== TERM MAPPINGS ===\n")
		prompt.WriteString("You MUST use the mapped target term exactly whenever its source term appears in a line.\n")
		prompt.WriteString("Do NOT replace mapped terms with synonyms, aliases, or alternative transliterations.\n")
		for source, target := range media.TermMap {
			prompt.WriteString(fmt.Sprintf("  %s -> %s\n", source, target))
		}
	}

	if t.searchEnabled {
		searchBudget, unresolved := computeWebSearchBudget(subtitleTexts, media.TermMap, hasTermMap)
		prompt.WriteString("\n=== WEB SEARCH TOOL ===\n")
		if searchBudget <= 0 {
			prompt.WriteString("Do NOT call web_search for this batch.\n")
		} else {
			callWord := "calls"
			if searchBudget == 1 {
				callWord = "call"
			}
			prompt.WriteString(fmt.Sprintf("You may use web_search at most %d web_search %s for this batch.\n", searchBudget, callWord))
			if hasTermMap {
				prompt.WriteString("Only use web_search for unresolved proper nouns not covered by TERM MAPPINGS.\n")
			} else {
				prompt.WriteString("Use web_search only to confirm official names for key characters/places before translating.\n")
			}
			if len(unresolved) > 0 {
				limit := min(5, len(unresolved))
				prompt.WriteString("Potential unresolved proper nouns: " + strings.Join(unresolved[:limit], ", ") + "\n")
			}
		}
	}

	prompt.WriteString("\n=== TRANSLATION GUIDELINES ===\n")
	prompt.WriteString("1. Maintain character voice and relationship dynamics\n")
	prompt.WriteString("2. Ensure " + targetLanguage + " flows naturally while preserving meaning\n")
	prompt.WriteString("3. Keep subtitle length appropriate for screen reading\n")
	prompt.WriteString("4. You MUST preserve the count of " + inlineBreakerPlaceholder + " in each line exactly\n")
	prompt.WriteString("5. Keep one translated line per input line index\n")
	prompt.WriteString("6. Do NOT merge, split, reorder, or drop lines\n")
	prompt.WriteString("7. If an input line is empty, output text for that index MUST be an empty string\n")
	prompt.WriteString("8. Priority for proper nouns and terms: TERM MAPPINGS > official localized names > transliteration\n")
	prompt.WriteString("9. Output plain subtitle text only; remove HTML/XML tags and do not add markup\n")

	prompt.WriteString("\n=== OUTPUT FORMAT ===\n")
	prompt.WriteString("Return ONLY a valid JSON array of objects.\n")
	prompt.WriteString("Schema: [{\"index\":1,\"text\":\"translated line\"}]\n")
	prompt.WriteString("Rules: each object MUST include integer index (1-based) and string text; include each index exactly once; no extra keys.\n")
	prompt.WriteString("Do NOT output literal newline characters in JSON text. Use " + inlineBreakerPlaceholder + " to represent every line break.\n")
	prompt.WriteString("No markdown, no explanations, no prose outside JSON.\n")

	return prompt.String()
}

type translationInputLine struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

func buildTranslationUserMessage(subtitleTexts []string) (string, error) {
	lines := make([]translationInputLine, 0, len(subtitleTexts))
	for i, line := range subtitleTexts {
		lines = append(lines, translationInputLine{Index: i + 1, Text: line})
	}

	payload := struct {
		Lines []translationInputLine `json:"lines"`
	}{
		Lines: lines,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

type translationOutputLine struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

var errUnsupportedTranslationSchema = errors.New("unsupported translation json schema")

var translationWrapperFields = []string{
	"translations",
	"translation",
	"lines",
	"translated_lines",
	"translated_texts",
	"results",
	"items",
	"output",
}

var translationIndexFields = []string{
	"index",
	"line",
	"line_index",
	"line_number",
	"id",
	"number",
}

var translationTextFields = []string{
	"text",
	"translation",
	"translated",
	"translated_text",
	"content",
	"value",
}

func parseTranslationOutput(content string, expectedCount int) ([]string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("empty translation output")
	}

	raw := trimmed
	if fenced := unwrapCodeFence(trimmed); fenced != trimmed {
		raw = strings.TrimSpace(fenced)
	}

	translations, parseErr := parseJSONTranslations(raw, expectedCount)
	if parseErr == nil {
		return translations, nil
	}

	if extracted := extractJSONArray(raw); extracted != "" {
		if translations, err := parseJSONTranslations(extracted, expectedCount); err == nil {
			return translations, nil
		}
	}

	if extracted := extractJSONObject(raw); extracted != "" {
		if translations, err := parseJSONTranslations(extracted, expectedCount); err == nil {
			return translations, nil
		}
	}

	return nil, fmt.Errorf("translation output must be valid JSON with required schema: %w", parseErr)
}

func parseJSONTranslations(content string, expectedCount int) ([]string, error) {
	rawContent := []byte(content)

	translations, err := parseTranslationContainer(rawContent, expectedCount)
	if err == nil {
		return translations, nil
	}
	if !errors.Is(err, errUnsupportedTranslationSchema) {
		return nil, err
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(rawContent, &object); err == nil {
		for _, field := range translationWrapperFields {
			if raw, ok := lookupRawField(object, field); ok {
				translations, err := parseTranslationContainer(raw, expectedCount)
				if err != nil {
					return nil, fmt.Errorf("unsupported %s schema: %w", field, err)
				}
				return translations, nil
			}
		}
	}

	return nil, errUnsupportedTranslationSchema
}

func parseTranslationContainer(raw []byte, expectedCount int) ([]string, error) {
	indexed, indexedErr := parseIndexedTranslationLines(raw)
	if indexedErr == nil {
		return reorderIndexedTranslations(indexed, expectedCount)
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		if err := validateExpectedLineCount(list, expectedCount); err != nil {
			return nil, err
		}
		return list, nil
	}

	if isTopLevelJSONKind(raw, '[') {
		return nil, indexedErr
	}

	indexed, numericErr := parseNumericKeyedTranslations(raw)
	if numericErr == nil {
		return reorderIndexedTranslations(indexed, expectedCount)
	}
	if !errors.Is(numericErr, errUnsupportedTranslationSchema) {
		return nil, numericErr
	}

	return nil, errUnsupportedTranslationSchema
}

func parseIndexedTranslationLines(raw []byte) ([]translationOutputLine, error) {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}

	lines := make([]translationOutputLine, 0, len(items))
	for i, item := range items {
		indexRaw, ok := lookupRawField(item, translationIndexFields...)
		if !ok {
			return nil, fmt.Errorf("translation item %d index: missing required field", i+1)
		}
		index, err := parseTranslationIndex(indexRaw)
		if err != nil {
			return nil, fmt.Errorf("translation item %d index: %w", i+1, err)
		}
		textRaw, ok := lookupRawField(item, translationTextFields...)
		if !ok {
			return nil, fmt.Errorf("translation item %d text: missing required field", i+1)
		}
		text, err := parseTranslationText(textRaw)
		if err != nil {
			return nil, fmt.Errorf("translation item %d text: %w", i+1, err)
		}
		lines = append(lines, translationOutputLine{Index: index, Text: text})
	}
	return lines, nil
}

func parseNumericKeyedTranslations(raw []byte) ([]translationOutputLine, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, errUnsupportedTranslationSchema
	}
	if len(object) == 0 {
		return nil, errUnsupportedTranslationSchema
	}

	lines := make([]translationOutputLine, 0, len(object))
	for key, rawText := range object {
		index, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil {
			return nil, errUnsupportedTranslationSchema
		}
		text, err := parseTranslationText(rawText)
		if err != nil {
			return nil, fmt.Errorf("translation key %q text: %w", key, err)
		}
		lines = append(lines, translationOutputLine{Index: index, Text: text})
	}
	return lines, nil
}

func isTopLevelJSONKind(raw []byte, kind byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) > 0 && trimmed[0] == kind
}

func lookupRawField(fields map[string]json.RawMessage, names ...string) (json.RawMessage, bool) {
	for _, name := range names {
		if raw, ok := fields[name]; ok {
			return raw, true
		}
	}

	for key, raw := range fields {
		normalizedKey := normalizeJSONFieldName(key)
		for _, name := range names {
			if normalizedKey == normalizeJSONFieldName(name) {
				return raw, true
			}
		}
	}

	return nil, false
}

func normalizeJSONFieldName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	return name
}

func parseTranslationIndex(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("missing required field")
	}

	var index int
	if err := json.Unmarshal(raw, &index); err == nil {
		return index, nil
	}

	var indexString string
	if err := json.Unmarshal(raw, &indexString); err == nil {
		parsed, err := strconv.Atoi(strings.TrimSpace(indexString))
		if err != nil {
			return 0, fmt.Errorf("must be an integer: %q", indexString)
		}
		return parsed, nil
	}

	var indexFloat float64
	if err := json.Unmarshal(raw, &indexFloat); err == nil {
		if math.Trunc(indexFloat) != indexFloat {
			return 0, fmt.Errorf("must be an integer: %v", indexFloat)
		}
		return int(indexFloat), nil
	}

	return 0, fmt.Errorf("must be an integer")
}

func parseTranslationText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("missing required field")
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return "", nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", fmt.Errorf("must be a string: %w", err)
	}
	return text, nil
}

func reorderIndexedTranslations(lines []translationOutputLine, expectedCount int) ([]string, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("translation output array is empty")
	}

	byIndex := make(map[int]string, len(lines))
	maxIndex := 0
	for _, line := range lines {
		if line.Index <= 0 {
			return nil, fmt.Errorf("translation index must be positive: %d", line.Index)
		}
		if _, exists := byIndex[line.Index]; exists {
			return nil, fmt.Errorf("duplicate translation index: %d", line.Index)
		}
		byIndex[line.Index] = line.Text
		if line.Index > maxIndex {
			maxIndex = line.Index
		}
	}

	total := maxIndex
	if expectedCount > 0 {
		total = expectedCount
	}

	if len(byIndex) != total {
		return nil, fmt.Errorf("translation index count mismatch: expected %d, got %d", total, len(byIndex))
	}

	ordered := make([]string, total)
	for index := 1; index <= total; index++ {
		text, ok := byIndex[index]
		if !ok {
			return nil, fmt.Errorf("missing translation index: %d", index)
		}
		ordered[index-1] = text
	}

	return ordered, nil
}

func validateExpectedLineCount(lines []string, expectedCount int) error {
	if expectedCount > 0 && len(lines) != expectedCount {
		return fmt.Errorf("translation line count mismatch: expected %d, got %d", expectedCount, len(lines))
	}
	return nil
}

// fixInlineBreakers reconciles the inline breaker count between source and
// translated lines. When the LLM drops or adds breakers, this function repairs
// the translated line instead of failing the entire batch.
//   - Missing breakers: inserted at the midpoint of the translated text.
//   - Extra breakers:   surplus ones are removed from the end.
func fixInlineBreakers(sourceLines []string, translatedLines []string) {
	for i := range sourceLines {
		expected := strings.Count(sourceLines[i], inlineBreakerPlaceholder)
		actual := strings.Count(translatedLines[i], inlineBreakerPlaceholder)
		if expected == actual {
			continue
		}
		if actual < expected {
			// Insert missing breakers at midpoint of the text.
			translatedLines[i] = insertBreakers(translatedLines[i], expected-actual)
			log.Debug("Inserted %d missing inline breaker(s) at line %d", expected-actual, i+1)
		} else {
			// Remove surplus breakers from the end.
			translatedLines[i] = removeBreakers(translatedLines[i], actual-expected)
			log.Debug("Removed %d extra inline breaker(s) at line %d", actual-expected, i+1)
		}
	}
}

// insertBreakers adds n inline breaker placeholders into text. It tries to
// split the text into n+1 roughly equal segments separated by breakers.
func insertBreakers(text string, n int) string {
	// Strip any existing breakers to get clean text, then re-split.
	clean := strings.ReplaceAll(text, inlineBreakerPlaceholder, "")
	if clean == "" {
		parts := make([]string, n+1)
		for i := range parts {
			parts[i] = ""
		}
		return strings.Join(parts, inlineBreakerPlaceholder)
	}

	runes := []rune(clean)
	segLen := len(runes) / (n + 1)
	if segLen < 1 {
		segLen = 1
	}

	var b strings.Builder
	inserted := 0
	for j, r := range runes {
		b.WriteRune(r)
		if inserted < n && (j+1)%segLen == 0 && j+1 < len(runes) {
			b.WriteString(inlineBreakerPlaceholder)
			inserted++
		}
	}
	// If we still haven't inserted enough (very short text), append the rest.
	for inserted < n {
		b.WriteString(inlineBreakerPlaceholder)
		inserted++
	}
	return b.String()
}

// removeBreakers removes n inline breaker placeholders from text,
// starting from the last occurrence.
func removeBreakers(text string, n int) string {
	for n > 0 {
		idx := strings.LastIndex(text, inlineBreakerPlaceholder)
		if idx < 0 {
			break
		}
		text = text[:idx] + text[idx+len(inlineBreakerPlaceholder):]
		n--
	}
	return text
}

func validateTermMappings(sourceLines []string, translatedLines []string, termMap map[string]string) error {
	if len(termMap) == 0 {
		return nil
	}
	if len(sourceLines) != len(translatedLines) {
		return fmt.Errorf("line count mismatch before term mapping validation: expected %d, got %d", len(sourceLines), len(translatedLines))
	}

	sortedSources := make([]string, 0, len(termMap))
	for source := range termMap {
		sortedSources = append(sortedSources, source)
	}
	sort.Strings(sortedSources)

	violations := make([]string, 0, 3)
	for lineIndex := range sourceLines {
		translatedLower := strings.ToLower(translatedLines[lineIndex])

		for _, source := range sortedSources {
			target := strings.TrimSpace(termMap[source])
			source = strings.TrimSpace(source)
			if source == "" || target == "" {
				continue
			}

			if termmap.ContainsWordFold(sourceLines[lineIndex], source) && !strings.Contains(translatedLower, strings.ToLower(target)) {
				violations = append(violations, fmt.Sprintf("line %d requires %q -> %q", lineIndex+1, source, target))
				if len(violations) == cap(violations) {
					return fmt.Errorf("term mapping constraint violated: %s", strings.Join(violations, "; "))
				}
			}
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("term mapping constraint violated: %s", strings.Join(violations, "; "))
	}

	return nil
}

func normalizeTranslatedLines(lines []string) []string {
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		normalized = append(normalized, sanitizeTranslatedText(line))
	}
	return normalized
}

func sanitizeTranslatedText(text string) string {
	text = html.UnescapeString(text)
	text = htmlCommentPattern.ReplaceAllString(text, "")
	text = htmlLineBreakPattern.ReplaceAllString(text, "\n")
	text = htmlTagPattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, inlineBreakerPlaceholder, "\n")

	parts := strings.Split(text, "\n")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func buildRepairUserMessage(originalUserMessage string, previousOutput string, validationErr error, expectedCount int) string {
	var builder strings.Builder
	builder.WriteString("Your previous translation output was invalid and must be corrected.\n")
	if validationErr != nil {
		builder.WriteString("Validation error: " + validationErr.Error() + "\n")
	}
	if strings.TrimSpace(previousOutput) != "" {
		builder.WriteString("Previous output:\n")
		builder.WriteString(previousOutput)
		builder.WriteString("\n")
	}
	builder.WriteString("Input lines JSON:\n")
	builder.WriteString(originalUserMessage)
	builder.WriteString("\n")
	builder.WriteString("Return ONLY valid JSON array objects using schema [{\"index\":1,\"text\":\"...\"}].\n")
	builder.WriteString(fmt.Sprintf("Each index from 1 to %d must appear exactly once.\n", expectedCount))
	builder.WriteString("Preserve all required term mappings and inline break markers exactly.\n")
	builder.WriteString("Do NOT merge/split lines and do NOT output literal newlines in text; use " + inlineBreakerPlaceholder + " only.\n")
	builder.WriteString("Return plain subtitle text only; remove HTML/XML tags and do not add markup.\n")
	return builder.String()
}

func computeWebSearchBudget(subtitleTexts []string, termMap map[string]string, hasTermMap bool) (int, []string) {
	candidates := extractProperNounCandidates(subtitleTexts)
	unresolved := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidateCoveredByTermMap(candidate, termMap) {
			continue
		}
		unresolved = append(unresolved, candidate)
	}

	unresolvedCount := len(unresolved)
	if hasTermMap {
		if unresolvedCount == 0 {
			return 0, unresolved
		}
		budget := (unresolvedCount + 2) / 3
		return min(budget, 2), unresolved
	}

	if unresolvedCount == 0 {
		return 1, unresolved
	}

	budget := (unresolvedCount + 1) / 2
	budget = max(1, budget)
	return min(budget, 3), unresolved
}

func extractProperNounCandidates(subtitleTexts []string) []string {
	seen := make(map[string]struct{})
	candidates := make([]string, 0)

	for _, line := range subtitleTexts {
		line = strings.ReplaceAll(line, inlineBreakerPlaceholder, " ")
		for _, match := range properNounPattern.FindAllString(line, -1) {
			candidate := strings.TrimSpace(match)
			if candidate == "" {
				continue
			}

			normalized := strings.ToLower(candidate)
			if _, ignored := ignoredProperNouns[normalized]; ignored {
				continue
			}
			if _, exists := seen[normalized]; exists {
				continue
			}

			seen[normalized] = struct{}{}
			candidates = append(candidates, candidate)
		}
	}

	sort.Strings(candidates)
	return candidates
}

func candidateCoveredByTermMap(candidate string, termMap map[string]string) bool {
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	if candidate == "" {
		return true
	}

	for source, target := range termMap {
		source = strings.ToLower(strings.TrimSpace(source))
		target = strings.ToLower(strings.TrimSpace(target))
		if source == "" && target == "" {
			continue
		}
		if strings.Contains(source, candidate) || strings.Contains(candidate, source) {
			return true
		}
		if strings.Contains(target, candidate) || strings.Contains(candidate, target) {
			return true
		}
	}

	return false
}

func unwrapCodeFence(content string) string {
	if !strings.HasPrefix(content, "```") {
		return content
	}
	inner := content[3:]
	if nl := strings.Index(inner, "\n"); nl >= 0 {
		inner = inner[nl+1:]
	}
	if end := strings.LastIndex(inner, "```"); end >= 0 {
		inner = inner[:end]
	}
	return inner
}

func extractJSONArray(s string) string {
	start := strings.Index(s, "[")
	if start < 0 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '[' {
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// CollectedToolCalls returns all accumulated tool calls since the last reset.
func (t *agentTranslator) CollectedToolCalls() []agent.ToolCallRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]agent.ToolCallRecord, len(t.collectedCalls))
	copy(result, t.collectedCalls)
	return result
}

// ResetCollectedToolCalls clears the accumulated tool calls.
func (t *agentTranslator) ResetCollectedToolCalls() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.collectedCalls = nil
}
