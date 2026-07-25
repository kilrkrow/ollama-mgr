package catalog

import "testing"

func TestParsePopularHTML(t *testing.T) {
	html := `
<a href="/library/llama3.1" class="group w-full space-y-5">llama</a>
<span>117.7M</span><span class="hidden sm:flex">&nbsp;Pulls</span>
<a href="/library/deepseek-r1" class="group w-full space-y-5">ds</a>
<span>90.4M</span><span class="hidden sm:flex">&nbsp;Pulls</span>
<a href="/library/llama3.1" class="group w-full space-y-5">dup</a>
`
	list := parsePopularHTML(html)
	if len(list) < 2 {
		t.Fatalf("got %d", len(list))
	}
	if list[0].Name != "llama3.1" || list[0].Rank != 1 {
		t.Fatalf("%+v", list[0])
	}
	if list[0].Pulls != "117.7M" {
		t.Fatalf("pulls=%q", list[0].Pulls)
	}
	if list[1].Name != "deepseek-r1" {
		t.Fatalf("%+v", list[1])
	}
}

func TestPopularPage(t *testing.T) {
	all := make([]RankedModel, 30)
	for i := range all {
		all[i] = RankedModel{Rank: i + 1, Name: "m" + string(rune('a'+i%26))}
	}
	// fix names uniquely
	for i := range all {
		all[i].Name = "model-" + itoa(i)
		all[i].Rank = i + 1
	}
	page, total := PopularPage(all, 25, 1, 10)
	if total != 25 {
		t.Fatalf("total=%d", total)
	}
	if len(page) != 10 || page[0].Rank != 11 {
		t.Fatalf("page=%+v", page)
	}
}

func TestNormalizeTop(t *testing.T) {
	if NormalizeTop(7) != 10 || NormalizeTop(30) != 50 || NormalizeTop(100) != 100 {
		t.Fatal(NormalizeTop(7), NormalizeTop(30), NormalizeTop(100))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
