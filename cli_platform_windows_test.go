//go:build windows && cli

package main

import "testing"

// 带空格、单引号和空字符串的参数提权后要原样传给子进程。
func TestElevatedArgumentsQuotesEachArgument(t *testing.T) {
	got := elevatedArguments([]string{"edit", "1", "My Node", "it's", ""})
	want := `'edit 1 "My Node" it''s ""'`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
