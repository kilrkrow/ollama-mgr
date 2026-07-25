// Package origin maps known Ollama model lines to an approximate country of origin
// (lab / company HQ). This is curated metadata — not provided by the Ollama API.
package origin

import (
	"strings"
	"unicode"
)

// Info is a country-of-origin guess for a model family.
type Info struct {
	Code    string `json:"code"`    // ISO 3166-1 alpha-2
	Name    string `json:"name"`    // English short name
	Flag    string `json:"flag"`    // emoji flag (CLI; GUI uses flagcdn PNGs)
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
	if info, ok := exact[base]; ok {
		return info
	}
	for _, rule := range rules {
		if rule.match(base) {
			return rule.info
		}
	}
	return unknown()
}

func unknown() Info {
	return Info{Code: "", Name: "Unknown", Flag: "", Org: "", Unknown: true}
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
func se(org string) Info { return Info{Code: "SE", Name: "Sweden", Flag: flag("SE"), Org: org} }
func au(org string) Info { return Info{Code: "AU", Name: "Australia", Flag: flag("AU"), Org: org} }
func in_(org string) Info { return Info{Code: "IN", Name: "India", Flag: flag("IN"), Org: org} }

func flag(code string) string {
	code = strings.ToUpper(code)
	if len(code) != 2 {
		return ""
	}
	r1 := rune(code[0]) - 'A' + 0x1F1E6
	r2 := rune(code[1]) - 'A' + 0x1F1E6
	if r1 < 0x1F1E6 || r2 < 0x1F1E6 {
		return ""
	}
	return string([]rune{r1, r2})
}

var exact = map[string]Info{
	"llama2": us("Meta"), "llama3": us("Meta"), "llama3.1": us("Meta"), "llama3.2": us("Meta"),
	"llama3.3": us("Meta"), "llama4": us("Meta"), "codellama": us("Meta"),
	"gemma": us("Google"), "gemma2": us("Google"), "gemma3": us("Google"), "gemma4": us("Google"),
	"phi": us("Microsoft"), "phi3": us("Microsoft"), "phi4": us("Microsoft"), "phi4-mini": us("Microsoft"),
	"nemotron": us("NVIDIA"), "nemotron3": us("NVIDIA"), "nemotron-3-nano": us("NVIDIA"),
	"mistral": fr("Mistral AI"), "mixtral": fr("Mistral AI"), "codestral": fr("Mistral AI"),
	"pixtral": fr("Mistral AI"), "ministral": fr("Mistral AI"), "devstral": fr("Mistral AI"),
	"mistral-nemo": fr("Mistral AI"), "mistral-small": fr("Mistral AI"), "mistral-large": fr("Mistral AI"),
	"command-r": ca("Cohere"), "command-r-plus": ca("Cohere"), "aya": ca("Cohere"), "command-a": ca("Cohere"),
	"deepseek-r1": cn("DeepSeek"), "deepseek-v2": cn("DeepSeek"), "deepseek-v3": cn("DeepSeek"),
	"deepseek-coder": cn("DeepSeek"), "deepseek-coder-v2": cn("DeepSeek"),
	"qwen": cn("Alibaba"), "qwen2": cn("Alibaba"), "qwen2.5": cn("Alibaba"),
	"qwen2.5-coder": cn("Alibaba"), "qwen3": cn("Alibaba"), "qwen3-coder": cn("Alibaba"),
	"qwen3.5": cn("Alibaba"), "qwen3.6": cn("Alibaba"), "qwq": cn("Alibaba"),
	"yi": cn("01.AI"), "glm4": cn("Zhipu"), "glm-4": cn("Zhipu"), "glm-4.7-flash": cn("Zhipu"),
	"internlm": cn("Shanghai AI Lab"), "internlm2": cn("Shanghai AI Lab"),
	"nomic-embed-text": us("Nomic"), "nomic-embed-text-v2-moe": us("Nomic"),
	"llava": us("community"), "moondream": us("Vikhyat"), "tinyllama": us("community"),
	"orca-mini": us("community"), "falcon": ae("TII"), "jais": ae("G42 / Core42"),
	"stablelm": gb("Stability AI"), "stable-code": gb("Stability AI"),
	"solar": kr("Upstage"), "eeve": kr("Yanolja"),
	"granite": us("IBM"), "granite-code": us("IBM"),
	"olmo": us("AI2"), "tulu": us("AI2"), "molmo": us("AI2"),
	"smollm": us("Hugging Face"), "smollm2": us("Hugging Face"),
	"all-minilm": us("community / SBERT"), "mxbai-embed-large": de("Mixedbread"),
	"bge-m3": cn("BAAI"), "bge-large": cn("BAAI"),
	"firefunction": us("Fireworks"), "firefunction-v2": us("Fireworks"),
	"athene": us("Nexusflow"), "athene-v2": us("Nexusflow"),
	"reflection": us("HyperWrite / community"),
	"wizardlm2": us("Microsoft / community"), "wizardlm": us("Microsoft / community"),
	"openchat": us("community"), "zephyr": us("Hugging Face / community"),
	"neural-chat": us("Intel"), "sqlcoder": us("Defog"),
	"codegemma": us("Google"), "codeqwen": cn("Alibaba"),
	"starcoder2": us("BigCode"), "starcoder": us("BigCode"),
	"deepseek-llm": cn("DeepSeek"), "yi-coder": cn("01.AI"),
	"qwen2-math": cn("Alibaba"), "qwen2.5-math": cn("Alibaba"),
	"llama-guard": us("Meta"), "llama-guard3": us("Meta"),
	"shieldgemma": us("Google"), "hermes3": us("Nous Research"),
	"openhermes": us("Nous Research"), "nous-hermes": us("Nous Research"),
	"dolphin-mixtral": us("cognitivecomputations"), "dolphin-llama3": us("cognitivecomputations"),
	"samantha": us("community"), "meditron": us("EPFL / community"),
	"medllama2": us("community"), "bioscience": us("community"),
	"reader-lm": us("Jina AI"), "jina-embeddings": de("Jina AI"),
	"snowflake-arctic-embed": us("Snowflake"), "snowflake-arctic": us("Snowflake"),
	"dbrx": us("Databricks"), "gte-qwen2": cn("Alibaba"),
	"paraphrase-multilingual": de("UKP / community"),
	"embed-english": ca("Cohere"), "embed-multilingual": ca("Cohere"),
	"mistrallite": us("community"), "notux": us("community"),
	"tinydolphin": us("cognitivecomputations"), "stable-beluga": us("Stability AI"),
	"orca2": us("Microsoft"), "phi3.5": us("Microsoft"),
	"exaone": kr("LG AI Research"), "hyperclovax": kr("Naver"),
	"japanese-stablelm": jp("Stability AI Japan / community"),
	"swallow": jp("Tokyo Tech / community"), "llm-jp": jp("community"),
	"sarvam": in_("Sarvam AI"), "airavata": in_("community"),
	"vikhr": us("community"), // often RU data but community US-hosted — leave community US
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

var rules = []matchRule{
	{prefixes: []string{"qwen2.5-coder", "qwen3-coder", "qwen2.5", "qwen3", "qwen2", "qwen", "qwq", "codeqwen"}, info: cn("Alibaba")},
	{prefixes: []string{"deepseek"}, info: cn("DeepSeek")},
	{prefixes: []string{"llama-guard", "llama", "codellama"}, info: us("Meta")},
	{prefixes: []string{"codegemma", "shieldgemma", "gemma"}, info: us("Google")},
	{prefixes: []string{"phi"}, info: us("Microsoft")},
	{prefixes: []string{"nemotron"}, info: us("NVIDIA")},
	{prefixes: []string{"mistral", "mixtral", "codestral", "pixtral", "ministral", "devstral", "magistral"}, info: fr("Mistral AI")},
	{prefixes: []string{"command", "aya", "c4ai", "embed-english", "embed-multilingual"}, info: ca("Cohere")},
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
	{prefixes: []string{"solar", "eeve", "exaone", "hyperclova"}, info: kr("Korea labs")},
	{prefixes: []string{"stablelm", "stable-code", "stable-beluga", "stable-diffusion"}, info: gb("Stability AI")},
	{prefixes: []string{"wizardlm", "wizardcoder", "wizard-vicuna"}, info: us("Microsoft / community")},
	{prefixes: []string{"vicuna", "alpaca"}, info: us("community (US academia)")},
	{prefixes: []string{"llava", "bakllava"}, info: us("community")},
	{prefixes: []string{"openhermes", "hermes", "nous-"}, info: us("Nous Research")},
	{prefixes: []string{"dolphin", "tinydolphin"}, info: us("cognitivecomputations")},
	{prefixes: []string{"starcoder", "starling"}, info: us("BigCode / community")},
	{prefixes: []string{"snowflake-arctic", "arctic"}, info: us("Snowflake")},
	{prefixes: []string{"dbrx"}, info: us("Databricks")},
	{prefixes: []string{"jina", "reader-lm"}, info: de("Jina AI")},
	{prefixes: []string{"mxbai", "mixedbread"}, info: de("Mixedbread")},
	{prefixes: []string{"bge-", "bge"}, info: cn("BAAI")},
	{prefixes: []string{"gte-"}, info: cn("Alibaba")},
	{prefixes: []string{"smollm", "zephyr"}, info: us("Hugging Face / community")},
	{prefixes: []string{"firefunction", "athene"}, info: us("US labs")},
	{prefixes: []string{"sqlcoder"}, info: us("Defog")},
	{prefixes: []string{"neural-chat"}, info: us("Intel")},
	{prefixes: []string{"sarvam", "airavata"}, info: in_("India labs / community")},
	{prefixes: []string{"swallow", "llm-jp", "japanese-stablelm"}, info: jp("Japan community")},
	{prefixes: []string{"embed"}, contains: []string{"nomic-embed"}, info: us("Nomic")},
}
