package rules

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestConvertTextFormats(t *testing.T) {
	text := "payload:\n" +
		"  - '+.google.com'\n" +
		"  - '.youtube.com'\n" +
		"  - 'example.org'\n" +
		"  - DOMAIN-KEYWORD,openai,PROXY\n" +
		"  - IP-CIDR,10.1.2.3/8,no-resolve\n" +
		"  - IP-CIDR6,2001:db8::/32\n" +
		"  - DST-PORT,8443\n" +
		"  - PROCESS-NAME,Telegram.exe\n" +
		"  - USER-AGENT,Foo*\n" +
		"# comment\n" +
		"||ads.example.net^\n" +
		"0.0.0.0 tracker.example.com\n" +
		"1.1.1.1\n" +
		"example.org\n"
	data, count, err := ConvertText([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if count != 11 {
		t.Fatalf("count=%d", count)
	}
	var got struct {
		Version int              `json:"version"`
		Rules   []map[string]any `json:"rules"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := []map[string]any{
		{
			"domain":         []any{"example.org", "tracker.example.com"},
			"domain_suffix":  []any{"google.com", "youtube.com", "ads.example.net"},
			"domain_keyword": []any{"openai"},
			"ip_cidr":        []any{"10.0.0.0/8", "2001:db8::/32", "1.1.1.1/32"},
		},
		{"port": []any{float64(8443)}},
		{"process_name": []any{"Telegram.exe"}},
	}
	if got.Version != 2 || !reflect.DeepEqual(got.Rules, want) {
		t.Fatalf("got=%s", data)
	}
	if _, _, err := ConvertText([]byte("<html>not found</html>")); err == nil {
		t.Fatal("网页不应被当成规则列表")
	}
}

func TestFileFormat(t *testing.T) {
	if FileFormat("a/b.json") != "source" || FileFormat("a/b.srs") != "binary" {
		t.Fatal("格式判断错误")
	}
}
