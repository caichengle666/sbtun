package config

import "strings"

// 节点地区：测速时查到落地 IP 就用查到的；还没测过的按节点名猜一个（国旗、中文地名、英文地名或代码）。
var regionAliases = map[string][]string{
	"HK": {"香港", "hong kong", "hongkong"},
	"MO": {"澳门", "macau", "macao"},
	"TW": {"台湾", "台灣", "台北", "taiwan", "taipei"},
	"JP": {"日本", "东京", "大阪", "japan", "tokyo", "osaka"},
	"KR": {"韩国", "韓國", "首尔", "korea", "seoul"},
	"SG": {"新加坡", "狮城", "singapore"},
	"US": {"美国", "美國", "洛杉矶", "圣何塞", "硅谷", "纽约", "西雅图", "芝加哥", "达拉斯", "凤凰城", "united states", "america", "los angeles", "san jose", "seattle", "new york", "usa"},
	"GB": {"英国", "伦敦", "united kingdom", "london", "uk"},
	"DE": {"德国", "法兰克福", "germany", "frankfurt"},
	"FR": {"法国", "巴黎", "france", "paris"},
	"NL": {"荷兰", "阿姆斯特丹", "netherlands", "amsterdam"},
	"CA": {"加拿大", "多伦多", "温哥华", "canada", "toronto", "vancouver"},
	"AU": {"澳大利亚", "澳洲", "悉尼", "australia", "sydney"},
	"IN": {"印度", "孟买", "india", "mumbai"},
	"ID": {"印尼", "印度尼西亚", "雅加达", "indonesia", "jakarta"},
	"TR": {"土耳其", "伊斯坦布尔", "turkey", "istanbul"},
	"RU": {"俄罗斯", "莫斯科", "russia", "moscow"},
	"MY": {"马来西亚", "吉隆坡", "malaysia"},
	"TH": {"泰国", "曼谷", "thailand", "bangkok"},
	"VN": {"越南", "vietnam"},
	"PH": {"菲律宾", "philippines", "manila"},
	"AR": {"阿根廷", "argentina"},
	"BR": {"巴西", "brazil"},
}

// GuessRegion 从节点名推测地区代码，猜不出返回空。名字里出现多个地区时取最靠前的。
func GuessRegion(name string) string {
	if code := flagRegion(name); code != "" {
		return code
	}
	lower := strings.ToLower(name)
	best, bestAt, bestLen := "", len(lower), 0
	consider := func(code string, at, size int) {
		if at >= 0 && (at < bestAt || at == bestAt && size > bestLen) {
			best, bestAt, bestLen = code, at, size
		}
	}
	for code, aliases := range regionAliases {
		for _, alias := range aliases {
			consider(code, wordIndex(lower, alias), len(alias))
		}
		consider(code, wordIndex(lower, strings.ToLower(code)), 2)
	}
	return best
}

// flagRegion 识别名字里的国旗 emoji（两个区域指示符）。
func flagRegion(name string) string {
	runes := []rune(name)
	for i := 0; i+1 < len(runes); i++ {
		a, b := runes[i], runes[i+1]
		if a >= 0x1F1E6 && a <= 0x1F1FF && b >= 0x1F1E6 && b <= 0x1F1FF {
			code := string([]rune{'A' + a - 0x1F1E6, 'A' + b - 0x1F1E6})
			if code == "UK" {
				return "GB"
			}
			return code
		}
	}
	return ""
}

// wordIndex 找子串位置；纯英文词要求前后不是字母，避免 "us" 命中 "plus"。
func wordIndex(s, word string) int {
	ascii := word[0] < 0x80
	for start := 0; ; {
		i := strings.Index(s[start:], word)
		if i < 0 {
			return -1
		}
		i += start
		if !ascii || !isLetter(s, i-1) && !isLetter(s, i+len(word)) {
			return i
		}
		start = i + 1
	}
}

func isLetter(s string, i int) bool {
	return i >= 0 && i < len(s) && (s[i] >= 'a' && s[i] <= 'z')
}

// fillRegions 给还没检测过落地 IP 的节点按名字补地区。
func fillRegions(cfg *Config) {
	for i := range cfg.Nodes {
		if cfg.Nodes[i].ExitIP == "" {
			cfg.Nodes[i].Country = GuessRegion(cfg.Nodes[i].Name)
		}
	}
}
