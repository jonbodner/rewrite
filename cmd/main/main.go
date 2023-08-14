package main

import (
	"fmt"
	"github.com/jonbodner/rewrite"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: rewrite rule_file source_file")
		os.Exit(1)
	}
	varType, rules, err := rewrite.ParseRuleFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	code, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}

	rulePairs := rewrite.BuildRulePairs(rules)
	out, err := rewrite.Process(varType, rulePairs, string(code))
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
}
