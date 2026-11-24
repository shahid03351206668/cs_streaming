package main

import (
	"fmt"
	"os"
	"time"
)

const (
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
)

type Runner struct {
	total, failures, warnings, skipped int
	section                            string
	failed, warned                     []string
}

func (r *Runner) Section(title string) {
	r.section = title
	fmt.Printf("\n%s== %s ==%s\n", colorBold, title, colorReset)
}

func (r *Runner) Check(name string, fn func() (string, error)) bool {
	r.total++
	start := time.Now()
	detail, err := fn()
	elapsed := time.Since(start).Round(time.Millisecond)
	if err == nil {
		fmt.Printf("%s[PASS]%s %-70s %s (%s)\n", colorGreen, colorReset, name, detail, elapsed)
		return true
	}
	r.failures++
	r.failed = append(r.failed, r.section+" › "+name+": "+err.Error())
	fmt.Printf("%s[FAIL]%s %-70s %v (%s)\n", colorRed, colorReset, name, err, elapsed)
	return false
}

// Warn is for findings that need a decision rather than a fix.
func (r *Runner) Warn(name string, fn func() (string, error)) bool {
	r.total++
	detail, err := fn()
	if err == nil {
		fmt.Printf("%s[PASS]%s %-70s %s\n", colorGreen, colorReset, name, detail)
		return true
	}
	r.warnings++
	r.warned = append(r.warned, r.section+" › "+name+": "+err.Error())
	fmt.Printf("%s[WARN]%s %-70s %v\n", colorYellow, colorReset, name, err)
	return false
}

func (r *Runner) MustCheck(name string, fn func() (string, error)) {
	if !r.Check(name, fn) {
		r.Summary()
		fmt.Println("\naborting: every later step depends on this one")
		os.Exit(1)
	}
}

func (r *Runner) Skip(name, reason string) {
	r.skipped++
	fmt.Printf("%s[SKIP]%s %-70s %s\n", colorCyan, colorReset, name, reason)
}

func (r *Runner) Summary() {
	fmt.Printf("\n%s%d checks: %s%d passed%s, %s%d failed%s, %s%d warnings%s, %s%d skipped%s\n",
		colorBold, r.total,
		colorGreen, r.total-r.failures-r.warnings, colorReset,
		colorRed, r.failures, colorReset,
		colorYellow, r.warnings, colorReset,
		colorCyan, r.skipped, colorReset)
	for _, f := range r.failed {
		fmt.Println("  FAIL  " + f)
	}
	for _, w := range r.warned {
		fmt.Println("  WARN  " + w)
	}
}

func (r *Runner) ExitCode() int {
	if r.failures > 0 {
		return 1
	}
	return 0
}
