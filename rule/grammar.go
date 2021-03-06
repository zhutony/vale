package rule

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/errata-ai/vale/core"
)

var skipped = []string{
	// Collides with `Vale.Repetition`
	"ENGLISH_WORD_REPEAT_RULE",
	// Doesn't work well
	"WHITESPACE_RULE",
	// Collides with `Vale.Spelling`
	"MORFOLOGIK_RULE",
	"MORFOLOGIK_RULE_EN_US",
	"HUNSPELL_RULE",
	"HUNSPELL_RULE_EN_US",
	"HUNSPELL_NO_SUGGEST_RULE",
}
var disabled = []string{
	"GENDER_NEUTRALITY", "COLLOQUIALISMS", "WIKIPEDIA", "BARBARISM",
	"SEMANTICS", "STYLE", "CASING", "REDUNDANCY", "TYPOGRAPHY",
	"PUNCTUATION",
}
var enabled = []string{
	"MISC", "GRAMMAR", "CONFUSED_WORDS", "TYPOS",
}
var index = map[string]string{
	"MISSING_COMMA_WITH_NNP":                  "Missing a comma in '%s'.",
	"MISSING_COMMA_AFTER_INTRODUCTORY_PHRASE": "Missing a comma in '%s'.",
	"COMMA_AFTER_A_MONTH":                     "Unnecessary comma in '%s'.",
	"MISSING_COMMA_BETWEEN_DAY_AND_YEAR":      "Missing a comma in '%s'.",
	"UNNECESSARY_COMMA":                       "Unnecessary comma in '%s'.",
	"MISSING_COMMA_AFTER_WEEKDAY":             "Missing a comma in '%s'.",
	"COMMA_TAG_QUESTION":                      "Missing a comma in '%s'.",
	"APOS_ARE":                                "Unnecessary apostrophe in '%s'.",
	"MISSING_HYPHEN":                          "'%s' should be hyphenated.",
	"SWORN_AFFIDAVIT":                         "'%s' is redundant.",
	"ENGLISH_WORD_REPEAT_RULE":                "Repeated word: '%s'!",
	"DT_DT":                                   "Repeated word: '%s'!",
}

// LTResult represents a JSON object returned by LanguageTool.
type LTResult struct {
	Software software `json:"software"`
	Warnings warnings `json:"warnings"`
	Language language `json:"language"`
	Matches  []match  `json:"matches"`
}

// CheckWithLT interfaces with a running instace of LanguageTool.
//
// TODO: How do we speed this up?
func CheckWithLT(text, path string, f *core.File, debug bool) []core.Alert {
	alerts := []core.Alert{}

	resp, err := checkWithURL(text, "en-US", path)
	core.CheckError(err, debug)

	for _, m := range resp.Matches {
		alerts = append(alerts, matchToAlert(m))
	}

	return alerts
}

// Convert a LanguageTool-style Match object to an Alert.
func matchToAlert(m match) core.Alert {
	ctx := m.Context

	target := ctx.Text[ctx.Offset : ctx.Offset+ctx.Length]
	suggestions := replacementsToParams(m.Replacements)

	alert := core.Alert{
		Severity: "warning",
		Match:    target,
		Check:    "LanguageTool." + m.Rule.ID,
		Span:     []int{ctx.Offset, ctx.Offset + ctx.Length},
		Action: core.Action{
			Name:   "replace",
			Params: suggestions,
		},
	}

	if msg, found := index[m.Rule.ID]; found {
		alert.Message = core.FormatMessage(
			msg,
			target)
	} else if len(suggestions) > 0 {
		alert.Message = fmt.Sprintf(
			"Did you mean '%s'?",
			core.ToSentence(suggestions, "or"))
	} else if len(m.ShortMessage) > 0 {
		alert.Message = strings.Replace(m.ShortMessage, `"`, "'", -1)
	} else {
		alert.Message = fmt.Sprintf(
			"Did you really mean '%s'?",
			target)
	}

	return alert
}

func replacementsToParams(options []replacement) []string {
	suggestions := []string{}
	for _, opt := range options {
		suggestions = append(suggestions, opt.Value)
	}
	return suggestions
}

func checkWithURL(text, lang, apiURL string) (LTResult, error) {
	data := url.Values{}

	data.Set("text", text)
	data.Set("language", lang)
	data.Set("enabledCategories", strings.Join(enabled, ","))
	data.Set("disabledCategories", strings.Join(disabled, ","))
	data.Set("disabledRules", strings.Join(skipped, ","))
	data.Set("motherTongue", "en")

	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return LTResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return LTResult{}, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	result := LTResult{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return LTResult{}, err
	}
	if resp.StatusCode == 200 {
		return result, nil
	}

	// TODO handle other status codes

	return LTResult{}, nil
}

type warnings struct {
	IncompleteResults bool `json:"incompleteResults"`
}

type software struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	BuildDate  string `json:"buildDate"`
	APIVersion int    `json:"apiVersion"`
	Status     string `json:"status"`
}

type language struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type match struct {
	Message      string        `json:"message"`
	ShortMessage string        `json:"shortMessage"`
	Replacements []replacement `json:"replacements"`
	Offset       int           `json:"offset"`
	Length       int           `json:"length"`
	Context      context       `json:"context"`
	Rule         rule          `json:"rule"`
}

type replacement struct {
	Value string `json:"value"`
}

type context struct {
	Text   string `json:"text"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

type rule struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	IssueType   string   `json:"issueType"`
	Category    category `json:"category"`
}

type category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
