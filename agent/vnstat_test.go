package main

import "testing"

func TestParseVnStatJSON(t *testing.T) {
	b := []byte(`{"interfaces":[{"name":"eth0","traffic":{"day":[{"date":{"year":2026,"month":7,"day":11},"rx":200,"tx":100}]}},{"name":"eth1","traffic":{"day":[{"date":{"year":2026,"month":7,"day":11},"rx":20,"tx":10}]}}]}`)
	days, err := parseVnStatJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Sent != 110 || days[0].Recv != 220 {
		t.Fatalf("vnStat 解析错误: %+v", days)
	}
}
