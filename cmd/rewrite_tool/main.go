package main

import (
	"fmt"
	"github.com/jonbodner/rewrite"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

/*
go build -a -toolexec "/Users/jon/projects/rewrite/rewrite_tool /Users/jon/projects/rewrite/chi_sample/ /Users/jon/projects/rewrite/chi_sample/chi_rules.txt"
*/
func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: rewrite_tool path rule_file <toolexec arguments>")
		os.Exit(1)
	}

	path := os.Args[1]

	varType, rules, err := rewrite.ParseRuleFile(os.Args[2])
	if err != nil {
		panic(err)
	}

	rulePairs := rewrite.BuildRulePairs(rules)

	tool, args := os.Args[3], os.Args[4:]
	toolName := filepath.Base(tool)
	if len(args) > 0 && args[0] == "-V=full" {
		// We can't alter the version output.
	} else {
		if toolName == "compile" {
			for _, v := range args {
				if strings.HasPrefix(v, path) && strings.HasSuffix(v, ".go") {
					fmt.Println("modifying:", v)
					code, err := os.ReadFile(v)
					if err != nil {
						panic(err)
					}

					out, err := rewrite.Process(varType, rulePairs, string(code))
					if err != nil {
						panic(err)
					}
					prefix := ""
					fmt.Println(os.Args, os.Environ())
					err = os.WriteFile(prefix+v, out, fs.ModePerm)
					if err != nil {
						panic(err)
					}
				}
			}
		}
	}

	// Simply run the tool.
	cmd := exec.Command(tool, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
