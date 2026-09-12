package app

import "testing"

func TestParseCustomRulesText(t *testing.T) {
	rules, err := parseCustomRules([]byte("proxy,domain_suffix,example.com\ndirect,ip_cidr,10.0.0.0/8\n# comment\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Action != "proxy" || rules[1].MatchType != "ip_cidr" {
		t.Fatalf("rules=%+v", rules)
	}
}

func TestParseCustomRulesJSON(t *testing.T) {
	rules, err := parseCustomRules([]byte(`[{"match_type":"port","value":"443","action":"proxy"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Value != "443" {
		t.Fatalf("rules=%+v", rules)
	}
}
