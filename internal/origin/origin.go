// Package origin maps known Ollama model lines to an approximate country of origin
// (lab / company HQ). This is curated metadata â€” not provided by the Ollama API.
package origin

import (
	"strings"
	"unicode"
)

// Info is a country-of-origin guess for a model family.
type Info struct {
	Code    string `json:"code"`    // ISO 3166-1 alpha-2
	Name    string `json:"name"`    // English short name
	Flag    string `json:"flag"`    // emoji flag
	Org     string `json:"org"`     // lab / company when known
	Unknown bool   `json:"unknown"` // true if we have no mapping
}

// Lookup returns origin for a library base name (e.g. qwen3-coder, llama3.1, mistral).
func Lookup(base string) Info {
	base = strings.ToLower(strings.TrimSpace(base))
	if i := strings.Index(base, ":"); i >= 0 {
		base = base[:i]
	}
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if base == "" {
		return unknown()
	}

	// Exact overrides first
	if info, ok := exact[base]; ok {
		return info
	}

	// Prefix / contains rules (longest brand tokens first)
	for _, rule := range rules {
		if rule.match(base) {
			return rule.info
		}
	}
	return unknown()
}

func unknown() Info {
	return Info{Code: "", Name: "Unknown", Flag: "ðŸ³ï¸", Org: "", Unknown: true}
}

func us(org string) Info { return Info{Code: "US", Name: "United States", Flag: flag("US"), Org: org} }
func cn(org string) Info { return Info{Code: "CN", Name: "China", Flag: flag("CN"), Org: org} }
func fr(org string) Info { return Info{Code: "FR", Name: "France", Flag: flag("FR"), Org: org} }
func ca(org string) Info { return Info{Code: "CA", Name: "Canada", Flag: flag("CA"), Org: org} }
func kr(org string) Info { return Info{Code: "KR", Name: "South Korea", Flag: flag("KR"), Org: org} }
func il(org string) Info { return Info{Code: "IL", Name: "Israel", Flag: flag("IL"), Org: org} }
func ae(org string) Info { return Info{Code: "AE", Name: "UAE", Flag: flag("AE"), Org: org} }
func jp(org string) Info { return Info{Code: "JP", Name: "Japan", Flag: flag("JP"), Org: org} }
func de(org string) Info { return Info{Code: "DE", Name: "Germany", Flag: flag("DE"), Org: org} }
func gb(org string) Info { return Info{Code: "GB", Name: "United Kingdom", Flag: flag("GB"), Org: org} }

// flag builds a regional-indicator emoji pair from an ISO alpha-2 code.
func flag(code string) string {
	code = strings.ToUpper(code)
	if len(code) != 2 {
		return "ðŸ³ï¸"
	}
	r1 := rune(code[0]) - 'A' + 0x1F1E6
	r2 := rune(code[1]) - 'A' + 0x1F1E6
	if r1 < 0x1F1E6 || r2 < 0x1F1E6 {
		return "ðŸ³ï¸"
	}
	return string([]rune{r1, r2})
}

var exact = map[string]Info{
	"llama2": us("Meta"), "llama3": us("Meta"), "llama3.1": us("Meta"), "llama3.2": us("Meta"),
	"llama3.3": us("Meta"), "llama4": us("Meta"), "codellama": us("Meta"),
	"gemma": us("Google"), "gemma2": us("Google"), "gemma3": us("Google"), "gemma4": us("Google"),
	"phi": us("Microsoft"), "phi3": us("Microsoft"), "phi4": us("Microsoft"),
	"nemotron": us("NVIDIA"), "nemotron3": us("NVIDIA"), "nemotron-3-nano": us("NVIDIA"),
	"mistral": fr("Mistral AI"), "mixtral": fr("Mistral AI"), "codestral": fr("Mistral AI"),
	"pixtral": fr("Mistral AI"), "ministral": fr("Mistral AI"),
	"command-r": ca("Cohere"), "command-r-plus": ca("Cohere"), "aya": ca("Cohere"),
	"deepseek-r1": cn("DeepSeek"), "deepseek-v2": cn("DeepSeek"), "deepseek-v3": cn("DeepSeek"),
	"deepseek-coder": cn("DeepSeek"), "deepseek-coder-v2": cn("DeepSeek"),
	"qwen": cn("Alibaba"), "qwen2": cn("Alibaba"), "qwen2.5": cn("Alibaba"),
	"qwen2.5-coder": cn("Alibaba"), "qwen3": cn("Alibaba"), "qwen3-coder": cn("Alibaba"),
	"qwen3.5": cn("Alibaba"), "qwen3.6": cn("Alibaba"), "qwq": cn("Alibaba"),
	"yi": cn("01.AI"), "glm4": cn("Zhipu"), "glm-4": cn("Zhipu"),
	"internlm": cn("Shanghai AI Lab"), "internlm2": cn("Shanghai AI Lab"),
	"nomic-embed-text": us("Nomic"), "nomic-embed-text-v2-moe": us("Nomic"),
	"llava": us("community / UWâ€“Madison et al."), "moondream": us("Vikhyat"),
	"tinyllama": us("community"), "orca-mini": us("community"),
	"falcon": ae("TII"), "jais": ae("G42 / Core42"),
	"stablelm": gb("Stability AI"), "stable-code": gb("Stability AI"),
	"solar": kr("Upstage"), "eeve": kr("Yanolja"),
	"granite": us("IBM"), "granite-code": us("IBM"),
	"olmo": us("AI2"), "tulu": us("AI2"),
	"command-a": ca("Cohere"),
}

type matchRule struct {
	prefixes []string
	contains []string
	info     Info
}

func (r matchRule) match(base string) bool {
	for _, p := range r.prefixes {
		if base == p || strings.HasPrefix(base, p+"-") || strings.HasPrefix(base, p+".") || strings.HasPrefix(base, p+"_") {
			return true
		}
		// qwen3.6 style already covered by prefix qwen
		if strings.HasPrefix(base, p) && len(base) > len(p) {
			next := rune(base[len(p)])
			if unicode.IsDigit(next) || next == '-' || next == '.' || next == '_' {
				return true
			}
		}
	}
	for _, c := range r.contains {
		if strings.Contains(base, c) {
			return true
		}
	}
	return false
}

// Order matters: more specific prefixes before shorter ones where needed.
var rules = []matchRule{
	{prefixes: []string{"qwen2.5-coder", "qwen3-coder", "qwen2.5", "qwen3", "qwen2", "qwen", "qwq"}, info: cn("Alibaba")},
	{prefixes: []string{"deepseek"}, info: cn("DeepSeek")},
	{prefixes: []string{"llama", "codellama"}, info: us("Meta")},
	{prefixes: []string{"gemma"}, info: us("Google")},
	{prefixes: []string{"phi"}, info: us("Microsoft")},
	{prefixes: []string{"nemotron"}, info: us("NVIDIA")},
	{prefixes: []string{"mistral", "mixtral", "codestral", "pixtral", "ministral", "devstral", "magistral"}, info: fr("Mistral AI")},
	{prefixes: []string{"command", "aya", "c4ai"}, info: ca("Cohere")},
	{prefixes: []string{"glm", "chatglm"}, info: cn("Zhipu")},
	{prefixes: []string{"yi-", "yi"}, info: cn("01.AI")},
	{prefixes: []string{"internlm", "internvl"}, info: cn("Shanghai AI Lab")},
	{prefixes: []string{"baichuan"}, info: cn("Baichuan")},
	{prefixes: []string{"hunyuan"}, info: cn("Tencent")},
	{prefixes: []string{"ernie", "paddle"}, info: cn("Baidu")},
	{prefixes: []string{"minicpm"}, info: cn("ModelBest / OpenBMB")},
	{prefixes: []string{"nomic"}, info: us("Nomic")},
	{prefixes: []string{"granite"}, info: us("IBM")},
	{prefixes: []string{"olmo", "tulu", "molmo"}, info: us("AI2")},
	{prefixes: []string{"falcon"}, info: ae("TII")},
	{prefixes: []string{"jais"}, info: ae("G42")},
	{prefixes: []string{"solar", "eeve"}, info: kr("Upstage / Yanolja")},
	{prefixes: []string{"stablelm", "stable-code", "stable-diffusion"}, info: gb("Stability AI")},
	{prefixes: []string{"wizardlm", "wizardcoder", "wizard-vicuna"}, info: us("Microsoft / community")},
	{prefixes: []string{"vicuna", "alpaca"}, info: us("community (US academia)")},
	{prefixes: []string{"llava", "bakllava"}, info: us("community")},
	{prefixes: []string{"openhermes", "hermes", "nous-"}, info: us("Nous Research")},
	{prefixes: []string{"dolphin"}, info: us("cognitivecomputations")},
	{prefixes: []string{"starcoder", "starling"}, contains: []string{"starcoder"}, info: us("BigCode / community")},
	{prefixes: []string{"snowflake-arctic", "arctic"}, info: us("Snowflake")},
	{prefixes: []string{"dbrx"}, info: us("Databricks")},
	{prefixes: []string{"jina"}, info: de("Jina AI")},
	{prefixes: []string{"mxbai", "mixedbread"}, info: de("Mixedbread")},
	{prefixes: []string{"bge-", "bge"}, info: cn("BAAI")},
	{prefixes: []string{"gte-"}, info: cn("Alibaba")},
	{prefixes: []string{"embed"}, contains: []string{"nomic-embed"}, info: us("Nomic")},
}
